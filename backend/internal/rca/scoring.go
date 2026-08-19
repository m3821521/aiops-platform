package rca

import (
	"math"
	"sort"
	"time"
)

// ScoringConfig 是 RCA 评分配置。
type ScoringConfig struct {
	AnomalyWeight   float64
	AlertWeight     float64
	EventWeight     float64
	MetricWeight    float64
	LogWeight       float64
	TopologyWeight  float64
	TemporalWeight  float64
}

// DefaultScoringConfig 返回默认评分配置。
func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		AnomalyWeight:  0.25,
		AlertWeight:    0.20,
		EventWeight:    0.25,
		MetricWeight:   0.10,
		LogWeight:      0.10,
		TopologyWeight: 0.10,
		TemporalWeight: 0.15,
	}
}

// TemporalScore 计算时间相关性分数。
// cause 必须发生在 effect 之前或同时。
// 0-30s: 1.0, 30s-2m: 0.8, 2-5m: 0.5, 5-10m: 0.2, >10m: 0
func TemporalScore(cause, effect time.Time) float64 {
	if cause.After(effect) {
		return 0 // cause 不能在 effect 之后
	}
	delta := effect.Sub(cause)
	switch {
	case delta <= 30*time.Second:
		return 1.0
	case delta <= 2*time.Minute:
		return 0.8
	case delta <= 5*time.Minute:
		return 0.5
	case delta <= 10*time.Minute:
		return 0.2
	default:
		return 0
	}
}

// Scorer 是 RCA 评分引擎。
type Scorer struct {
	config ScoringConfig
}

func NewScorer(config ScoringConfig) *Scorer {
	return &Scorer{config: config}
}

// ScoreCandidate 计算单个候选根因的分数。
func (s *Scorer) ScoreCandidate(ctx IncidentContext, candidate RootCauseCandidate) float64 {
	var total float64

	// 1. Anomaly 分数。
	anomalyScore := s.scoreAnomalies(ctx, candidate)
	total += anomalyScore * s.config.AnomalyWeight

	// 2. Alert 分数。
	alertScore := s.scoreAlerts(ctx, candidate)
	total += alertScore * s.config.AlertWeight

	// 3. Event 分数。
	eventScore := s.scoreEvents(ctx, candidate)
	total += eventScore * s.config.EventWeight

	// 4. Metric 分数。
	metricScore := s.scoreMetrics(ctx, candidate)
	total += metricScore * s.config.MetricWeight

	// 5. Log 分数。
	logScore := s.scoreLogs(ctx, candidate)
	total += logScore * s.config.LogWeight

	// 6. Topology 分数。
	topologyScore := s.scoreTopology(ctx, candidate)
	total += topologyScore * s.config.TopologyWeight

	return math.Min(1.0, total)
}

