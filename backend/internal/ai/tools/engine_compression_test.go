package tools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestSummarizeToolResult_SmallResultNotTruncated 验证 < 2KB 的结果不被截断。
func TestSummarizeToolResult_SmallResultNotTruncated(t *testing.T) {
	result := ToolResult{
		ToolName:  "get_incident",
		Success:   true,
		Available: true,
		Data: map[string]interface{}{
			"id":     43,
			"title":  "KubePodCrashLooping",
			"status": "open",
		},
		Source:    "mysql",
		Timestamp: time.Now(),
	}

	summary := summarizeToolResult(result)

	// 小结果应该直接返回完整 JSON，不包含 truncated 字段
	if strings.Contains(summary, `"truncated":true`) {
		t.Errorf("small result should not be truncated, got: %s", summary)
	}
	if strings.Contains(summary, `"data_note"`) {
		t.Errorf("small result should not have data_note, got: %s", summary)
	}

	// 验证是合法 JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("summary should be valid JSON: %v", err)
	}

	// 验证完整数据保留
	if parsed["success"] != true {
		t.Errorf("success should be true, got: %v", parsed["success"])
	}
	data, ok := parsed["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data should be preserved as map")
	}
	if data["id"].(float64) != 43 {
		t.Errorf("data.id should be 43, got: %v", data["id"])
	}
}

// TestSummarizeToolResult_Boundary2KB 验证 2KB 边界行为。
// 刚好小于 2KB 不截断，大于等于 2KB 触发压缩。
func TestSummarizeToolResult_Boundary2KB(t *testing.T) {
	// 构造一个接近 2KB 的结果
	largeString := strings.Repeat("x", 1800)
	result := ToolResult{
		ToolName:  "search_logs",
		Success:   true,
		Available: true,
		Data: map[string]interface{}{
			"logs": largeString,
		},
		Source:    "elasticsearch",
		Timestamp: time.Now(),
	}

	fullJSON, _ := json.Marshal(result)
	t.Logf("full JSON size: %d bytes (threshold: %d)", len(fullJSON), compressionThreshold)

	summary := summarizeToolResult(result)

	if len(fullJSON) < compressionThreshold {
		// 小于阈值，不截断
		if strings.Contains(summary, `"truncated":true`) {
			t.Errorf("result < 2KB should not be truncated, size=%d", len(fullJSON))
		}
	} else {
		// 大于等于阈值，应该压缩
		if !strings.Contains(summary, `"truncated":true`) {
			t.Errorf("result >= 2KB should be truncated, size=%d", len(fullJSON))
		}
	}
}

// TestSummarizeToolResult_LargeResultCompressed 验证 > 2KB 的结果被压缩。
func TestSummarizeToolResult_LargeResultCompressed(t *testing.T) {
	// 构造一个 > 2KB 的结果（大量日志条目）
	logs := make([]interface{}, 0, 50)
	for i := 0; i < 50; i++ {
		logs = append(logs, map[string]interface{}{
			"timestamp": "2026-08-27T10:00:00Z",
			"level":     "error",
			"message":   strings.Repeat("error message detail ", 10),
			"pod":       "test-pod-xyz",
			"namespace": "monitoring",
		})
	}

	result := ToolResult{
		ToolName:  "search_logs",
		Success:   true,
		Available: true,
		Data: map[string]interface{}{
			"logs":  logs,
			"total": 50,
		},
		Source:    "elasticsearch",
		Timestamp: time.Now(),
	}

	fullJSON, _ := json.Marshal(result)
	t.Logf("full JSON size: %d bytes", len(fullJSON))
	if len(fullJSON) <= compressionThreshold {
		t.Fatalf("test setup: result should be > 2KB, got %d", len(fullJSON))
	}

	summary := summarizeToolResult(result)

	// 验证被压缩
	if !strings.Contains(summary, `"truncated":true`) {
		t.Errorf("large result should be truncated")
	}
	if !strings.Contains(summary, `"original_size"`) {
		t.Errorf("compressed result should have original_size")
	}

	// 验证是合法 JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("compressed summary should be valid JSON: %v", err)
	}

	// 验证 metadata 保留
	if parsed["tool_name"] != "search_logs" {
		t.Errorf("tool_name should be preserved, got: %v", parsed["tool_name"])
	}
	if parsed["success"] != true {
		t.Errorf("success should be preserved")
	}
	if parsed["available"] != true {
		t.Errorf("available should be preserved")
	}
	if parsed["source"] != "elasticsearch" {
		t.Errorf("source should be preserved")
	}

	// 验证压缩后大小明显小于原始
	compressedSize := len(summary)
	if compressedSize >= len(fullJSON) {
		t.Errorf("compressed size (%d) should be smaller than original (%d)", compressedSize, len(fullJSON))
	}
	t.Logf("compression: %d -> %d bytes (%.1f%% reduction)",
		len(fullJSON), compressedSize, float64(len(fullJSON)-compressedSize)/float64(len(fullJSON))*100)
}

