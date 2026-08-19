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
	}
	candidates = RankCandidates(candidates)

	// 5. 取 Top 候选作为最终根因。
	top := candidates[0]
	result.RootCause = top.RootCause
	result.Confidence = top.Confidence
	result.Candidates = candidates
	result.Evidence = top.Evidence
	result.Impact = top.Impact
	result.Status = RCAStatusCompleted

	// 6. 构建统一时间线。
	result.Timeline = p.buildTimeline(incidentCtx)

	// 7. 生成整体解释。
	result.Explanation = p.generateOverallExplanation(top, candidates, incidentCtx)

	return result, nil
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
	}
	var res result

	if alerts, err := p.collector.CollectAlerts(ctx, incidentID); err == nil {
		res.alerts = alerts
	} else {
		slog.Warn("rca: collect alerts failed", "err", err)
	}

	if anomalies, err := p.collector.CollectAnomalies(ctx, cluster, namespace, resourceType, resourceName, since); err == nil {
		res.anomalies = anomalies
	} else {
		slog.Warn("rca: collect anomalies failed", "err", err)
	}

	if events, err := p.collector.CollectEvents(ctx, cluster, namespace, resourceType, resourceName, since); err == nil {
		res.events = events
	} else {
		slog.Warn("rca: collect events failed", "err", err)
	}

	if metrics, err := p.collector.CollectMetrics(ctx, cluster, namespace, resourceType, resourceName, since, endTime); err == nil {
		res.metrics = metrics
	} else {
		slog.Warn("rca: collect metrics failed", "err", err)
	}

	if resourceType == "pod" {
		if logs, err := p.collector.CollectLogs(ctx, cluster, namespace, resourceName, since, endTime); err == nil {
			res.logs = logs
		} else {
			slog.Warn("rca: collect logs failed", "err", err)
		}
	}

	if topology, err := p.collector.CollectTopology(ctx, cluster, namespace); err == nil {
		res.topology = topology
	} else {
		slog.Warn("rca: collect topology failed", "err", err)
	}

	incidentCtx.Alerts = res.alerts
	incidentCtx.Anomalies = res.anomalies
	incidentCtx.Events = res.events
	incidentCtx.Metrics = res.metrics
	incidentCtx.Logs = res.logs
	incidentCtx.Topology = res.topology

	return incidentCtx, nil
}

// generateCandidates 从上下文中生成候选根因。
func (p *Pipeline) generateCandidates(ctx IncidentContext) []RootCauseCandidate {
	candidateMap := make(map[string]*RootCauseCandidate)

	// 从 Anomaly 生成候选。
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
		cand.Evidence = append(cand.Evidence, Evidence{
			ID:           fmt.Sprintf("anomaly-%d", a.ID),
			Type:         EvidenceTypeAnomaly,
			Source:       "prometheus",
			Timestamp:    a.Timestamp,
			ResourceType: a.ResourceType,
			ResourceName: a.ResourceName,
			Namespace:    a.Namespace,
			Metric:       a.Metric,
			Value:        a.Value,
			Severity:     a.Severity,
			Description:  a.Reason,
			Score:        a.AnomalyScore,
		})
	}

	// 从 K8s Event 生成候选（强证据）。
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
		score := 0.7
		if e.Type == "Warning" {
			score = 0.9
		}
		cand.Evidence = append(cand.Evidence, Evidence{
			ID:           fmt.Sprintf("event-%s-%s", e.Reason, e.ResourceName),
			Type:         EvidenceTypeEvent,
			Source:       "kubernetes",
			Timestamp:    e.Timestamp,
			ResourceType: e.ResourceType,
			ResourceName: e.ResourceName,
			Namespace:    e.Namespace,
			Severity:     strings.ToLower(e.Type),
			Description:  fmt.Sprintf("%s: %s (count=%d)", e.Reason, e.Message, e.Count),
			Score:        score,
		})
	}

	// 从 Alert 生成候选。
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
			Source:       "alertmanager",
			Timestamp:    a.StartsAt,
			ResourceType: resType,
			ResourceName: resName,
			Namespace:    a.Namespace,
			Severity:     a.Severity,
			Description:  fmt.Sprintf("%s (%s)", a.Alertname, a.Severity),
			Score:        score,
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
