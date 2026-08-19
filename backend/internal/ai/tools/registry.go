package tools

import (
	"context"
	"fmt"
	"sync"
)

// Registry 是 AI Tool 的注册表。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 创建空的 Tool Registry。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册一个 Tool。如果 Tool 不是 ReadOnly，返回错误（P2-6 安全约束）。
func (r *Registry) Register(tool Tool) error {
	if !tool.ReadOnly() {
		return fmt.Errorf("tool %s 不是只读工具，P2-6 禁止注册写操作 Tool", tool.Name())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	return nil
}

// Get 获取指定名称的 Tool。
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// List 返回所有已注册的 Tool 名称。
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Execute 执行指定 Tool。
func (r *Registry) Execute(ctx context.Context, name string, input map[string]interface{}) (ToolResult, error) {
	tool, ok := r.Get(name)
	if !ok {
		return ToolResult{ToolName: name, Success: false, Error: "tool not found"}, fmt.Errorf("tool %s not found", name)
	}
	return tool.Execute(ctx, input)
}

// All 返回所有 Tool 的描述（用于构建 AI Prompt 中的工具列表）。
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		all = append(all, t)
	}
	return all
}
