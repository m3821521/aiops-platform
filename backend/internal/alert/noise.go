package alert

import (
	"context"
	"fmt"
	"time"
)

// 默认降噪参数。
const (
	defaultNoiseWindow    = 5 * time.Minute // 只看最近 5 分钟的告警
	defaultStormThreshold = 10              // 单组超过 10 条视为风暴
	defaultTotalStorm     = 30              // 总数超过 30 条视为全局风暴
)

// NoiseConfig 降噪配置。
type NoiseConfig struct {
	Window         time.Duration // 时间窗口
	StormThreshold int           // 单组风暴阈值
	TotalStorm     int           // 全局风暴阈值
}

// DefaultNoiseConfig 返回默认降噪配置。
func DefaultNoiseConfig() NoiseConfig {
	return NoiseConfig{
		Window:         defaultNoiseWindow,
		StormThreshold: defaultStormThreshold,
		TotalStorm:     defaultTotalStorm,
	}
}

// NoiseResult 降噪后的结果。
type NoiseResult struct {
	Groups      []AlertGroup `json:"groups"`       // 降噪后的告警组
	TotalBefore int          `json:"total_before"` // 降噪前告警总数
	TotalAfter  int          `json:"total_after"`  // 降噪后告警总数
	IsStorm     bool         `json:"is_storm"`     // 是否检测到告警风暴
	StormReason string       `json:"storm_reason,omitempty"`
}

// NoiseReducer 告警降噪引擎。
type NoiseReducer struct {
	repo   *Repository
	config NoiseConfig
}

// NewNoiseReducer 创建降噪引擎，使用默认配置。
func NewNoiseReducer(repo *Repository) *NoiseReducer {
	return &NoiseReducer{repo: repo, config: DefaultNoiseConfig()}
}

// NewNoiseReducerWithConfig 创建降噪引擎，使用自定义配置。
func NewNoiseReducerWithConfig(repo *Repository, config NoiseConfig) *NoiseReducer {
	if config.Window <= 0 {
		config.Window = defaultNoiseWindow
	}
	if config.StormThreshold <= 0 {
		config.StormThreshold = defaultStormThreshold
	}
	if config.TotalStorm <= 0 {
		config.TotalStorm = defaultTotalStorm
	}
	return &NoiseReducer{repo: repo, config: config}
}

// Reduce 查询最近时间窗口内的 firing 告警，按维度分组并去重，同时检测风暴。
// dimension 取值：service / node / namespace。
func (n *NoiseReducer) Reduce(ctx context.Context, dimension string) (*NoiseResult, error) {
	if dimension == "" {
		dimension = DimByService
	}

	// 查询所有 firing 告警（不分页，降噪需要全量）。
	alerts, _, err := n.repo.List(ctx, ListFilter{Status: StatusFiring}, 1, 10000)
	if err != nil {
		return nil, err
	}

	totalBefore := len(alerts)

	// 按时间窗口过滤：只保留窗口内的告警。
	cutoff := time.Now().Add(-n.config.Window)
	var windowed []Alert
	for _, a := range alerts {
		if a.StartsAt.After(cutoff) || a.StartsAt.Equal(cutoff) {
			windowed = append(windowed, a)
		}
	}

	// 按维度分组，组内按 fingerprint 去重（保留最新一条）。
	groupsMap := make(map[string]*AlertGroup)
	fingerprintSet := make(map[string]bool)
	totalAfter := 0

	for i := range windowed {
		a := windowed[i]
		key := groupKey(dimension, &a)
		if key == "" {
			key = "ungrouped"
		}

		// 全局 fingerprint 去重。
		if fingerprintSet[a.Fingerprint] {
			continue
		}
		fingerprintSet[a.Fingerprint] = true
		totalAfter++

		g, ok := groupsMap[key]
		if !ok {
			g = &AlertGroup{
				Key:       key,
				Dimension: dimension,
				FirstSeen: a.StartsAt,
				LastSeen:  a.StartsAt,
			}
			groupsMap[key] = g
		}
		g.Count++
		g.Alerts = append(g.Alerts, a)
		g.Severity = higherSeverity(g.Severity, a.Severity)
		if !containsStr(g.Alertnames, a.Alertname) {
			g.Alertnames = append(g.Alertnames, a.Alertname)
		}
		if a.StartsAt.Before(g.FirstSeen) {
			g.FirstSeen = a.StartsAt
		}
		if a.StartsAt.After(g.LastSeen) {
			g.LastSeen = a.StartsAt
		}
	}

	// 转为切片，按数量倒序。
	groups := make([]AlertGroup, 0, len(groupsMap))
	for _, g := range groupsMap {
		groups = append(groups, *g)
	}
	// 按数量倒序排序（复用 aggregate.go 中的排序逻辑，这里内联）。
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			if groups[j].Count > groups[i].Count {
				groups[i], groups[j] = groups[j], groups[i]
			}
		}
	}

	result := &NoiseResult{
		Groups:      groups,
		TotalBefore: totalBefore,
		TotalAfter:  totalAfter,
	}

	// 风暴检测。
	if totalAfter >= n.config.TotalStorm {
		result.IsStorm = true
		result.StormReason = fmt.Sprintf("全局告警风暴：%d 条告警超过阈值 %d", totalAfter, n.config.TotalStorm)
	} else {
		for _, g := range groups {
			if g.Count >= n.config.StormThreshold {
				result.IsStorm = true
				result.StormReason = fmt.Sprintf("组 %s 告警风暴：%d 条超过阈值 %d", g.Key, g.Count, n.config.StormThreshold)
				break
			}
		}
	}

	return result, nil
}