// TestSummarizeToolResult_LargeArrayPreservesCountAndFirstN 验证大数组保留 count 和前 N 项。
func TestSummarizeToolResult_LargeArrayPreservesCountAndFirstN(t *testing.T) {
	// 构造 20 项的大数组
	items := make([]interface{}, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, map[string]interface{}{
			"id":    i,
			"name":  strings.Repeat("item-name-", 5),
			"value": strings.Repeat("value-data-", 10),
		})
	}

	result := ToolResult{
		ToolName:  "get_alerts",
		Success:   true,
		Available: true,
		Data:      items, // Data 直接是数组
		Source:    "mysql",
		Timestamp: time.Now(),
	}

	summary := summarizeToolResult(result)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("summary should be valid JSON: %v", err)
	}

	// 验证 data 被压缩为包含 count 的结构
	data, ok := parsed["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("compressed data should be map with count, got: %T", parsed["data"])
	}

	count, ok := data["count"].(float64)
	if !ok {
		t.Fatalf("compressed array should have count, got: %v", data["count"])
	}
	if int(count) != 20 {
		t.Errorf("count should be 20, got: %v", count)
	}

	// 验证保留前 N 项
	compressedItems, ok := data["items"].([]interface{})
	if !ok {
		t.Fatalf("compressed array should have items")
	}
	if len(compressedItems) != maxCompressedArrayItems {
		t.Errorf("should preserve %d items, got %d", maxCompressedArrayItems, len(compressedItems))
	}

	// 验证有 note 说明
	if _, ok := data["note"]; !ok {
		t.Errorf("large array should have note")
	}
}

// TestSummarizeToolResult_LargeStringTruncated 验证大字符串被截断。
func TestSummarizeToolResult_LargeStringTruncated(t *testing.T) {
	// 构造一个超长字符串（> 2KB）
	longLog := strings.Repeat("2026-08-27T10:00:00Z ERROR pod/test error message detail ", 100)

	result := ToolResult{
		ToolName:  "search_logs",
		Success:   true,
		Available: true,
		Data: map[string]interface{}{
			"raw_log": longLog,
		},
		Source:    "elasticsearch",
		Timestamp: time.Now(),
	}

	summary := summarizeToolResult(result)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("summary should be valid JSON: %v", err)
	}

	data, ok := parsed["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data should be map")
	}

	rawLog, ok := data["raw_log"].(string)
	if !ok {
		t.Fatalf("raw_log should be string")
	}

	// 验证字符串被截断（包含截断标记）
	if !strings.Contains(rawLog, "truncated") {
		t.Errorf("large string should be truncated with marker")
	}
	if len(rawLog) > maxCompressedStringLength+100 { // 允许截断标记的额外长度
		t.Errorf("truncated string should be <= maxLength+marker, got %d", len(rawLog))
	}
}

