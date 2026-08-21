package rca

import (
	"context"
	"time"
)

// Service 是 RCA 服务，整合 Pipeline 和 Repository。
type Service struct {
	pipeline   *Pipeline
	repository *AnalysisRepository
}

func NewService(pipeline *Pipeline, repository *AnalysisRepository) *Service {
	return &Service{pipeline: pipeline, repository: repository}
}

// Analyze 执行 RCA 分析并保存结果。
func (s *Service) Analyze(ctx context.Context, incidentID int64, cluster, namespace, service, resourceType, resourceName string, startTime, endTime time.Time) (*RCAResult, error) {
	result, err := s.pipeline.Analyze(ctx, incidentID, cluster, namespace, service, resourceType, resourceName, startTime, endTime)
	if err != nil {
		return nil, err
	}

	// 保存分析快照。
	analysis := &IncidentAnalysis{
		IncidentID: incidentID,
		Type:       AnalysisTypeRCA,
		Result:     result,
	}
	if _, err := s.repository.Create(ctx, analysis); err != nil {
		// 保存失败不影响返回结果。
		// 但记录日志。
	}

	return result, nil
}

// GetLatest 获取最近的 RCA 结果。
func (s *Service) GetLatest(ctx context.Context, incidentID int64) (*RCAResult, error) {
	analysis, err := s.repository.FindLatest(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	return analysis.Result, nil
}

// GetHistory 获取 RCA 历史。
func (s *Service) GetHistory(ctx context.Context, incidentID int64, limit int) ([]IncidentAnalysis, error) {
	return s.repository.FindHistory(ctx, incidentID, limit)
}

// CollectEvidence 只收集 Evidence，不执行 RCA 分析。
// 用于 GET /incidents/:id/evidence API，不依赖 RCA 先执行。
func (s *Service) CollectEvidence(ctx context.Context, incidentID int64, cluster, namespace, service, resourceType, resourceName string, startTime, endTime time.Time) (*EvidenceBundle, error) {
	return s.pipeline.CollectEvidence(ctx, incidentID, cluster, namespace, service, resourceType, resourceName, startTime, endTime)
}
