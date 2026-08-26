package rca

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// Pipeline 是 RCA 分析流水线。
type Pipeline struct {
	collector ContextCollector
	scorer    *Scorer
}

// NewPipeline 创建 RCA Pipeline。
func NewPipeline(collector ContextCollector) *Pipeline {
	return &Pipeline{
		collector: collector,
		scorer:    NewScorer(DefaultScoringConfig()),
	}
}

// Analyze 对指定 Incident 执行 RCA 分析。
func (p *Pipeline) Analyze(ctx context.Context, incidentID int64, cluster, namespace, service, resourceType, resourceName string, startTime, endTime time.Time) (*RCAResult, error) {
	result := &RCAResult{
		IncidentID:  incidentID,
		Status:      RCAStatusAnalyzing,
		GeneratedAt: time.Now(),
	}

	// 1. 收集上下文。
	incidentCtx, err := p.collectContext(ctx, incidentID, cluster, namespace, service, resourceType, resourceName, startTime, endTime)
	if err != nil {
		slog.Warn("rca: collect context failed", "err", err)
		// 部分失败不阻塞，继续用已有数据。
	}

	// 2. 检查证据是否充足。
	totalEvidence := len(incidentCtx.Alerts) + len(incidentCtx.Anomalies) + len(incidentCtx.Events)
	if totalEvidence == 0 {
		result.Status = RCAStatusInsufficientEvidence
		result.RootCause = "当前证据不足，无法确定根因"
		result.Explanation = "未收集到告警、异常或 Kubernetes Event 证据。请检查数据源是否正常。"
		result.Confidence = 0
		return result, nil
	}

	// 3. 生成候选根因。
	candidates := p.generateCandidates(incidentCtx)
	if len(candidates) == 0 {
		result.Status = RCAStatusInsufficientEvidence
		result.RootCause = "无法确定根因资源"
		result.Explanation = "收集到证据但无法定位到具体资源。"
		return result, nil
	}

	// 4. 评分排序。
	for i := range candidates {
		candidates[i].Score = p.scorer.ScoreCandidate(incidentCtx, candidates[i])
		candidates[i].Confidence = CalculateConfidence(candidates[i], totalEvidence)
		candidates[i].Explanation = p.generateExplanation(candidates[i])
		// P1-X.10: 为每个候选判定 RootCauseStatus。
		candidates[i].Status = determineRootCauseStatus(candidates[i])
	}
	candidates = RankCandidates(candidates)

	// 5. 取 Top 候选作为最终根因。
	top := candidates[0]

	// P1-X.10: Evidence Sufficiency Gate — 根据 Evidence Level 判定最终状态。
	sufficiency := evaluateEvidenceSufficiency(top)
	result.EvidenceSufficiency = &sufficiency
	result.RootCauseStatus = top.Status
	result.ConfidenceReason = GetConfidenceCapReason(top)

	if top.Status == RootCauseStatusUnknown || top.Status == RootCauseStatusHypothesis {
		// P1-X.10: 证据不足时，不输出确定性 root_cause，改为 possible_causes。
		result.RootCause = ""
		result.Status = RCAStatusInsufficientEvidence
		result.PossibleCauses = buildPossibleCauses(candidates)
		result.Explanation = "证据不足，无法确认根因。请查看 Possible Causes 和 Recommendations 进行进一步调查。"
	} else {
		result.RootCause = top.RootCause
		result.Status = RCAStatusCompleted
	}

	result.Confidence = top.Confidence
	result.Candidates = candidates
	result.Evidence = top.Evidence
	result.Impact = top.Impact

	// P1-X.10: 生成 Recommendations（含 Safety Gate）。
	result.Recommendations = p.generateRecommendations(top, sufficiency)

	// 6. 构建统一时间线。
	result.Timeline = p.buildTimeline(incidentCtx)

	// 7. 生成整体解释。
	result.Explanation = p.generateOverallExplanation(top, candidates, incidentCtx)

	return result, nil
}

