package rca

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Engine 是根因分析引擎（规则引擎版）。
// 第一版不做复杂 AI，基于告警统计和时间关联推断根因。
type Engine struct{}

// NewEngine 创建 RCA 引擎。
func NewEngine() *Engine {
	return &Engine{}
}

// Analyze 对一组告警进行根因分析。
func (e *Engine) Analyze(alerts []AlertInfo) *Result {
	if len(alerts) == 0 {
		return &Result{
			RootCause:  "无告警",
			Confidence: 0,
			AnalyzedAt: time.Now(),
		}
	}

	// 1. 按服务分组统计。
	services := groupByService(alerts)

	// 2. 按异常分数排序（critical 权重 3，warning 权重 1）。
	sort.Slice(services, func(i, j int) bool {
		return scoreService(services[i]) > scoreService(services[j])
	})

	// 3. 分数最高的服务视为根因候选。
	root := services[0]

	// 4. 构建受影响服务列表。
	affected := make([]string, 0, len(services))
	for _, s := range services {
		if s.Name != "" {
			affected = append(affected, s.Namespace+"/"+s.Name)
		}
	}

	// 5. 构建证据链。
	evidence := buildEvidence(root, services, alerts)

	// 6. 构建时间线。
	timeline := buildTimeline(alerts)

	// 7. 计算置信度。
	confidence := calcConfidence(root, alerts)

	// 8. 生成根因描述。
	rootCause := describeRootCause(root)

	return &Result{
		RootCause:        rootCause,
		Confidence:       confidence,
		AffectedServices: affected,
		Evidence:         evidence,
		Timeline:         timeline,
		AnalyzedAt:       time.Now(),
	}
}

// groupByService 按服务分组告警。
func groupByService(alerts []AlertInfo) []serviceStats {
	m := make(map[string]*serviceStats)
	for _, a := range alerts {
		key := a.Namespace + "/" + a.Service
		if a.Service == "" {
			key = "_unknown_"
		}
		s, ok := m[key]
		if !ok {
			s = &serviceStats{
				Name:      a.Service,
				Namespace: a.Namespace,
				FirstSeen: a.StartsAt,
				LastSeen:  a.StartsAt,
			}
			m[key] = s
		}
		s.Count++
		if a.Severity == SeverityCritical {
			s.Critical++
		} else if a.Severity == SeverityWarning {
			s.Warning++
		}
		s.Alerts = append(s.Alerts, a)
		if a.StartsAt.Before(s.FirstSeen) {
			s.FirstSeen = a.StartsAt
		}
		if a.StartsAt.After(s.LastSeen) {
			s.LastSeen = a.StartsAt
		}
	}

	result := make([]serviceStats, 0, len(m))
	for _, s := range m {
		result = append(result, *s)
	}
	return result
}

// scoreService 计算服务的异常分数。
func scoreService(s serviceStats) int {
	return s.Critical*3 + s.Warning*1 + s.Count
}

// buildEvidence 构建证据链。
func buildEvidence(root serviceStats, all []serviceStats, alerts []AlertInfo) []Evidence {
	var evidence []Evidence
	order := 1

	// 证据 1：根因服务告警统计。
	evidence = append(evidence, Evidence{
		Order: order,
		Description: fmt.Sprintf("%s/%s 有 %d 条告警（%d critical, %d warning），是告警最集中的服务",
			root.Namespace, root.Name, root.Count, root.Critical, root.Warning),
		Timestamp: root.FirstSeen,
		Severity:  SeverityCritical,
	})
	order++

	// 证据 2：根因服务最早告警时间。
	evidence = append(evidence, Evidence{
		Order: order,
		Description: fmt.Sprintf("%s 最早告警时间 %s，可能是故障起点",
			root.Name, root.FirstSeen.Format("15:04:05")),
		Timestamp: root.FirstSeen,
	})
	order++

	// 证据 3：受影响的其他服务。
	otherServices := make([]string, 0)
	for _, s := range all {
		if s.Name != root.Name && s.Name != "" {
			otherServices = append(otherServices, fmt.Sprintf("%s/%s(%d条)", s.Namespace, s.Name, s.Count))
		}
	}
	if len(otherServices) > 0 {
		evidence = append(evidence, Evidence{
			Order:       order,
			Description: fmt.Sprintf("其他受影响服务: %s", strings.Join(otherServices, ", ")),
		})
		order++
	}

	// 证据 4：根因服务的具体告警名称。
	alertNames := make([]string, 0)
	for _, a := range root.Alerts {
		if !containsStr(alertNames, a.Alertname) {
			alertNames = append(alertNames, a.Alertname)
		}
	}
	if len(alertNames) > 0 {
		evidence = append(evidence, Evidence{
			Order:       order,
			Description: fmt.Sprintf("根因服务告警类型: %s", strings.Join(alertNames, ", ")),
		})
	}

	return evidence
}

// buildTimeline 构建告警时间线。
func buildTimeline(alerts []AlertInfo) []TimelineEvent {
	sorted := make([]AlertInfo, len(alerts))
	copy(sorted, alerts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartsAt.Before(sorted[j].StartsAt)
	})

	timeline := make([]TimelineEvent, 0, len(sorted))
	for _, a := range sorted {
		timeline = append(timeline, TimelineEvent{
			Time:        a.StartsAt,
			Service:     a.Service,
			Alertname:   a.Alertname,
			Severity:    a.Severity,
			Description: fmt.Sprintf("%s/%s - %s", a.Namespace, a.Service, a.Alertname),
		})
	}
	return timeline
}

// calcConfidence 计算置信度。
func calcConfidence(root serviceStats, alerts []AlertInfo) float64 {
	if len(alerts) == 0 {
		return 0
	}
	// 根因服务告警占比。
	ratio := float64(root.Count) / float64(len(alerts))
	// 有 critical 告警加分。
	criticalBonus := 0.0
	if root.Critical > 0 {
		criticalBonus = 0.2
	}
	confidence := ratio*0.7 + criticalBonus
	if confidence > 1.0 {
		confidence = 1.0
	}
	return confidence
}

// describeRootCause 生成根因描述。
func describeRootCause(root serviceStats) string {
	if root.Name == "" {
		return "无法确定根因服务（告警缺少 service 标签）"
	}
	// 根据告警名称推断根因类型。
	for _, a := range root.Alerts {
		name := strings.ToLower(a.Alertname)
		if strings.Contains(name, "cpu") {
			return fmt.Sprintf("%s 服务 CPU 资源耗尽", root.Name)
		}
		if strings.Contains(name, "memory") || strings.Contains(name, "mem") {
			return fmt.Sprintf("%s 服务内存资源耗尽", root.Name)
		}
		if strings.Contains(name, "disk") {
			return fmt.Sprintf("%s 服务磁盘空间不足", root.Name)
		}
		if strings.Contains(name, "restart") || strings.Contains(name, "crash") {
			return fmt.Sprintf("%s 服务频繁重启/崩溃", root.Name)
		}
		if strings.Contains(name, "error") || strings.Contains(name, "5xx") {
			return fmt.Sprintf("%s 服务错误率升高", root.Name)
		}
		if strings.Contains(name, "latency") || strings.Contains(name, "slow") {
			return fmt.Sprintf("%s 服务响应延迟升高", root.Name)
		}
	}
	return fmt.Sprintf("%s 服务发生异常（%d 条告警）", root.Name, root.Count)
}

func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
