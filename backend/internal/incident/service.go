package incident

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Service 是 Incident 业务逻辑层。
// 负责：信号接入 → 关联 → 创建/更新 Incident → 生命周期管理。
type Service struct {
	repo       *Repository
	correlator *Correlator
}

func NewService(repo *Repository, correlator *Correlator) *Service {
	return &Service{repo: repo, correlator: correlator}
}

// IngestSignal 接入一个新信号，自动关联或创建 Incident。
// 这是 Alert/Anomaly/Log/Event 等所有信号进入 Incident 系统的统一入口。
func (s *Service) IngestSignal(ctx context.Context, sig Signal) (*Incident, *IncidentSignal, error) {
	if sig.SignalID == "" {
		return nil, nil, fmt.Errorf("signal_id 不能为空")
	}
	if sig.Timestamp.IsZero() {
		sig.Timestamp = time.Now()
	}

	// 1. 检查信号是否已存在（去重）。
	existingSig, err := s.repo.FindSignalByExternalID(ctx, string(sig.SignalType), sig.SignalID)
	if err == nil && existingSig.ID > 0 {
		// 信号已存在，更新状态。
		return s.updateExistingSignal(ctx, existingSig, sig)
	}

	// 2. 查找可关联的活跃 Incident。
	since := sig.Timestamp.Add(-s.correlator.cfg.TimeWindow * 2)
	candidates, err := s.repo.FindActiveByResource(ctx, sig.Cluster, sig.Namespace, sig.Service, since)
	if err != nil {
		slog.Warn("incident: find active candidates failed", "err", err)
		candidates = nil
	}

	// 3. 关联评分。
	bestInc, score := s.correlator.FindBestMatch(sig, candidates)

	var inc *Incident
	if bestInc != nil {
		// 关联到已有 Incident。
		inc = bestInc
		slog.Info("incident: signal correlated",
			"signal_id", sig.SignalID, "incident_id", inc.ID, "score", score.Total)
	} else {
		// 创建新 Incident。
		inc = s.buildIncidentFromSignal(sig)
		inc, err = s.repo.Create(ctx, inc)
		if err != nil {
			return nil, nil, fmt.Errorf("create incident failed: %w", err)
		}
		slog.Info("incident: created", "id", inc.ID, "title", inc.Title)
	}

	// 4. 创建信号记录。
	dbSig := s.signalToDB(sig, inc.ID)
	dbSig, err = s.repo.UpsertSignal(ctx, dbSig)
	if err != nil {
		return nil, nil, fmt.Errorf("upsert signal failed: %w", err)
	}

	// 5. 更新 Incident 的 signal_count 和聚合信息。
	if err := s.repo.IncrementSignalCount(ctx, inc.ID); err != nil {
		slog.Warn("incident: increment signal count failed", "err", err)
	}
	s.updateIncidentAggregation(ctx, inc, sig)

	// 6. 重新加载 Incident（含 signals）。
	inc, _ = s.repo.FindByID(ctx, inc.ID)
	return inc, dbSig, nil
}

// updateExistingSignal 更新已存在信号的状态（如 resolved）。
func (s *Service) updateExistingSignal(ctx context.Context, existing *IncidentSignal, sig Signal) (*Incident, *IncidentSignal, error) {
	existing.Resolved = sig.Resolved
	if sig.Resolved {
		now := time.Now()
		existing.ResolvedAt = &now
	}
	existing.Timestamp = sig.Timestamp
	if sig.Title != "" {
		existing.Title = sig.Title
	}
	if sig.Severity != "" {
		existing.Severity = sig.Severity
	}
	updated, err := s.repo.UpsertSignal(ctx, existing)
	if err != nil {
		return nil, nil, err
	}

	// 重新计算 Incident 状态。
	inc, err := s.repo.FindByID(ctx, existing.IncidentID)
	if err == nil {
		s.reevaluateStatus(ctx, inc)
	}
	return inc, updated, nil
}

// buildIncidentFromSignal 从信号构建新 Incident。
func (s *Service) buildIncidentFromSignal(sig Signal) *Incident {
	title := sig.Title
	if title == "" {
		title = fmt.Sprintf("%s: %s", sig.Severity, sig.SignalType)
	}
	return &Incident{
		Title:        title,
		Severity:     sig.Severity,
		Status:       StatusOpen,
		Cluster:      sig.Cluster,
		Namespace:    sig.Namespace,
		Service:      sig.Service,
		ResourceType: string(sig.ResourceType),
		ResourceName: sig.ResourceName,
		StartTime:    sig.Timestamp,
		SignalCount:  0,
	}
}