// CollectEvidence 只收集 Evidence，不执行 RCA 分析。
// 用于 GET /incidents/:id/evidence API，不依赖 RCA 先执行。
func (p *Pipeline) CollectEvidence(ctx context.Context, incidentID int64, cluster, namespace, service, resourceType, resourceName string, startTime, endTime time.Time) (*EvidenceBundle, error) {
	incidentCtx, err := p.collectContext(ctx, incidentID, cluster, namespace, service, resourceType, resourceName, startTime, endTime)
	if err != nil {
		return nil, err
	}

	bundle := &EvidenceBundle{
		IncidentID:   incidentID,
		Cluster:      cluster,
		Namespace:    namespace,
		Service:      service,
		ResourceType: resourceType,
		ResourceName: resourceName,
		TimeWindow: EvidenceTimeWindow{
			Start:    startTime,
			End:      endTime,
			Before:   startTime.Add(-30 * time.Minute),
		},
		Alerts:    incidentCtx.Alerts,
		Anomalies: incidentCtx.Anomalies,
		Events:    incidentCtx.Events,
		Metrics:   incidentCtx.Metrics,
		Logs:      incidentCtx.Logs,
		Topology:  incidentCtx.Topology,
		PodResourceState: incidentCtx.PodResourceState,
	}

	// 统计各数据源状态（P1-X.10 DataTrust: 区分 error vs no_data vs success）。
	bundle.Sources = EvidenceSources{
		Alerts:    dataSourceStatus(len(incidentCtx.Alerts), incidentCtx.SourceErrors["alerts"]),
		Anomalies: dataSourceStatus(len(incidentCtx.Anomalies), incidentCtx.SourceErrors["anomalies"]),
		Events:    dataSourceStatus(len(incidentCtx.Events), incidentCtx.SourceErrors["events"]),
		Metrics:   dataSourceStatus(len(incidentCtx.Metrics), incidentCtx.SourceErrors["metrics"]),
		Logs:      dataSourceStatus(len(incidentCtx.Logs), incidentCtx.SourceErrors["logs"]),
		Topology:  dataSourceStatus(len(incidentCtx.Topology.Nodes), incidentCtx.SourceErrors["topology"]),
		PodResourceState: podResourceStateStatus(incidentCtx.PodResourceState, incidentCtx.SourceErrors["pod_resource_state"]),
	}

	// 构建统一时间线。
	bundle.Timeline = p.buildTimeline(incidentCtx)

	return bundle, nil
}

// dataSourceStatus 返回数据源状态（P1-X.10 DataTrust: 区分 error vs no_data vs success）。
func dataSourceStatus(count int, err error) string {
	if err != nil {
		return "error"
	}
	if count == 0 {
		return "no_data"
	}
	return "success"
}

func podResourceStateStatus(state *PodResourceState, err error) string {
	if err != nil {
		return "error"
	}
	if state == nil {
		return "no_data"
	}
	return "success"
}

// collectContext 收集 RCA 上下文。
func (p *Pipeline) collectContext(ctx context.Context, incidentID int64, cluster, namespace, service, resourceType, resourceName string, startTime, endTime time.Time) (IncidentContext, error) {
	incidentCtx := IncidentContext{
		IncidentID:   incidentID,
		Cluster:      cluster,
		Namespace:    namespace,
		Service:      service,
		ResourceType: resourceType,
		ResourceName: resourceName,
		StartTime:    startTime,
		EndTime:      endTime,
		SourceErrors: make(map[string]error),
	}

	since := startTime.Add(-30 * time.Minute) // 往前看 30 分钟找根因。

	// 并行收集（每个失败只记日志，不阻塞）。
	type result struct {
		alerts    []AlertInfo
		anomalies []AnomalyInfo
		events    []EventInfo
		metrics   []MetricInfo
		logs      []LogInfo
		topology  TopologyInfo
		podState  *PodResourceState
	}
	var res result

	if alerts, err := p.collector.CollectAlerts(ctx, incidentID); err == nil {
		res.alerts = alerts
	} else {
		// P1-X.10 DataTrust: 记录错误，区分 Empty vs Error
		incidentCtx.SourceErrors["alerts"] = err
		slog.Warn("rca: collect alerts failed", "err", err)
	}

	if anomalies, err := p.collector.CollectAnomalies(ctx, cluster, namespace, resourceType, resourceName, since); err == nil {
		res.anomalies = anomalies
	} else {
		incidentCtx.SourceErrors["anomalies"] = err
		slog.Warn("rca: collect anomalies failed", "err", err)
	}

	if events, err := p.collector.CollectEvents(ctx, cluster, namespace, resourceType, resourceName, since); err == nil {
		res.events = events
	} else {
		incidentCtx.SourceErrors["events"] = err
		slog.Warn("rca: collect events failed", "err", err)
	}

	if metrics, err := p.collector.CollectMetrics(ctx, cluster, namespace, resourceType, resourceName, since, endTime); err == nil {
		res.metrics = metrics
	} else {
		incidentCtx.SourceErrors["metrics"] = err
		slog.Warn("rca: collect metrics failed", "err", err)
	}

	if resourceType == "pod" {
		if logs, err := p.collector.CollectLogs(ctx, cluster, namespace, resourceName, since, endTime); err == nil {
			res.logs = logs
		} else {
			incidentCtx.SourceErrors["logs"] = err
			slog.Warn("rca: collect logs failed", "err", err)
		}
	}

	if topology, err := p.collector.CollectTopology(ctx, cluster, namespace); err == nil {
		res.topology = topology
	} else {
		incidentCtx.SourceErrors["topology"] = err
		slog.Warn("rca: collect topology failed", "err", err)
	}

	// Pod Resource State：仅当 resourceType == "pod" 时收集。
	if resourceType == "pod" && resourceName != "" {
		if podState, err := p.collector.CollectPodResourceState(ctx, cluster, namespace, resourceName); err == nil {
			res.podState = podState
		} else {
			incidentCtx.SourceErrors["pod_resource_state"] = err
			slog.Warn("rca: collect pod resource state failed", "err", err)
		}
	}

	incidentCtx.Alerts = res.alerts
	incidentCtx.Anomalies = res.anomalies
	incidentCtx.Events = res.events
	incidentCtx.Metrics = res.metrics
	incidentCtx.Logs = res.logs
	incidentCtx.Topology = res.topology
	incidentCtx.PodResourceState = res.podState

	return incidentCtx, nil
}

