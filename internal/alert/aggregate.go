package alert

import (
	"context"
	"sort"
	"time"
)

// 聚合维度。
const (
	DimByService   = "service"
	DimByNode      = "node"
	DimByNamespace = "namespace"
)

// AlertGroup 是聚合后的告警组。
type AlertGroup struct {
	Key        string    `json:"key"`        // 分组键，如 "default/order-service"
	Dimension  string    `json:"dimension"`  // service / node / namespace
	Count      int       `json:"count"`      // 组内告警数量
	Severity   string    `json:"severity"`   // 组内最高级别
	Alertnames []string  `json:"alertnames"` // 去重后的告警名称
	FirstSeen  time.Time `json:"first_seen"` // 最早开始时间
	LastSeen   time.Time `json:"last_seen"`  // 最晚开始时间
	Alerts     []Alert   `json:"alerts"`     // 组内告警详情
}

// Aggregator 告警聚合引擎。
type Aggregator struct {
	repo *Repository
}

// NewAggregator 创建聚合引擎。
func NewAggregator(repo *Repository) *Aggregator {
	return &Aggregator{repo: repo}
}

// Aggregate 查询所有 firing 状态的告警，按指定维度聚合。
// dimension 取值：service / node / namespace，默认按 service。
func (a *Aggregator) Aggregate(ctx context.Context, dimension string) ([]AlertGroup, error) {
	if dimension == "" {
		dimension = DimByService
	}

	// 查询所有 firing 状态的告警（不分页，聚合需要全量数据）。
	alerts, _, err := a.repo.List(ctx, ListFilter{Status: StatusFiring}, 1, 10000)
	if err != nil {
		return nil, err
	}

	groups := make(map[string]*AlertGroup)
	for i := range alerts {
		alert := alerts[i]
		key := groupKey(dimension, &alert)
		if key == "" {
			// 没有对应维度标签的告警归入 "ungrouped"。
			key = "ungrouped"
		}

		g, ok := groups[key]
		if !ok {
			g = &AlertGroup{
				Key:       key,
				Dimension: dimension,
				FirstSeen: alert.StartsAt,
				LastSeen:  alert.StartsAt,
			}
			groups[key] = g
		}

		g.Count++
		g.Alerts = append(g.Alerts, alert)
		g.Severity = higherSeverity(g.Severity, alert.Severity)

		// 记录告警名称（去重）。
		if !containsStr(g.Alertnames, alert.Alertname) {
			g.Alertnames = append(g.Alertnames, alert.Alertname)
		}

		if alert.StartsAt.Before(g.FirstSeen) {
			g.FirstSeen = alert.StartsAt
		}
		if alert.StartsAt.After(g.LastSeen) {
			g.LastSeen = alert.StartsAt
		}
	}

	// 转为切片，按告警数量倒序。
	result := make([]AlertGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, *g)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})
	return result, nil
}

// groupKey 根据维度生成分组键。
func groupKey(dimension string, a *Alert) string {
	switch dimension {
	case DimByService:
		if a.Service == "" {
			return ""
		}
		return a.Namespace + "/" + a.Service
	case DimByNode:
		return a.Node
	case DimByNamespace:
		return a.Namespace
	default:
		return a.Namespace + "/" + a.Service
	}
}

// severityRank 返回级别优先级数值，越大越严重。
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

// higherSeverity 返回两个级别中更严重的那个。
func higherSeverity(a, b string) string {
	if severityRank(a) >= severityRank(b) {
		return a
	}
	return b
}

// containsStr 检查字符串切片是否包含指定字符串。
func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
