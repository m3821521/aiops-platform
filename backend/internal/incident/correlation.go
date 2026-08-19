package incident

import (
	"math"
	"time"
)

// Correlator 是事件关联引擎。
// 基于可解释的规则评分，不使用机器学习。
type Correlator struct {
	cfg      CorrelationConfig
	topology TopologyCorrelationProvider // 可选，P2-3 接入
}

// NewCorrelator 创建关联引擎，使用默认配置。
func NewCorrelator(cfg CorrelationConfig) *Correlator {
	return &Correlator{cfg: cfg}
}

// SetTopologyProvider 设置拓扑关联提供者（可选）。
func (c *Correlator) SetTopologyProvider(p TopologyCorrelationProvider) {
	c.topology = p
}

// Score 计算新信号与现有 Incident 的关联分数。
// 返回各维度明细和总分（0.0~1.0）。
func (c *Correlator) Score(newSig Signal, inc *Incident) CorrelationScore {
	score := CorrelationScore{}

	// 1. 时间关联：信号时间在 Incident 时间窗口内。
	score.TimeScore = c.timeScore(newSig, inc)

	// 2. 资源关联：比较信号与 Incident 的资源。
	score.ResourceScore = c.resourceScore(newSig, inc)

	// 3. Namespace 关联。
	score.NamespaceScore = c.namespaceScore(newSig, inc)

	// 4. 标签关联。
	score.LabelScore = c.labelScore(newSig, inc)

	// 5. 拓扑关联（可选）。
	score.TopologyScore = c.topologyScore(newSig, inc)

	// 加权总分。
	total := score.TimeScore*c.cfg.WeightTime +
		score.ResourceScore*c.cfg.WeightResource +
		score.NamespaceScore*c.cfg.WeightNamespace +
		score.LabelScore*c.cfg.WeightLabel +
		score.TopologyScore*c.cfg.WeightTopology
	score.Total = math.Round(total*100) / 100
	return score
}

// Matches 判断信号是否与 Incident 关联（分数 >= 阈值）。
func (c *Correlator) Matches(newSig Signal, inc *Incident) bool {
	return c.Score(newSig, inc).Total >= c.cfg.ScoreThreshold
}

// timeScore 时间关联评分。
// 信号时间在 Incident.start_time ± time_window 内得满分，超出按距离衰减。
func (c *Correlator) timeScore(sig Signal, inc *Incident) float64 {
	window := c.cfg.TimeWindow
	if window <= 0 {
		window = 5 * time.Minute
	}
	// 以 Incident 开始时间为基准，也考虑 Incident 最后信号时间。
	refTime := inc.StartTime
	if inc.EndTime != nil {
		// 已结束的 Incident 不关联新信号。
		return 0
	}
	diff := sig.Timestamp.Sub(refTime)
	if diff < 0 {
		diff = -diff
	}
	if diff <= window {
		return 1.0
	}
	// 超出窗口后线性衰减到 2 倍窗口处为 0。
	if diff <= 2*window {
		return 1.0 - float64(diff-window)/float64(window)
	}
	return 0
}

// resourceScore 资源关联评分。
// 同 Pod/Deployment/Service/Node 分别加分。
func (c *Correlator) resourceScore(sig Signal, inc *Incident) float64 {
	if sig.ResourceName == "" || inc.ResourceName == "" {
		// 资源为空时，用 service 作为降级匹配。
		if sig.Service != "" && sig.Service == inc.Service {
			return c.cfg.ScoreSameService
		}
		return 0
	}
	if string(sig.ResourceType) == inc.ResourceType && sig.ResourceName == inc.ResourceName {
		switch ResourceType(inc.ResourceType) {
		case ResourcePod:
			return c.cfg.ScoreSamePod
		case ResourceDeployment:
			return c.cfg.ScoreSameDeployment
		case ResourceService:
			return c.cfg.ScoreSameService
		case ResourceNode:
			return c.cfg.ScoreSameNode
		}
		return 0.8
	}
	// 不同资源类型但同名（如 pod 和 deployment 同名）给中等分。
	if sig.ResourceName == inc.ResourceName {
		return 0.4
	}
	// service 降级匹配。
	if sig.Service != "" && sig.Service == inc.Service {
		return c.cfg.ScoreSameService * 0.7
	}
	return 0
}

// namespaceScore 命名空间关联评分。
func (c *Correlator) namespaceScore(sig Signal, inc *Incident) float64 {
	if sig.Namespace != "" && sig.Namespace == inc.Namespace {
		return c.cfg.ScoreSameNamespace
	}
	return 0
}

// labelScore 标签关联评分。
// 比较 app/service/component 等关键标签。
func (c *Correlator) labelScore(sig Signal, inc *Incident) float64 {
	if len(sig.Labels) == 0 {
		return 0
	}
	// Incident 本身没有 labels 字段，从 signals 中提取。
	// 简化：用 service 标签匹配。
	// 未来可从 Incident.Signals 聚合标签。
	keyLabels := []string{"app", "service", "component", "app.kubernetes.io/name", "app.kubernetes.io/component"}
	matches := 0
	for _, key := range keyLabels {
		if v, ok := sig.Labels[key]; ok && v != "" {
			// 与 Incident 的 service 字段比较。
			if v == inc.Service {
				matches++
			}
		}
	}
	if matches > 0 {
		return math.Min(1.0, float64(matches)*c.cfg.ScoreLabelMatch)
	}
	return 0
}

// topologyScore 拓扑关联评分（P2-1 不实现，预留接口）。
func (c *Correlator) topologyScore(sig Signal, inc *Incident) float64 {
	if c.topology == nil {
		return 0
	}
	if sig.ResourceName == "" || inc.ResourceName == "" {
		return 0
	}
	if c.topology.Related(sig.Cluster, sig.Namespace,
		string(sig.ResourceType), sig.ResourceName,
		inc.ResourceType, inc.ResourceName) {
		return 0.8
	}
	return 0
}

// FindBestMatch 在候选 Incident 中找到关联分数最高的一个。
// 返回 (incident, score)，如果没有匹配返回 nil。
func (c *Correlator) FindBestMatch(sig Signal, candidates []Incident) (*Incident, CorrelationScore) {
	var best *Incident
	var bestScore CorrelationScore
	for i := range candidates {
		score := c.Score(sig, &candidates[i])
		if score.Total >= c.cfg.ScoreThreshold && score.Total > bestScore.Total {
			best = &candidates[i]
			bestScore = score
		}
	}
	return best, bestScore
}
