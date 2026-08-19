package logging

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// LogGroup 是一组相似日志的聚合结果。
type LogGroup struct {
	Template        string    `json:"template"`          // 归一化后的消息模板
	Count           int       `json:"count"`             // 出现次数
	FirstSeen       time.Time `json:"first_seen"`        // 首次出现
	LastSeen        time.Time `json:"last_seen"`         // 最后出现
	Services        []string  `json:"services"`          // 受影响的服务（去重）
	Namespaces      []string  `json:"namespaces"`        // 受影响的命名空间
	SampleMessage   string    `json:"sample_message"`    // 示例原始消息
	IsHighFrequency bool      `json:"is_high_frequency"` // 是否高频错误
	IsNew           bool      `json:"is_new"`            // 是否新异常
}

// AnalyzeResult 是日志分析结果。
type AnalyzeResult struct {
	TotalLogs     int        `json:"total_logs"`
	Groups        []LogGroup `json:"groups"`
	HighFreqCount int        `json:"high_frequency_count"`
	NewCount      int        `json:"new_count"`
	AnalyzedAt    time.Time  `json:"analyzed_at"`
}

// Analyzer 是日志智能分析器。
type Analyzer struct {
	HighFreqThreshold int           // 高频阈值，出现次数超过此值视为高频
	NewWindow         time.Duration // 新异常窗口，首次出现在此窗口内视为新异常
}

// NewAnalyzer 创建日志分析器。
func NewAnalyzer(highFreqThreshold int, newWindow time.Duration) *Analyzer {
	if highFreqThreshold <= 0 {
		highFreqThreshold = 10
	}
	if newWindow <= 0 {
		newWindow = 1 * time.Hour
	}
	return &Analyzer{
		HighFreqThreshold: highFreqThreshold,
		NewWindow:         newWindow,
	}
}

// 归一化正则表达式。
var (
	uuidRegex    = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	ipRegex      = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+(:\d+)?`)
	hexRegex     = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	numberRegex  = regexp.MustCompile(`\b\d+\b`)
	quotedRegex  = regexp.MustCompile(`"[^"]*"`)
	quotedRegex2 = regexp.MustCompile(`'[^']*'`)
)

// NormalizeMessage 将日志消息归一化为模板。
// 去除 UUID、IP、数字、十六进制、引号内容等变量部分。
func NormalizeMessage(msg string) string {
	result := msg
	result = uuidRegex.ReplaceAllString(result, "<uuid>")
	result = ipRegex.ReplaceAllString(result, "<ip>")
	result = hexRegex.ReplaceAllString(result, "<hex>")
	result = quotedRegex.ReplaceAllString(result, "<str>")
	result = quotedRegex2.ReplaceAllString(result, "<str>")
	result = numberRegex.ReplaceAllString(result, "<num>")
	// 压缩多余空格。
	result = strings.Join(strings.Fields(result), " ")
	return result
}

// Analyze 对一组日志命中进行聚合分析。
func (a *Analyzer) Analyze(hits []LogHit) *AnalyzeResult {
	if len(hits) == 0 {
		return &AnalyzeResult{AnalyzedAt: time.Now()}
	}

	// 按模板分组。
	groupsMap := make(map[string]*LogGroup)
	for _, hit := range hits {
		template := NormalizeMessage(hit.Message)
		if template == "" {
			template = "<empty>"
		}

		g, ok := groupsMap[template]
		if !ok {
			g = &LogGroup{
				Template:      template,
				FirstSeen:     hit.Timestamp,
				LastSeen:      hit.Timestamp,
				SampleMessage: hit.Message,
			}
			groupsMap[template] = g
		}

		g.Count++
		if hit.Timestamp.Before(g.FirstSeen) || g.FirstSeen.IsZero() {
			g.FirstSeen = hit.Timestamp
		}
		if hit.Timestamp.After(g.LastSeen) {
			g.LastSeen = hit.Timestamp
		}
		// 记录服务和命名空间。
		svc := hit.Pod
		if svc != "" && !containsStrLog(svc, g.Services) {
			g.Services = append(g.Services, svc)
		}
		if hit.Namespace != "" && !containsStrLog(hit.Namespace, g.Namespaces) {
			g.Namespaces = append(g.Namespaces, hit.Namespace)
		}
	}

	// 转为切片并排序（按 count 倒序）。
	groups := make([]LogGroup, 0, len(groupsMap))
	now := time.Now()
	highFreqCount := 0
	newCount := 0
	for _, g := range groupsMap {
		g.IsHighFrequency = g.Count >= a.HighFreqThreshold
		if g.IsHighFrequency {
			highFreqCount++
		}
		// 新异常：首次出现在最近窗口内。
		if !g.FirstSeen.IsZero() && now.Sub(g.FirstSeen) <= a.NewWindow {
			g.IsNew = true
			newCount++
		}
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Count > groups[j].Count
	})

	return &AnalyzeResult{
		TotalLogs:     len(hits),
		Groups:        groups,
		HighFreqCount: highFreqCount,
		NewCount:      newCount,
		AnalyzedAt:    now,
	}
}

func containsStrLog(s string, slice []string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