// generateCandidates 从上下文中生成候选根因。
func (p *Pipeline) generateCandidates(ctx IncidentContext) []RootCauseCandidate {
	candidateMap := make(map[string]*RootCauseCandidate)
	fetchedAt := time.Now() // P1-X.10 Phase 4: Evidence 级 provenance 时间戳

	// 从 Anomaly 生成候选（Corroborating Evidence）。
	for _, a := range ctx.Anomalies {
		key := a.ResourceType + "/" + a.ResourceName
		cand, ok := candidateMap[key]
		if !ok {
			cand = &RootCauseCandidate{
				ResourceType: a.ResourceType,
				ResourceName: a.ResourceName,
				Namespace:    a.Namespace,
			}
			candidateMap[key] = cand
		}
		anomalyTS := a.Timestamp
		cand.Evidence = append(cand.Evidence, Evidence{
			ID:           fmt.Sprintf("anomaly-%d", a.ID),
			Type:         EvidenceTypeAnomaly,
			Level:        EvidenceLevelCorroborating, // P1-X.10: Anomaly 属于佐证证据
			Source:       "prometheus",
			SourceType:   "provider",
			Timestamp:    a.Timestamp,
			ResourceType: a.ResourceType,
			ResourceName: a.ResourceName,
			Namespace:    a.Namespace,
			Metric:       a.Metric,
			Value:        a.Value,
			Severity:     a.Severity,
			Description:  a.Reason,
			Score:        a.AnomalyScore,
			// P1-X.10 Phase 4: Evidence 级 provenance
			FetchedAt:          &fetchedAt,
			DataTimestamp:      &anomalyTS,
			TimestampAvailable: true,
			TrustStatus:        "fresh",
			CausalRelevance:    "supporting",
		})
	}

	// 从 K8s Event 生成候选（Direct / Corroborating Evidence）。
	for _, e := range ctx.Events {
		key := e.ResourceType + "/" + e.ResourceName
		cand, ok := candidateMap[key]
		if !ok {
			cand = &RootCauseCandidate{
				ResourceType: e.ResourceType,
				ResourceName: e.ResourceName,
				Namespace:    e.Namespace,
			}
			candidateMap[key] = cand
		}
		level := classifyEventEvidenceLevel(e.Reason, e.Message)
		score := 0.7
		if e.Type == "Warning" {
			score = 0.9
		}
		eventTS := e.Timestamp
		// P1-X.10 Phase 4: Causal relevance 基于 Evidence Level
		causalRelevance := "contextual"
		if level == EvidenceLevelDirect {
			causalRelevance = "direct_causal"
		} else if level == EvidenceLevelCorroborating {
			causalRelevance = "supporting"
		}
		cand.Evidence = append(cand.Evidence, Evidence{
			// P1-X.10 Phase 8: Event Evidence ID 包含 namespace/reason/resourceName/timestamp，确保稳定唯一
			ID:           fmt.Sprintf("event-%s-%s-%s-%d", e.Namespace, e.Reason, e.ResourceName, e.Timestamp.Unix()),
			Type:         EvidenceTypeEvent,
			Level:        level, // P1-X.10: Event 按 Reason 分级
			Source:       "kubernetes",
			SourceType:   "provider",
			Timestamp:    e.Timestamp,
			ResourceType: e.ResourceType,
			ResourceName: e.ResourceName,
			Namespace:    e.Namespace,
			Severity:     strings.ToLower(e.Type),
			Description:  fmt.Sprintf("%s: %s (count=%d)", e.Reason, e.Message, e.Count),
			Score:        score,
			// P1-X.10 Phase 4: Evidence 级 provenance
			FetchedAt:          &fetchedAt,
			DataTimestamp:      &eventTS,
			TimestampAvailable: true,
			TrustStatus:        "fresh",
			CausalRelevance:    causalRelevance,
		})
	}

	// P1-X.10 Phase 6: 从 Pod Resource State 提取直接故障证据（OOMKilled 等）。
	// OOMKilled 通常不在 K8s Events 中，而在 containerStatuses[].lastState.terminated.reason。
	if ctx.PodResourceState != nil && ctx.PodResourceState.Containers != nil {
		for _, cs := range ctx.PodResourceState.Containers {
			if cs.LastState != "terminated" || cs.LastReason == "" {
				continue
			}
			if cs.LastExitCode != nil && *cs.LastExitCode == 0 && cs.LastReason == "" {
				continue
			}

			key := "pod/" + ctx.PodResourceState.Pod
			cand, ok := candidateMap[key]
			if !ok {
				cand = &RootCauseCandidate{
					ResourceType: "pod",
					ResourceName: ctx.PodResourceState.Pod,
					Namespace:    ctx.PodResourceState.Namespace,
				}
				candidateMap[key] = cand
			}

			level := classifyPodStatusEvidenceLevel(cs.LastReason)
			causalRelevance := "contextual"
			if level == EvidenceLevelDirect {
				causalRelevance = "direct_causal"
			} else if level == EvidenceLevelCorroborating {
				causalRelevance = "supporting"
			}

			var dataTimestamp *time.Time
			timestampAvailable := false
			// P1-X.10 Phase 6: 优先使用 lastState.terminated.finishedAt
			tsSource := cs.LastFinishedAt
			if tsSource == "" {
				tsSource = cs.FinishedAt
			}
			if tsSource != "" {
				if t, err := time.Parse(time.RFC3339, tsSource); err == nil {
					dataTimestamp = &t
					timestampAvailable = true
				}
			}

			evidenceID := fmt.Sprintf("pod-status-%s-%s-%s-%s", cs.LastReason, ctx.PodResourceState.Namespace, ctx.PodResourceState.Pod, cs.Name)
			if dataTimestamp != nil {
				evidenceID = fmt.Sprintf("%s-%d", evidenceID, dataTimestamp.Unix())
			}

			description := fmt.Sprintf("Container %s last terminated with reason %s", cs.Name, cs.LastReason)
			if cs.LastExitCode != nil {
				description = fmt.Sprintf("%s (exitCode=%d)", description, *cs.LastExitCode)
			}

			score := 0.9
			if level == EvidenceLevelCorroborating {
				score = 0.7
			}

			evTimestamp := time.Time{}
			if dataTimestamp != nil {
				evTimestamp = *dataTimestamp
			}

			cand.Evidence = append(cand.Evidence, Evidence{
				ID:           evidenceID,
				Type:         EvidenceTypePodStatus,
				Level:        level,
				Source:       "kubernetes",
				SourceType:   "provider",
				Timestamp:    evTimestamp,
				ResourceType: "pod",
				ResourceName: ctx.PodResourceState.Pod,
				Namespace:    ctx.PodResourceState.Namespace,
				Description:  description,
				Score:        score,
				FetchedAt:          &fetchedAt,
				DataTimestamp:      dataTimestamp,
				TimestampAvailable: timestampAvailable,
				TrustStatus:        "fresh",
				CausalRelevance:    causalRelevance,
			})
		}
	}

	// 从 Alert 生成候选（Context Evidence，禁止单独用于确认 Root Cause）。
	for _, a := range ctx.Alerts {
		// 优先用 Pod，然后 Node，然后 Service。
		resType := "service"
		resName := a.Service
		if a.Pod != "" {
			resType = "pod"
			resName = a.Pod
		} else if a.Node != "" {
			resType = "node"
			resName = a.Node
		}
		if resName == "" {
			continue
		}
		key := resType + "/" + resName
		cand, ok := candidateMap[key]
		if !ok {
			cand = &RootCauseCandidate{
				ResourceType: resType,
				ResourceName: resName,
				Namespace:    a.Namespace,
			}
			candidateMap[key] = cand
		}
		score := 0.5
		if a.Severity == "critical" {
			score = 0.9
		} else if a.Severity == "warning" {
			score = 0.7
		}
		cand.Evidence = append(cand.Evidence, Evidence{
			ID:           fmt.Sprintf("alert-%s", a.Fingerprint),
			Type:         EvidenceTypeAlert,
			Level:        EvidenceLevelContext, // P1-X.10: Alert 仅作背景，禁止单独确认 Root Cause
			Source:       "alertmanager",
			SourceType:   "provider",
			Timestamp:    a.StartsAt,
			ResourceType: resType,
			ResourceName: resName,
			Namespace:    a.Namespace,
			Severity:     a.Severity,
			Description:  fmt.Sprintf("%s (%s)", a.Alertname, a.Severity),
			Score:        score,
			// P1-X.10 Phase 4: Evidence 级 provenance
			FetchedAt:          &fetchedAt,
			DataTimestamp:      &a.StartsAt,
			TimestampAvailable: true,
			TrustStatus:        "fresh",
			CausalRelevance:    "contextual",
		})
	}

	// 转换为切片。
	candidates := make([]RootCauseCandidate, 0, len(candidateMap))
	for _, c := range candidateMap {
		// 生成根因描述。
		c.RootCause = p.describeRootCause(*c)
		candidates = append(candidates, *c)
	}

	return candidates
}

