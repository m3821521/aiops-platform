package tools

import (
	"context"
	"time"
)

// ToolSchema 描述 Tool 的输入参数 Schema。
type ToolSchema struct {
	Type       string                  `json:"type"` // object
	Properties map[string]ToolProperty `json:"properties"`
	Required   []string                `json:"required,omitempty"`
}

// ToolProperty 是单个输入参数的描述。
type ToolProperty struct {
	Type        string   `json:"type"` // string/number/integer/boolean
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolResult 是 Tool 执行的标准化结果。
type ToolResult struct {
	ToolName  string      `json:"tool_name"`
	Success   bool        `json:"success"`
	Available bool        `json:"available"` // 数据源是否可用
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	Source    string      `json:"source"`
	Timestamp time.Time   `json:"timestamp"`
}

// Tool 是 AI 可调用的工具接口。
// 所有 P2-6 Tool 必须 ReadOnly() == true。
type Tool interface {
	Name() string
	Description() string
	InputSchema() ToolSchema
	Execute(ctx context.Context, input map[string]interface{}) (ToolResult, error)
	ReadOnly() bool
}

// ToolCall 记录一次 Tool 调用（用于 UI 展示和审计）。
type ToolCall struct {
	ToolName  string                 `json:"tool_name"`
	Input     map[string]interface{} `json:"input"`
	Result    ToolResult             `json:"result"`
	Duration  time.Duration          `json:"duration_ms"`
	Timestamp time.Time              `json:"timestamp"`
}