// scoreAnomalies 计算异常证据分数。
func (s *Scorer) scoreAnomalies(ctx IncidentContext, candidate RootCauseCandidate) float64 {
	var maxScore float64
	for _, a := range ctx.Anomalies {
		if a.ResourceType != candidate.ResourceType || a.ResourceName != candidate.ResourceName {
			continue
		}
		// 异常分数本身 + 时间相关性。
		temporal := TemporalScore(a.Timestamp, ctx.StartTime)
		score := a.AnomalyScore*0.6 + temporal*0.4
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

// scoreAlerts 计算告警证据分数。
func (s *Scorer) scoreAlerts(ctx IncidentContext, candidate RootCauseCandidate) float64 {
	var maxScore float64
	for _, a := range ctx.Alerts {
		// 匹配 Pod 或 Node。
		matched := false
		if candidate.ResourceType == "pod" && a.Pod == candidate.ResourceName {
			matched = true
		} else if candidate.ResourceType == "node" && a.Node == candidate.ResourceName {
			matched = true
		} else if candidate.ResourceType == "service" && a.Service == candidate.ResourceName {
			matched = true
		}
		if !matched {
			continue
		}
		severityScore := 0.5
		if a.Severity == "critical" {
			severityScore = 1.0
		} else if a.Severity == "warning" {
			severityScore = 0.7
		}
		temporal := TemporalScore(a.StartsAt, ctx.StartTime)
		score := severityScore*0.6 + temporal*0.4
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

// scoreEvents 计算 K8s Event 证据分数。
// OOMKilled, CrashLoopBackOff 等属于强证据。
func (s *Scorer) scoreEvents(ctx IncidentContext, candidate RootCauseCandidate) float64 {
	var maxScore float64
	highEvidenceReasons := map[string]float64{
		"OOMKilled":           1.0,
		"CrashLoopBackOff":    0.9,
		"ImagePullBackOff":    0.9,
		"ErrImagePull":        0.9,
		"FailedScheduling":    0.8,
		"FailedMount":         0.7,
		"Unhealthy":           0.7,
		"FailedAttachVolume":  0.7,
		"FailedCreatePodSandBox": 0.7,
		"BackOff":             0.6,
		"Killing":             0.6,
	}
	for _, e := range ctx.Events {
		if e.ResourceType != candidate.ResourceType || e.ResourceName != candidate.ResourceName {
			continue
		}
		baseScore, ok := highEvidenceReasons[e.Reason]
		if !ok {
			baseScore = 0.3
		}
		if e.Type == "Warning" {
			baseScore = math.Min(1.0, baseScore+0.1)
		}
		temporal := TemporalScore(e.Timestamp, ctx.StartTime)
		score := baseScore*0.7 + temporal*0.3
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

// scoreMetrics 计算指标证据分数。
func (s *Scorer) scoreMetrics(ctx IncidentContext, candidate RootCauseCandidate) float64 {
	var maxScore float64
	for _, m := range ctx.Metrics {
		if m.Resource != candidate.ResourceName {
			continue
		}
		temporal := TemporalScore(m.Timestamp, ctx.StartTime)
		score := 0.5 + temporal*0.5
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

// scoreLogs 计算日志证据分数。
func (s *Scorer) scoreLogs(ctx IncidentContext, candidate RootCauseCandidate) float64 {
	var maxScore float64
	highEvidenceKeywords := []string{"OOM", "out of memory", "connection refused", "timeout", "error", "exception", "fatal"}
	for _, l := range ctx.Logs {
		if l.Pod != candidate.ResourceName {
			continue
		}
		keywordScore := 0.3
		for _, kw := range highEvidenceKeywords {
			if containsIgnoreCase(l.Message, kw) {
				keywordScore = 0.8
				break
			}
		}
		temporal := TemporalScore(l.Timestamp, ctx.StartTime)
		score := keywordScore*0.6 + temporal*0.4
		if score > maxScore {
			maxScore = score
		}
	}
	return maxScore
}

// scoreTopology 计算拓扑证据分数。
// 如果候选资源在拓扑中是其他故障资源的上游，则加分。
func (s *Scorer) scoreTopology(ctx IncidentContext, candidate RootCauseCandidate) float64 {
	candidateID := candidate.ResourceType + "/" + candidate.ResourceName
	upstreamCount := 0
	for _, e := range ctx.Topology.Edges {
		// 如果 candidate 是 edge 的 target（被其他资源依赖），则它是上游。
		if e.Target == candidateID || e.Source == candidateID {
			upstreamCount++
		}
	}
	if upstreamCount == 0 {
		return 0
	}
	return math.Min(1.0, float64(upstreamCount)*0.2)
}

// CalculateConfidence 计算整体置信度。
// 基于证据数量、类型多样性、时间一致性、拓扑一致性。
func CalculateConfidence(candidate RootCauseCandidate, evidenceCount int) float64 {
	if evidenceCount == 0 || len(candidate.Evidence) == 0 {
		return 0
	}

	// 证据类型多样性。
	typeSet := make(map[EvidenceType]bool)
	for _, e := range candidate.Evidence {
		typeSet[e.Type] = true
	}
	diversityScore := math.Min(1.0, float64(len(typeSet))/4.0)

	// 证据数量。
	countScore := math.Min(1.0, float64(len(candidate.Evidence))/5.0)

	// 平均证据分数。
	var avgScore float64
	for _, e := range candidate.Evidence {
		avgScore += e.Score
	}
	avgScore /= float64(len(candidate.Evidence))

	// 综合。
	confidence := diversityScore*0.3 + countScore*0.2 + avgScore*0.5
	return math.Min(0.95, confidence)
}

// RankCandidates 按分数排序候选根因。
func RankCandidates(candidates []RootCauseCandidate) []RootCauseCandidate {
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	return candidates
}

// containsIgnoreCase 不区分大小写的字符串包含检查。
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && (indexOfIgnoreCase(s, substr) >= 0)
}

func indexOfIgnoreCase(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			a := s[i+j]
			b := substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
