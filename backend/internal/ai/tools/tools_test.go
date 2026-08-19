package tools

import (
	"context"
	"testing"
	"time"
)

// mockTool 是测试用 Tool。
type mockTool struct {
	name     string
	readOnly bool
	result   ToolResult
	err      error
}

func (m *mockTool) Name() string                  { return m.name }
func (m *mockTool) Description() string           { return "mock tool" }
func (m *mockTool) InputSchema() ToolSchema       { return ToolSchema{Type: "object"} }
func (m *mockTool) ReadOnly() bool                { return m.readOnly }
func (m *mockTool) Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error) {
	return m.result, m.err
}

func TestRegistry_RegisterReadOnly(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&mockTool{name: "test", readOnly: true})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, ok := r.Get("test"); !ok {
		t.Error("tool not found after register")
	}
}

func TestRegistry_RejectWriteTool(t *testing.T) {
	r := NewRegistry()
	err := r.Register(&mockTool{name: "dangerous", readOnly: false})
	if err == nil {
		t.Error("expected error for write tool")
	}
}

func TestRegistry_ExecuteNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&mockTool{name: "a", readOnly: true})
	_ = r.Register(&mockTool{name: "b", readOnly: true})
	names := r.List()
	if len(names) != 2 {
		t.Errorf("expected 2 tools, got %d", len(names))
	}
}

func TestParseToolCallRequest_Valid(t *testing.T) {
	raw := `{"tool_name": "get_incident", "input": {"incident_id": 7}}`
	req, err := parseToolCallRequest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if req.ToolName != "get_incident" {
		t.Errorf("expected get_incident, got %s", req.ToolName)
	}
	if req.Input["incident_id"].(float64) != 7 {
		t.Errorf("expected incident_id=7")
	}
}

func TestParseToolCallRequest_Markdown(t *testing.T) {
	raw := "```json\n{\"tool_name\": \"get_rca\", \"input\": {\"incident_id\": 1}}\n```"
	req, err := parseToolCallRequest(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if req.ToolName != "get_rca" {
		t.Errorf("expected get_rca, got %s", req.ToolName)
	}
}

func TestParseToolCallRequest_Invalid(t *testing.T) {
	_, err := parseToolCallRequest("not json at all")
	if err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestParseAgentResponse_Valid(t *testing.T) {
	raw := `{"answer": "test answer", "summary": "summary", "confidence": 0.85}`
	resp, err := parseAgentResponse(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if resp.Answer != "test answer" {
		t.Errorf("expected 'test answer', got '%s'", resp.Answer)
	}
	if resp.Confidence != 0.85 {
		t.Errorf("expected 0.85, got %f", resp.Confidence)
	}
}

func TestParseAgentResponse_EmptyAnswer(t *testing.T) {
	raw := `{"answer": ""}`
	_, err := parseAgentResponse(raw)
	if err == nil {
		t.Error("expected error for empty answer")
	}
}

func TestRedactSensitive(t *testing.T) {
	input := `{"password": "secret123", "token": "abc", "normal": "value"}`
	result := redactSensitive(input)
	if result == input {
		t.Error("expected redaction")
	}
	if contains(result, "secret123") {
		t.Error("password should be redacted")
	}
	if !contains(result, "[REDACTED]") {
		t.Error("expected [REDACTED] marker")
	}
}

func TestRedactSensitive_Empty(t *testing.T) {
	if redactSensitive("") != "" {
		t.Error("empty input should return empty")
	}
}

func TestToolAuditRecord(t *testing.T) {
	call := ToolCall{
		ToolName: "get_incident",
		Input:    map[string]interface{}{"incident_id": float64(7)},
		Result: ToolResult{
			Success:   true,
			Available: true,
			Source:    "mysql",
		},
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
	}
	record := RecordFromToolCall(call, "req-1", 7, 1)
	if record.ToolName != "get_incident" {
		t.Errorf("expected get_incident, got %s", record.ToolName)
	}
	if record.IncidentID != 7 {
		t.Errorf("expected incident_id=7")
	}
	if record.DurationMs != 100 {
		t.Errorf("expected 100ms, got %d", record.DurationMs)
	}
}

func TestEngineConfig_Defaults(t *testing.T) {
	cfg := DefaultEngineConfig()
	if cfg.MaxToolCalls != 8 {
		t.Errorf("expected 8, got %d", cfg.MaxToolCalls)
	}
	if cfg.ToolTimeout != 5*time.Second {
		t.Error("expected 5s timeout")
	}
}

func TestEngineConfig_ZeroValues(t *testing.T) {
	e := NewEngine(nil, nil, EngineConfig{})
	if e.config.MaxToolCalls != 8 {
		t.Error("zero MaxToolCalls should default to 8")
	}
	if e.config.ToolTimeout != 5*time.Second {
		t.Error("zero ToolTimeout should default to 5s")
	}
}

func TestBuildToolsDescription(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&mockTool{name: "test_tool", readOnly: true})
	e := NewEngine(nil, r, DefaultEngineConfig())
	desc := e.buildToolsDescription()
	if !contains(desc, "test_tool") {
		t.Error("tool description should contain tool name")
	}
}

func TestBuildInitialMessages(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&mockTool{name: "test_tool", readOnly: true})
	e := NewEngine(nil, r, DefaultEngineConfig())
	msgs := e.buildInitialMessages("test question", "incident context")
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Error("first message should be system")
	}
	if msgs[1].Role != "user" {
		t.Error("second message should be user")
	}
	if !contains(msgs[1].Content, "test question") {
		t.Error("user message should contain question")
	}
	if !contains(msgs[1].Content, "incident context") {
		t.Error("user message should contain incident context")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