// TestSummarizeToolResult_NestedObjectCompressed 验证嵌套对象递归压缩。
func TestSummarizeToolResult_NestedObjectCompressed(t *testing.T) {
	// 构造嵌套对象，包含大数组和大字符串
	nestedData := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"items": func() []interface{} {
					items := make([]interface{}, 10)
					for i := range items {
						items[i] = map[string]interface{}{
							"id":   i,
							"desc": strings.Repeat("nested-item-description-", 8),
						}
					}
					return items
				}(),
				"long_text": strings.Repeat("nested long text content ", 50),
			},
		},
		"metadata": map[string]interface{}{
			"total": 10,
			"status": "ok",
		},
	}

	result := ToolResult{
		ToolName:  "get_topology",
		Success:   true,
		Available: true,
		Data:      nestedData,
		Source:    "kubernetes",
		Timestamp: time.Now(),
	}

	fullJSON, _ := json.Marshal(result)
	t.Logf("full JSON size: %d bytes", len(fullJSON))

	summary := summarizeToolResult(result)

	// 验证是合法 JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("compressed summary should be valid JSON: %v", err)
	}

	// 验证嵌套结构保留（不是只保留 keys）
	data, ok := parsed["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data should be preserved as map, got: %T", parsed["data"])
	}

	level1, ok := data["level1"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested level1 should be preserved as map")
	}

	// 验证嵌套结构存在（level2）
	if _, ok := level1["level2"]; !ok {
		t.Fatalf("nested level2 should be preserved")
	}

	// 验证 metadata 保留
	metadata, ok := data["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata should be preserved")
	}
	if metadata["status"] != "ok" {
		t.Errorf("metadata.status should be 'ok', got: %v", metadata["status"])
	}

	// 验证压缩后大小小于原始
	if len(summary) >= len(fullJSON) {
		t.Errorf("compressed should be smaller: %d >= %d", len(summary), len(fullJSON))
	}
}

// TestSummarizeToolResult_ErrorResultPreserved 验证 error 结果的 error/success/available 不丢失。
func TestSummarizeToolResult_ErrorResultPreserved(t *testing.T) {
	result := ToolResult{
		ToolName:  "query_metrics",
		Success:   false,
		Available: false,
		Error:     "connection refused: prometheus at 127.0.0.1:9090 is not running",
		Source:    "prometheus",
		Timestamp: time.Now(),
	}

	summary := summarizeToolResult(result)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("summary should be valid JSON: %v", err)
	}

	// 验证 error 信息保留
	if parsed["success"] != false {
		t.Errorf("success should be false, got: %v", parsed["success"])
	}
	if parsed["available"] != false {
		t.Errorf("available should be false, got: %v", parsed["available"])
	}
	if parsed["error"] == nil || parsed["error"] == "" {
		t.Errorf("error should be preserved")
	}
	if !strings.Contains(parsed["error"].(string), "connection refused") {
		t.Errorf("error message should contain 'connection refused', got: %v", parsed["error"])
	}
	if parsed["source"] != "prometheus" {
		t.Errorf("source should be preserved")
	}
}

// TestSummarizeToolResult_UnavailableToolPreserved 验证 unavailable Tool 的 available=false 保留。
func TestSummarizeToolResult_UnavailableToolPreserved(t *testing.T) {
	// 构造一个大的 unavailable 结果（确保触发压缩）
	largeData := strings.Repeat("x", 3000)
	result := ToolResult{
		ToolName:  "get_k8s_events",
		Success:   false,
		Available: false,
		Error:     "kubernetes API not available",
		Data: map[string]interface{}{
			"raw": largeData,
		},
		Source:    "kubernetes",
		Timestamp: time.Now(),
	}

	summary := summarizeToolResult(result)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("summary should be valid JSON: %v", err)
	}

	// 验证 available=false 保留
	if parsed["available"] != false {
		t.Errorf("available should be false for unavailable tool")
	}
	if parsed["success"] != false {
		t.Errorf("success should be false")
	}
	if parsed["error"] == nil {
		t.Errorf("error should be preserved for unavailable tool")
	}
	// 验证被压缩
	if !strings.Contains(summary, `"truncated":true`) {
		t.Errorf("large unavailable result should be compressed")
	}
}

// TestSummarizeToolResult_CompressedResultCanBeAppendedToMessages 验证压缩结果可以被 Engine append 到 messages。
func TestSummarizeToolResult_CompressedResultCanBeAppendedToMessages(t *testing.T) {
	// 构造大结果
	largeItems := make([]interface{}, 100)
	for i := range largeItems {
		largeItems[i] = map[string]interface{}{
			"id":   i,
			"name": strings.Repeat("alert-name-", 10),
		}
	}

	result := ToolResult{
		ToolName:  "get_alerts",
		Success:   true,
		Available: true,
		Data: map[string]interface{}{
			"alerts": largeItems,
			"total":  100,
		},
		Source:    "mysql",
		Timestamp: time.Now(),
	}

	summary := summarizeToolResult(result)

	// 模拟 Engine 的 messages append 逻辑
	content := "工具 get_alerts 执行结果:\n" + summary + "\n\n如果已有足够证据，请直接给出最终回答。"

	// 验证 content 非空且包含关键信息
	if len(content) == 0 {
		t.Fatalf("content should not be empty")
	}
	if !strings.Contains(content, "get_alerts") {
		t.Errorf("content should contain tool name")
	}
	if !strings.Contains(content, `"truncated":true`) {
		t.Errorf("content should contain compressed result")
	}

	// 验证 summary 本身是合法 JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("summary should be valid JSON for messages append: %v", err)
	}
}