// classifyEventEvidenceLevel 根据 K8s Event Reason 和 Message 判定 Evidence Level。
// P1-X.10 Phase 5: 某些 K8s Event 的 Reason 为通用 "Failed"，具体错误在 Message 中（如 "Error: ErrImagePull"）。
func classifyEventEvidenceLevel(reason, message string) EvidenceLevel {
	directReasons := map[string]bool{
		"OOMKilled":            true,
		"FailedMount":          true,
		"FailedAttachVolume":   true,
		"ImagePullBackOff":     true,
		"ErrImagePull":         true,
		"FailedCreatePodSandBox": true,
		"NetworkNotReady":      true,
		"InvalidDiskCapacity":  true,
	}
	corroboratingReasons := map[string]bool{
		"CrashLoopBackOff": true,
		"BackOff":          true,
		"Unhealthy":        true,
		"Killing":          true,
		"FailedScheduling": true,
		"Preempting":       true,
		"Evicted":          true,
	}
	// 直接匹配 Reason。
	if directReasons[reason] {
		return EvidenceLevelDirect
	}
	if corroboratingReasons[reason] {
		return EvidenceLevelCorroborating
	}
	// P1-X.10 Phase 5: Reason 为通用 "Failed" 时，从 Message 中提取具体错误。
	// 例如 K8s Event: Reason="Failed", Message="Error: ErrImagePull"
	if reason == "Failed" || reason == "" {
		for directReason := range directReasons {
			if strings.Contains(message, directReason) {
				return EvidenceLevelDirect
			}
		}
		for corrReason := range corroboratingReasons {
			if strings.Contains(message, corrReason) {
				return EvidenceLevelCorroborating
			}
		}
	}
	return EvidenceLevelContext
}

