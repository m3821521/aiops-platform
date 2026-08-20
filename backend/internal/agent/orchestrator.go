package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Orchestrator 是多 Agent 编排器。
type Orchestrator struct {
	registry *Registry
	mu       sync.RWMutex
	results  map[string]*OrchestrationResult
	events   map[string][]*OrchestrationEvent
}

// NewOrchestrator 创建新的多 Agent 编排器。
func NewOrchestrator(registry *Registry) *Orchestrator {
	return &Orchestrator{
		registry: registry,
		results:  make(map[string]*OrchestrationResult),
		events:   make(map[string][]*OrchestrationEvent),
	}
}

// Orchestrate 执行多 Agent 编排。
func (o *Orchestrator) Orchestrate(ctx context.Context, req *OrchestrationRequest) (*OrchestrationResult, error) {
	taskID := req.TaskID
	if taskID == "" {
		taskID = uuid.New().String()
	}

	// 1. 任务分解
	tasks := o.decomposeTask(req)

	result := &OrchestrationResult{
		TaskID:    taskID,
		Title:     req.Title,
		Status:    TaskStatusInProgress,
		Tasks:     tasks,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	o.addEvent(taskID, &OrchestrationEvent{
		Timestamp: time.Now(),
		Type:      "orchestration_started",
		Message:   fmt.Sprintf("多 Agent 编排开始，共 %d 个子任务", len(tasks)),
	})

	// 2. 按依赖顺序执行 Agent
	for _, task := range tasks {
		select {
		case <-ctx.Done():
			result.Status = TaskStatusCancelled
			result.UpdatedAt = time.Now()
			o.results[taskID] = result
			return result, ctx.Err()
		default:
		}

		// 检查依赖
		if !o.dependenciesMet(task, tasks) {
			task.Status = AgentStatusSkipped
			o.addEvent(taskID, &OrchestrationEvent{
				Timestamp: time.Now(),
				Type:      "task_skipped",
				TaskID:    task.ID,
				Message:   fmt.Sprintf("任务 %s 依赖未满足，跳过", task.Title),
			})
			continue
		}

		// 执行任务
		o.executeTask(ctx, taskID, task, result)
	}

	// 3. 结果聚合
	o.aggregateResults(result)

	// 4. 风险评估
	o.assessRisk(result)

	// 5. 检查是否需要审批
	if result.RiskAssessment != nil && result.RiskAssessment.RequiresApproval && !req.AutoApprove {
		result.Status = TaskStatusAwaitingApproval
		o.addEvent(taskID, &OrchestrationEvent{
			Timestamp: time.Now(),
			Type:      "approval_required",
			Message:   fmt.Sprintf("操作风险等级 %s，需要人工审批", result.RiskAssessment.Level),
		})
	} else {
		result.Status = TaskStatusCompleted
	}

	result.UpdatedAt = time.Now()
	result.CompletedAt = func() *time.Time { t := time.Now(); return &t }()
	result.DurationMs = result.CompletedAt.Sub(result.CreatedAt).Milliseconds()

	o.results[taskID] = result

	o.addEvent(taskID, &OrchestrationEvent{
		Timestamp: time.Now(),
		Type:      "orchestration_completed",
		Message:   fmt.Sprintf("多 Agent 编排完成，状态 %s", result.Status),
	})

	return result, nil
}

// decomposeTask 任务分解，将复杂任务分解为多个子任务。
func (o *Orchestrator) decomposeTask(req *OrchestrationRequest) []*Task {
	var tasks []*Task

	// 确定需要的 Agent 类型
	agentTypes := req.AgentTypes
	if len(agentTypes) == 0 {
		// 默认流程：监控 → 诊断 → 风险评估 → 报告
		agentTypes = []AgentType{
			AgentTypeMonitor,
			AgentTypeDiagnosis,
			AgentTypeRisk,
			AgentTypeReporter,
		}
	}

	// 为每个 Agent 类型创建任务
	var prevTaskID string
	for i, agentType := range agentTypes {
		agents := o.registry.GetByType(agentType)
		if len(agents) == 0 {
			continue
		}

		agent := agents[0] // 使用第一个匹配的 Agent
		taskID := fmt.Sprintf("task-%d", i+1)

		task := &Task{
			ID:          taskID,
			Title:       fmt.Sprintf("%s - %s", agent.Name(), req.Title),
			Description: fmt.Sprintf("由 %s 执行：%s", agent.Description(), req.Description),
			AgentType:   agentType,
			AgentName:   agent.Name(),
			Priority:    i + 1,
			Parameters:  req.Parameters,
			Status:      AgentStatusPending,
		}

		if prevTaskID != "" {
			task.DependsOn = []string{prevTaskID}
		}

		tasks = append(tasks, task)
		prevTaskID = taskID
	}

	return tasks
}

// executeTask 执行单个任务。
func (o *Orchestrator) executeTask(ctx context.Context, taskID string, task *Task, result *OrchestrationResult) {
	agent, exists := o.registry.GetByName(task.AgentName)
	if !exists {
		task.Status = AgentStatusFailed
		task.Error = fmt.Sprintf("agent %s not found", task.AgentName)
		o.addEvent(taskID, &OrchestrationEvent{
			Timestamp: time.Now(),
			Type:      "task_failed",
			TaskID:    task.ID,
			AgentName: task.AgentName,
			Message:   fmt.Sprintf("Agent %s 未找到", task.AgentName),
		})
		return
	}

	task.Status = AgentStatusRunning
	now := time.Now()
	task.StartedAt = &now

	o.addEvent(taskID, &OrchestrationEvent{
		Timestamp: time.Now(),
		Type:      "task_started",
		TaskID:    task.ID,
		AgentName: task.AgentName,
		Message:   fmt.Sprintf("Agent %s 开始执行", task.AgentName),
	})

	// 执行 Agent
	agentResult, err := agent.Execute(ctx, task)
	if err != nil {
		task.Status = AgentStatusFailed
		task.Error = err.Error()
		finished := time.Now()
		task.FinishedAt = &finished
		task.DurationMs = finished.Sub(now).Milliseconds()

		o.addEvent(taskID, &OrchestrationEvent{
			Timestamp: time.Now(),
			Type:      "task_failed",
			TaskID:    task.ID,
			AgentName: task.AgentName,
			Message:   fmt.Sprintf("Agent %s 执行失败: %s", task.AgentName, err.Error()),
		})
		return
	}

	task.Result = agentResult
	task.Status = AgentStatusSuccess
	finished := time.Now()
	task.FinishedAt = &finished
	task.DurationMs = finished.Sub(now).Milliseconds()

	o.addEvent(taskID, &OrchestrationEvent{
		Timestamp: time.Now(),
		Type:      "task_completed",
		TaskID:    task.ID,
		AgentName: task.AgentName,
		Message:   fmt.Sprintf("Agent %s 执行完成，耗时 %dms", task.AgentName, task.DurationMs),
	})
}

// dependenciesMet 检查任务依赖是否满足。
func (o *Orchestrator) dependenciesMet(task *Task, allTasks []*Task) bool {
	if len(task.DependsOn) == 0 {
		return true
	}

	for _, depID := range task.DependsOn {
		for _, t := range allTasks {
			if t.ID == depID && t.Status != AgentStatusSuccess {
				return false
			}
		}
	}

	return true
}

// aggregateResults 聚合所有 Agent 的结果。
func (o *Orchestrator) aggregateResults(result *OrchestrationResult) {
	var allFindings []Finding
	var allEvidence []Evidence
	var allRecommendations []Recommendation
	var allMetrics = make(map[string]interface{})

	for _, task := range result.Tasks {
		if task.Result == nil {
			continue
		}

		allFindings = append(allFindings, task.Result.Findings...)
		allEvidence = append(allEvidence, task.Result.Evidence...)
		allRecommendations = append(allRecommendations, task.Result.Recommendations...)

		for k, v := range task.Result.Metrics {
			allMetrics[k] = v
		}
	}

	result.Findings = allFindings
	result.Evidence = allEvidence
	result.Recommendations = allRecommendations

	// 生成摘要
	if len(allFindings) > 0 {
		result.Summary = fmt.Sprintf("多 Agent 分析完成，共发现 %d 个问题，%d 条建议", len(allFindings), len(allRecommendations))
	} else {
		result.Summary = "多 Agent 分析完成，系统运行正常"
	}

	// 生成最终报告
	result.FinalReport = o.generateReport(result)
}

// generateReport 生成最终报告。
func (o *Orchestrator) generateReport(result *OrchestrationResult) string {
	report := fmt.Sprintf("## 多 Agent 分析报告\n\n")
	report += fmt.Sprintf("**任务**: %s\n\n", result.Title)
	report += fmt.Sprintf("**状态**: %s\n\n", result.Status)

	if len(result.Findings) > 0 {
		report += "### 主要发现\n\n"
		for i, finding := range result.Findings {
			report += fmt.Sprintf("%d. **%s** (%s)\n   %s\n\n", i+1, finding.Title, finding.Severity, finding.Description)
		}
	}

	if len(result.Recommendations) > 0 {
		report += "### 建议操作\n\n"
		for i, rec := range result.Recommendations {
			report += fmt.Sprintf("%d. **[%s] %s** (风险: %s)\n   %s\n   原因: %s\n\n",
				i+1, rec.Priority, rec.Title, rec.Risk, rec.Description, rec.Reason)
		}
	}

	if len(result.Evidence) > 0 {
		report += "### 证据\n\n"
		for i, ev := range result.Evidence {
			report += fmt.Sprintf("%d. [%s] %s\n", i+1, ev.Source, ev.Description)
		}
	}

	return report
}

// assessRisk 评估整体风险。
func (o *Orchestrator) assessRisk(result *OrchestrationResult) {
	// 从风险评估 Agent 的结果中获取风险评估
	for _, task := range result.Tasks {
		if task.AgentType == AgentTypeRisk && task.Result != nil && task.Result.RiskAssessment != nil {
			result.RiskAssessment = task.Result.RiskAssessment
			return
		}
	}

	// 如果没有风险评估 Agent，根据建议评估
	hasHighRisk := false
	for _, rec := range result.Recommendations {
		if rec.Risk == RiskLevelHigh || rec.Risk == RiskLevelCritical {
			hasHighRisk = true
			break
		}
	}

	if hasHighRisk {
		result.RiskAssessment = &RiskAssessment{
			Level:            RiskLevelHigh,
			Score:            70.0,
			Description:      "存在高风险操作建议，需要人工审批",
			RequiresApproval: true,
		}
	} else {
		result.RiskAssessment = &RiskAssessment{
			Level:            RiskLevelLow,
			Score:            20.0,
			Description:      "低风险操作，可自动执行",
			RequiresApproval: false,
		}
	}
}

// addEvent 添加编排事件。
func (o *Orchestrator) addEvent(taskID string, event *OrchestrationEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events[taskID] = append(o.events[taskID], event)
}

// GetResult 获取编排结果。
func (o *Orchestrator) GetResult(taskID string) (*OrchestrationResult, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	result, exists := o.results[taskID]
	return result, exists
}

// GetEvents 获取编排事件。
func (o *Orchestrator) GetEvents(taskID string) []*OrchestrationEvent {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.events[taskID]
}

// ListResults 列出所有编排结果。
func (o *Orchestrator) ListResults() []*OrchestrationResult {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var results []*OrchestrationResult
	for _, result := range o.results {
		results = append(results, result)
	}
	return results
}

// ApproveTask 批准任务执行。
func (o *Orchestrator) ApproveTask(taskID string, approver string) (*OrchestrationResult, error) {
	result, exists := o.GetResult(taskID)
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	if result.Status != TaskStatusAwaitingApproval {
		return nil, fmt.Errorf("task %s is not awaiting approval, current status: %s", taskID, result.Status)
	}

	result.Status = TaskStatusApproved
	result.UpdatedAt = time.Now()

	o.addEvent(taskID, &OrchestrationEvent{
		Timestamp: time.Now(),
		Type:      "approval_granted",
		Message:   fmt.Sprintf("任务已由 %s 批准", approver),
		Data: map[string]interface{}{
			"approver": approver,
		},
	})

	o.results[taskID] = result
	return result, nil
}

// RejectTask 拒绝任务执行。
func (o *Orchestrator) RejectTask(taskID string, rejector string, reason string) (*OrchestrationResult, error) {
	result, exists := o.GetResult(taskID)
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	result.Status = TaskStatusRejected
	result.UpdatedAt = time.Now()

	o.addEvent(taskID, &OrchestrationEvent{
		Timestamp: time.Now(),
		Type:      "approval_rejected",
		Message:   fmt.Sprintf("任务被 %s 拒绝: %s", rejector, reason),
		Data: map[string]interface{}{
			"rejector": rejector,
			"reason":   reason,
		},
	})

	o.results[taskID] = result
	return result, nil
}
