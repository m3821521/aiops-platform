package workflow

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// ActionExecutor 执行单个 Action 的接口。
type ActionExecutor interface {
	ExecuteAction(ctx context.Context, actionType, cluster, namespace, targetName string, params map[string]interface{}) (success bool, message string, err error)
}

type Service struct {
	repo     *Repository
	executor ActionExecutor
}

func NewService(repo *Repository, executor ActionExecutor) *Service {
	return &Service{repo: repo, executor: executor}
}

// CreateWorkflow 创建工作流（draft 状态）。
func (s *Service) CreateWorkflow(ctx context.Context, wf *Workflow, userID int64) (*Workflow, error) {
	wf.Status = WorkflowStatusDraft
	wf.CreatedBy = userID
	wf.CreatedAt = time.Now()
	wf.UpdatedAt = time.Now()
	if wf.Risk == "" {
		wf.Risk = "high"
	}
	if err := s.repo.Create(ctx, wf); err != nil {
		return nil, err
	}
	// 创建步骤
	for i := range wf.Steps {
		wf.Steps[i].WorkflowID = wf.ID
		wf.Steps[i].Status = StepStatusPending
		wf.Steps[i].CreatedAt = time.Now()
		wf.Steps[i].UpdatedAt = time.Now()
		if err := s.repo.CreateStep(ctx, &wf.Steps[i]); err != nil {
			return nil, err
		}
	}
	return wf, nil
}

// Submit 提交审批。
func (s *Service) Submit(ctx context.Context, workflowID, userID int64) (*Workflow, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("Workflow 不存在: %w", err)
	}
	if !CanTransition(wf.Status, WorkflowStatusPendingApproval) {
		return nil, fmt.Errorf("状态 %s 不能提交审批", wf.Status)
	}
	wf.Status = WorkflowStatusPendingApproval
	wf.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, wf); err != nil {
		return nil, err
	}
	return wf, nil
}

// Approve 审批通过。
func (s *Service) Approve(ctx context.Context, workflowID, userID int64) (*Workflow, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("Workflow 不存在: %w", err)
	}
	if !CanTransition(wf.Status, WorkflowStatusApproved) {
		return nil, fmt.Errorf("状态 %s 不能审批", wf.Status)
	}
	// 四眼原则
	if wf.CreatedBy == userID {
		return nil, fmt.Errorf("四眼原则：申请人不能审批自己创建的工作流")
	}
	now := time.Now()
	wf.Status = WorkflowStatusApproved
	wf.ApprovedBy = userID
	wf.ApprovedAt = &now
	wf.UpdatedAt = now
	if err := s.repo.Update(ctx, wf); err != nil {
		return nil, err
	}
	return wf, nil
}

// Execute 执行工作流（按顺序执行步骤，支持依赖）。
func (s *Service) Execute(ctx context.Context, workflowID, userID int64) (*Workflow, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("Workflow 不存在: %w", err)
	}
	if wf.Status != WorkflowStatusApproved {
		return nil, fmt.Errorf("只有 approved 状态才能执行，当前: %s", wf.Status)
	}
	if s.executor == nil {
		return nil, fmt.Errorf("Action executor 未配置")
	}

	now := time.Now()
	wf.Status = WorkflowStatusRunning
	wf.StartedAt = &now
	wf.UpdatedAt = now
	s.repo.Update(ctx, wf)

	steps := wf.Steps
	sort.Slice(steps, func(i, j int) bool { return steps[i].Order < steps[j].Order })

	stepResults := make(map[int64]bool)
	allSuccess := true

	for i := range steps {
		step := &steps[i]

		// 检查依赖
		if step.DependsOn > 0 {
			if !stepResults[step.DependsOn] {
				step.Status = StepStatusSkipped
				step.UpdatedAt = time.Now()
				s.repo.UpdateStep(ctx, step)
				continue
			}
		}

		// 执行步骤
		step.Status = StepStatusRunning
		step.StartedAt = &now
		step.UpdatedAt = time.Now()
		s.repo.UpdateStep(ctx, step)

		success, message, execErr := s.executor.ExecuteAction(
			ctx, step.ActionType, step.Cluster, step.Namespace, step.TargetName, step.GetParameters(),
		)

		finished := time.Now()
		step.FinishedAt = &finished
		step.Result = message
		if execErr != nil {
			step.Error = execErr.Error()
		}

		if success {
			step.Status = StepStatusSuccess
			stepResults[step.ID] = true
		} else {
			step.Status = StepStatusFailed
			stepResults[step.ID] = false
			allSuccess = false
			// 后续步骤跳过
			for j := i + 1; j < len(steps); j++ {
				steps[j].Status = StepStatusSkipped
				steps[j].UpdatedAt = time.Now()
				s.repo.UpdateStep(ctx, &steps[j])
			}
			break
		}
		step.UpdatedAt = time.Now()
		s.repo.UpdateStep(ctx, step)
	}

	finished := time.Now()
	wf.FinishedAt = &finished
	wf.DurationMs = finished.Sub(now).Milliseconds()
	if allSuccess {
		wf.Status = WorkflowStatusSuccess
	} else {
		wf.Status = WorkflowStatusFailed
	}
	wf.UpdatedAt = finished
	s.repo.Update(ctx, wf)

	return wf, nil
}

// Cancel 取消工作流。
func (s *Service) Cancel(ctx context.Context, workflowID, userID int64) (*Workflow, error) {
	wf, err := s.repo.FindByID(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	if wf.Status == WorkflowStatusRunning {
		return nil, fmt.Errorf("运行中的工作流不能取消")
	}
	wf.Status = WorkflowStatusCancelled
	wf.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, wf); err != nil {
		return nil, err
	}
	return wf, nil
}