// classifyPodStatusEvidenceLevel 根据 Pod container lastState.terminated.reason 判定 Evidence Level。
// P1-X.10 Phase 6: OOMKilled 等直接终止原因属于 direct_causal evidence。
func classifyPodStatusEvidenceLevel(reason string) EvidenceLevel {
	directReasons := map[string]bool{
		"OOMKilled":         true,
		"Error":             true,
		"ContainerCannotRun": true,
		"StartError":        true,
		"DeadlineExceeded":  true,
	}
	corroboratingReasons := map[string]bool{
		"CrashLoopBackOff": true,
	}
	if directReasons[reason] {
		return EvidenceLevelDirect
	}
	if corroboratingReasons[reason] {
		return EvidenceLevelCorroborating
	}
	return EvidenceLevelContext
}

// describeRootCause 根据证据生成根因描述。
func (p *Pipeline) describeRootCause(c RootCauseCandidate) string {
	// 检查是否有 OOMKilled 事件。
	for _, e := range c.Evidence {
		if e.Type == EvidenceTypeEvent {
			if strings.Contains(e.Description, "OOMKilled") {
				return fmt.Sprintf("%s %s 内存溢出（OOMKilled）", c.ResourceType, c.ResourceName)
			}
			if strings.Contains(e.Description, "CrashLoopBackOff") {
				return fmt.Sprintf("%s %s 频繁崩溃重启（CrashLoopBackOff）", c.ResourceType, c.ResourceName)
			}
			if strings.Contains(e.Description, "ImagePull") {
				return fmt.Sprintf("%s %s 镜像拉取失败", c.ResourceType, c.ResourceName)
			}
			if strings.Contains(e.Description, "FailedScheduling") {
				return fmt.Sprintf("%s %s 调度失败", c.ResourceType, c.ResourceName)
			}
		}
	}

	// P1-X.10 Phase 6: 检查 Pod Status Evidence（OOMKilled 等 lastState.terminated.reason）。
	for _, e := range c.Evidence {
		if e.Type == EvidenceTypePodStatus {
			if strings.Contains(e.Description, "OOMKilled") {
				return fmt.Sprintf("%s %s 容器因内存溢出被终止（OOMKilled）", c.ResourceType, c.ResourceName)
			}
			if strings.Contains(e.Description, "Error") {
				return fmt.Sprintf("%s %s 容器异常终止", c.ResourceType, c.ResourceName)
			}
		}
	}

	// 检查异常类型。
	for _, e := range c.Evidence {
		if e.Type == EvidenceTypeAnomaly {
			metric := strings.ToLower(e.Metric)
			if strings.Contains(metric, "cpu") {
				return fmt.Sprintf("%s %s CPU 使用率异常升高", c.ResourceType, c.ResourceName)
			}
			if strings.Contains(metric, "memory") || strings.Contains(metric, "mem") {
				return fmt.Sprintf("%s %s 内存使用率异常升高", c.ResourceType, c.ResourceName)
			}
			if strings.Contains(metric, "restart") {
				return fmt.Sprintf("%s %s 频繁重启", c.ResourceType, c.ResourceName)
			}
		}
	}

	// 检查告警类型。
	for _, e := range c.Evidence {
		if e.Type == EvidenceTypeAlert {
			desc := strings.ToLower(e.Description)
			if strings.Contains(desc, "cpu") {
				return fmt.Sprintf("%s %s CPU 资源压力", c.ResourceType, c.ResourceName)
			}
			if strings.Contains(desc, "memory") || strings.Contains(desc, "mem") {
				return fmt.Sprintf("%s %s 内存资源压力", c.ResourceType, c.ResourceName)
			}
			if strings.Contains(desc, "restart") || strings.Contains(desc, "crash") {
				return fmt.Sprintf("%s %s 频繁重启/崩溃", c.ResourceType, c.ResourceName)
			}
		}
	}

	return fmt.Sprintf("%s %s 发生异常", c.ResourceType, c.ResourceName)
}