// signalToDB 将统一 Signal 转换为数据库记录。
func (s *Service) signalToDB(sig Signal, incidentID int64) *IncidentSignal {
	dbSig := &IncidentSignal{
		IncidentID:   incidentID,
		SignalType:   string(sig.SignalType),
		SignalID:     sig.SignalID,
		Title:        sig.Title,
		Severity:     sig.Severity,
		Cluster:      sig.Cluster,
		Namespace:    sig.Namespace,
		Service:      sig.Service,
		ResourceType: string(sig.ResourceType),
		ResourceName: sig.ResourceName,
		Timestamp:    sig.Timestamp,
		Resolved:     sig.Resolved,
		Metadata:     sig.Metadata,
	}
	if sig.Resolved {
		now := time.Now()
		dbSig.ResolvedAt = &now
	}
	return dbSig
}

// updateIncidentAggregation 更新 Incident 的聚合信息（严重度取最高、时间范围等）。
// 使用选择性更新，避免覆盖 signal_count 等并发字段。
func (s *Service) updateIncidentAggregation(ctx context.Context, inc *Incident, sig Signal) {
	updates := map[string]any{}
	// 严重度升级。
	if severityRank(sig.Severity) > severityRank(inc.Severity) {
		inc.Severity = sig.Severity
		updates["severity"] = sig.Severity
	}
	// 更新最早开始时间。
	if sig.Timestamp.Before(inc.StartTime) {
		inc.StartTime = sig.Timestamp
		updates["start_time"] = sig.Timestamp
	}
	// 如果信号有 service 而 Incident 没有，补充。
	if inc.Service == "" && sig.Service != "" {
		inc.Service = sig.Service
		updates["service"] = sig.Service
	}
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := s.repo.db.WithContext(ctx).Model(&Incident{}).Where("id = ?", inc.ID).Updates(updates).Error; err != nil {
			slog.Warn("incident: update aggregation failed", "err", err)
		}
	}
}

// reevaluateStatus 重新评估 Incident 状态。
// 当所有主要信号都 resolved 时，自动将 Incident 标记为 resolved。
func (s *Service) reevaluateStatus(ctx context.Context, inc *Incident) {
	if inc.Status == StatusClosed {
		return
	}
	activeCount, err := s.repo.CountActiveSignals(ctx, inc.ID)
	if err != nil {
		slog.Warn("incident: count active signals failed", "err", err)
		return
	}
	if activeCount == 0 && inc.SignalCount > 0 {
		// 所有信号都 resolved，自动标记 Incident 为 resolved。
		if err := s.repo.UpdateStatus(ctx, inc.ID, StatusResolved); err != nil {
			slog.Warn("incident: auto resolve failed", "err", err)
			return
		}
		inc.Status = StatusResolved
		now := time.Now()
		inc.EndTime = &now
		slog.Info("incident: auto resolved", "id", inc.ID)
	}
}

// Acknowledge 确认 Incident。
func (s *Service) Acknowledge(ctx context.Context, id int64) error {
	return s.repo.UpdateStatus(ctx, id, StatusAcknowledged)
}

// Resolve 手动解决 Incident。
func (s *Service) Resolve(ctx context.Context, id int64) error {
	return s.repo.UpdateStatus(ctx, id, StatusResolved)
}

// Close 关闭 Incident。
func (s *Service) Close(ctx context.Context, id int64) error {
	return s.repo.UpdateStatus(ctx, id, StatusClosed)
}

// Get 获取 Incident 详情（含 signals）。
func (s *Service) Get(ctx context.Context, id int64) (*Incident, error) {
	return s.repo.FindByID(ctx, id)
}

// List 分页查询 Incident。
func (s *Service) List(ctx context.Context, filter ListFilter, page, pageSize int) ([]Incident, int64, error) {
	return s.repo.List(ctx, filter, page, pageSize)
}

// GetTimeline 获取 Incident 的统一时间线（所有信号按时间排序）。
func (s *Service) GetTimeline(ctx context.Context, id int64) ([]IncidentSignal, error) {
	return s.repo.ListSignals(ctx, id)
}

// severityRank 严重度排序，critical 最高。
func severityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}
