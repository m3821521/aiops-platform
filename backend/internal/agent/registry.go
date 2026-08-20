package agent

import (
	"context"
	"fmt"
	"sync"
)

// Registry 是 Agent 注册中心。
type Registry struct {
	mu     sync.RWMutex
	agents map[AgentType][]Agent
	byName map[string]Agent
}

// NewRegistry 创建新的 Agent 注册中心。
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[AgentType][]Agent),
		byName: make(map[string]Agent),
	}
}

// Register 注册一个 Agent。
func (r *Registry) Register(agent Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byName[agent.Name()]; exists {
		return fmt.Errorf("agent %s already registered", agent.Name())
	}

	r.agents[agent.Type()] = append(r.agents[agent.Type()], agent)
	r.byName[agent.Name()] = agent
	return nil
}

// GetByType 根据类型获取 Agent 列表。
func (r *Registry) GetByType(agentType AgentType) []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]Agent, len(r.agents[agentType]))
	copy(agents, r.agents[agentType])
	return agents
}

// GetByName 根据名称获取 Agent。
func (r *Registry) GetByName(name string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.byName[name]
	return agent, exists
}

// GetAll 获取所有已注册的 Agent。
func (r *Registry) GetAll() []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []Agent
	for _, agents := range r.agents {
		all = append(all, agents...)
	}
	return all
}

// GetTypes 获取所有已注册的 Agent 类型。
func (r *Registry) GetTypes() []AgentType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var types []AgentType
	for t := range r.agents {
		types = append(types, t)
	}
	return types
}

// SelectAgents 根据任务描述选择合适的 Agent。
func (r *Registry) SelectAgents(ctx context.Context, description string, preferredTypes []AgentType) []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 如果指定了优先类型，返回这些类型的 Agent
	if len(preferredTypes) > 0 {
		var selected []Agent
		for _, t := range preferredTypes {
			selected = append(selected, r.agents[t]...)
		}
		return selected
	}

	// 默认选择所有 Agent
	var all []Agent
	for _, agents := range r.agents {
		all = append(all, agents...)
	}
	return all
}

// BaseAgent 是基础 Agent 实现，可被嵌入。
type BaseAgent struct {
	name         string
	agentType    AgentType
	description  string
	capabilities []string
}

// NewBaseAgent 创建基础 Agent。
func NewBaseAgent(name string, agentType AgentType, description string, capabilities []string) *BaseAgent {
	return &BaseAgent{
		name:         name,
		agentType:    agentType,
		description:  description,
		capabilities: capabilities,
	}
}

// Name 返回 Agent 名称。
func (a *BaseAgent) Name() string { return a.name }

// Type 返回 Agent 类型。
func (a *BaseAgent) Type() AgentType { return a.agentType }

// Description 返回 Agent 描述。
func (a *BaseAgent) Description() string { return a.description }

// Capabilities 返回 Agent 能力列表。
func (a *BaseAgent) Capabilities() []string { return a.capabilities }