// generateExplanation 生成单个候选的解释。
func (p *Pipeline) generateExplanation(c RootCauseCandidate) string {
	parts := []string{}
	for _, e := range c.Evidence {
		parts = append(parts, fmt.Sprintf("[%s] %s", e.Type, e.Description))
	}
	return strings.Join(parts, "; ")
}

// generateOverallExplanation 生成整体解释。
func (p *Pipeline) generateOverallExplanation(top RootCauseCandidate, candidates []RootCauseCandidate, ctx IncidentContext) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("基于 %d 条证据（%d 告警, %d 异常, %d 事件）分析，",
		len(ctx.Alerts)+len(ctx.Anomalies)+len(ctx.Events),
		len(ctx.Alerts), len(ctx.Anomalies), len(ctx.Events)))
	sb.WriteString(fmt.Sprintf("最可能的根因是 %s（置信度 %.0f%%）。", top.RootCause, top.Confidence*100))
	if len(candidates) > 1 {
		sb.WriteString(fmt.Sprintf("其他候选：%s（%.0f%%）。",
			candidates[1].RootCause, candidates[1].Confidence*100))
	}
	return sb.String()
}

// buildTimeline 构建统一时间线。
func (p *Pipeline) buildTimeline(ctx IncidentContext) []TimelineItem {
	var items []TimelineItem

	for _, a := range ctx.Alerts {
		items = append(items, TimelineItem{
			Timestamp:   a.StartsAt,
			Type:        "alert",
			Description: fmt.Sprintf("%s: %s", a.Severity, a.Alertname),
			Severity:    a.Severity,
			Resource:    a.Pod,
		})
	}
	for _, a := range ctx.Anomalies {
		items = append(items, TimelineItem{
			Timestamp:   a.Timestamp,
			Type:        "anomaly",
			Description: fmt.Sprintf("%s: %.2f (score=%.2f)", a.Metric, a.Value, a.AnomalyScore),
			Severity:    a.Severity,
			Resource:    a.ResourceName,
		})
	}
	for _, e := range ctx.Events {
		items = append(items, TimelineItem{
			Timestamp:   e.Timestamp,
			Type:        "event",
			Description: fmt.Sprintf("%s: %s", e.Reason, e.Message),
			Severity:    strings.ToLower(e.Type),
			Resource:    e.ResourceName,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Timestamp.Before(items[j].Timestamp)
	})

	return items
}

// determineRootCauseStatus 根据 Evidence Level 判定根因确认状态（P1-X.10 Evidence-First）。
// determineRootCauseStatus 根据 Evidence Level 和 Causal Relevance 判定根因确认状态。
// P1-X.10 Phase 4: 不再机械要求 2 条 Direct Evidence，而是基于 Causal Relevance。
// 存在明确 Direct Causal Evidence（如 OOMKilled）且无矛盾证据即可 confirmed。
func determineRootCauseStatus(candidate RootCauseCandidate) RootCauseStatus {
	directCausalCount := 0
	directCount := 0
	corroboratingCount := 0
	contradictoryCount := 0
	errorEvidenceCount := 0

	for _, e := range candidate.Evidence {
		if e.TrustStatus == "error" {
			errorEvidenceCount++
			continue
		}
		if e.CausalRelevance == "contradictory" {
			contradictoryCount++
		}
		switch e.Level {
		case EvidenceLevelDirect:
			directCount++
			if e.CausalRelevance == "direct_causal" {
				directCausalCount++
			}
		case EvidenceLevelCorroborating:
			corroboratingCount++
		}
	}

	// 存在明确 Direct Causal Evidence + 无矛盾证据 + 无 error evidence → Confirmed。
	// P1-X.10 Phase 4: OOMKilled 等明确因果证据单条即可 confirmed。
	if directCausalCount >= 1 && contradictoryCount == 0 && errorEvidenceCount == 0 {
		// 如果有 Corroborating 佐证，更加确定。
		return RootCauseStatusConfirmed
	}

	// 有 Direct Evidence 但 Causal relevance 不明确（如 ImagePullBackOff 需确认是否解释当前 Incident）。
	if directCount >= 1 && directCausalCount == 0 {
		if corroboratingCount >= 1 {
			return RootCauseStatusProbable
		}
		return RootCauseStatusHypothesis
	}

	// 有 1 条 Direct Causal 但存在矛盾证据 → Probable（需进一步确认）。
	if directCausalCount >= 1 && contradictoryCount > 0 {
		return RootCauseStatusProbable
	}

	// 有 Corroborating 但无 Direct → Hypothesis。
	if corroboratingCount > 0 {
		return RootCauseStatusHypothesis
	}

	// 只有 Context（Alert/Metric）→ Unknown。
	return RootCauseStatusUnknown
}

// evaluateEvidenceSufficiency 评估证据充足性（P1-X.10 Sufficiency Gate）。
func evaluateEvidenceSufficiency(candidate RootCauseCandidate) EvidenceSufficiency {
	directCount := 0
	corroboratingCount := 0
	contextCount := 0
	for _, e := range candidate.Evidence {
		switch e.Level {
		case EvidenceLevelDirect:
			directCount++
		case EvidenceLevelCorroborating:
			corroboratingCount++
		default:
			contextCount++
		}
	}

	suff := EvidenceSufficiency{
		DirectEvidenceCount: directCount,
		CorroboratingCount:  corroboratingCount,
		ContextCount:         contextCount,
	}

	// 判定 missing evidence。
	missing := []string{}
	if directCount == 0 {
		missing = append(missing, "direct_evidence (OOMKilled/exit_code/previous_logs/PVC_mount_failed)")
	}
	if corroboratingCount == 0 && directCount < 2 {
		missing = append(missing, "corroborating_evidence (CrashLoopBackOff/restart_count/ready_status)")
	}
	if len(candidate.Evidence) < 3 {
		missing = append(missing, "sufficient_evidence_volume (minimum 3 evidence items)")
	}
	suff.MissingEvidence = missing

	// Confidence cap。
	if directCount == 0 && corroboratingCount == 0 {
		suff.ConfidenceCap = 0.40
		suff.ConfidenceCapReason = "仅 Context 级证据，上限 0.40"
	} else if directCount == 0 {
		suff.ConfidenceCap = 0.60
		suff.ConfidenceCapReason = "无 Direct 证据，上限 0.60"
	} else if directCount >= 2 {
		suff.ConfidenceCap = 0.95
		suff.ConfidenceCapReason = "2+ Direct 证据，允许高置信度"
	} else {
		suff.ConfidenceCap = 0.85
		suff.ConfidenceCapReason = "1 条 Direct 证据，上限 0.85"
	}

	suff.Sufficient = directCount >= 2
	return suff
}

// buildPossibleCauses 从候选列表构建 PossibleCauses（P1-X.10，证据不足时输出）。
func buildPossibleCauses(candidates []RootCauseCandidate) []PossibleCause {
	result := make([]PossibleCause, 0, len(candidates))
	for _, c := range candidates {
		evidenceIDs := make([]string, 0, len(c.Evidence))
		for _, e := range c.Evidence {
			evidenceIDs = append(evidenceIDs, e.ID)
		}
		result = append(result, PossibleCause{
			Cause:       c.RootCause,
			Status:      c.Status,
			Confidence:  c.Confidence,
			EvidenceIDs: evidenceIDs,
		})
	}
	return result
}

// generateRecommendations 生成建议操作（P1-X.10 Safety Gate）。
// root_cause_status != confirmed 时，Remediation 操作 allowed=false。
func (p *Pipeline) generateRecommendations(top RootCauseCandidate, suff EvidenceSufficiency) []Recommendation {
	recs := []Recommendation{}
	recID := 1

	// Investigation 操作（始终允许）。
	if suff.DirectEvidenceCount == 0 {
		recs = append(recs, Recommendation{
			ID:     fmt.Sprintf("rec-%d", recID),
			Title:  "收集 Pod previous container logs",
			Type:   RecommendationTypeInvestigation,
			Action: "collect_previous_logs",
			Reason: "当前无 Direct Evidence，需要收集 previous logs 确认崩溃原因",
			Risk:   "low",
			Allowed: true,
		})
		recID++
	}

	if top.ResourceType == "pod" || top.ResourceType == "deployment" {
		recs = append(recs, Recommendation{
			ID:     fmt.Sprintf("rec-%d", recID),
			Title:  "描述 Pod 状态和 LastState",
			Type:   RecommendationTypeInvestigation,
			Action: "describe_pod",
			Reason: "检查 Pod LastState、ExitCode、OOMKilled 等 Direct Evidence",
			Risk:   "low",
			Allowed: true,
		})
		recID++
	}

	recs = append(recs, Recommendation{
		ID:     fmt.Sprintf("rec-%d", recID),
		Title:  "收集 Kubernetes Events",
		Type:   RecommendationTypeInvestigation,
		Action: "collect_events",
		Reason: "收集 OOMKilled、FailedMount、ImagePullBackOff 等事件",
		Risk:   "low",
		Allowed: true,
	})
	recID++

	// Verification 操作（始终允许）。
	recs = append(recs, Recommendation{
		ID:     fmt.Sprintf("rec-%d", recID),
		Title:  "验证 Pod Ready 状态",
		Type:   RecommendationTypeVerification,
		Action: "verify_pod_ready",
		Reason: "确认当前 Pod 是否处于 Ready 状态",
		Risk:   "low",
		Allowed: true,
	})
	recID++

	// P1-X.10 Phase 4: Remediation Safety Gate 加强
	// 只有 confirmed + fresh evidence + no contradictory + no error evidence 才允许 remediation
	remediationAllowed := top.Status == RootCauseStatusConfirmed
	// P1-X.10 Phase 8: 防御性检查 — confirmed 但无 evidence 或空 root_cause 必须 blocked
	if remediationAllowed && (top.RootCause == "" || len(top.Evidence) == 0) {
		remediationAllowed = false
	}
	if remediationAllowed {
		for _, e := range top.Evidence {
			if e.TrustStatus == "error" || e.TrustStatus == "stale" {
				remediationAllowed = false
				break
			}
			if e.CausalRelevance == "contradictory" {
				remediationAllowed = false
				break
			}
		}
	}

	if top.ResourceType == "pod" || top.ResourceType == "deployment" {
		recs = append(recs, Recommendation{
			ID:     fmt.Sprintf("rec-%d", recID),
			Title:  "重启 Pod",
			Type:   RecommendationTypeRemediation,
			Action: "restart_pod",
			Reason: "如果确认是临时故障，重启 Pod 可能恢复服务",
			Risk:   "medium",
			Allowed: remediationAllowed,
		})
		recID++
	}

	return recs
}