// TestSummarizeToolResult_Deterministic 验证相同输入产生稳定的压缩结果。
func TestSummarizeToolResult_Deterministic(t *testing.T) {
	result := ToolResult{
		ToolName:  "search_logs",
		Success:   true,
		Available: true,
		Data: map[string]interface{}{
			"logs": []interface{}{
				map[string]interface{}{"id": 1, "msg": strings.Repeat("log-msg-", 20)},
				map[string]interface{}{"id": 2, "msg": strings.Repeat("log-msg-", 20)},
				map[string]interface{}{"id": 3, "msg": strings.Repeat("log-msg-", 20)},
				map[string]interface{}{"id": 4, "msg": strings.Repeat("log-msg-", 20)},
				map[string]interface{}{"id": 5, "msg": strings.Repeat("log-msg-", 20)},
			},
		},
		Source:    "elasticsearch",
		Timestamp: time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC), // 固定时间确保确定性
	}

	summary1 := summarizeToolResult(result)
	summary2 := summarizeToolResult(result)

	if summary1 != summary2 {
		t.Errorf("compression should be deterministic:\nresult1: %s\nresult2: %s", summary1, summary2)
	}
}

// TestSummarizeToolResult_NoPanicOnNilData 验证 nil Data 不 panic。
func TestSummarizeToolResult_NoPanicOnNilData(t *testing.T) {
	result := ToolResult{
		ToolName:  "test_tool",
		Success:   true,
		Available: true,
		Data:      nil,
		Source:    "test",
		Timestamp: time.Now(),
	}

	// 不应 panic
	summary := summarizeToolResult(result)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("summary should be valid JSON: %v", err)
	}
	if parsed["success"] != true {
		t.Errorf("success should be true")
	}
}

// TestSummarizeToolResult_CompressionReducesPromptSize 验证压缩确实减少了发送给 LLM 的上下文大小。
func TestSummarizeToolResult_CompressionReducesPromptSize(t *testing.T) {
	// 模拟 5 轮 Tool 调用，每轮结果都很大
	toolResults := make([]ToolResult, 5)
	for i := range toolResults {
		items := make([]interface{}, 30)
		for j := range items {
			items[j] = map[string]interface{}{
				"id":    j,
				"name":  strings.Repeat("item-name-", 8),
				"value": strings.Repeat("value-data-", 12),
			}
		}
		toolResults[i] = ToolResult{
			ToolName:  []string{"get_incident", "get_k8s_events", "get_k8s_resource", "get_rca", "search_logs"}[i],
			Success:   true,
			Available: true,
			Data: map[string]interface{}{
				"items": items,
				"total": 30,
			},
			Source:    "test",
			Timestamp: time.Now(),
		}
	}

	// 计算不压缩时的总大小（模拟旧逻辑：只有 >4KB 才压缩，且压缩后只保留 keys）
	// 这里直接比较：完整序列化 vs 新压缩逻辑
	var fullTotalSize int
	var compressedTotalSize int

	for _, result := range toolResults {
		fullJSON, _ := json.Marshal(result)
		fullTotalSize += len(fullJSON)

		compressed := summarizeToolResult(result)
		compressedTotalSize += len(compressed)
	}

	t.Logf("5 Tool Results - Full size: %d bytes, Compressed size: %d bytes", fullTotalSize, compressedTotalSize)
	t.Logf("Reduction: %.1f%%", float64(fullTotalSize-compressedTotalSize)/float64(fullTotalSize)*100)

	if compressedTotalSize >= fullTotalSize {
		t.Errorf("compression should reduce total size: %d >= %d", compressedTotalSize, fullTotalSize)
	}

	// 验证每个压缩结果都是合法 JSON
	for i, result := range toolResults {
		compressed := summarizeToolResult(result)
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(compressed), &parsed); err != nil {
			t.Errorf("result %d (%s) compressed output is not valid JSON: %v", i, result.ToolName, err)
		}
	}
}
