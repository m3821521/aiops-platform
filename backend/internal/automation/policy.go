package automation

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// PolicyEngine 负责操作前的策略检查。
type PolicyEngine struct {
	// 环境配置（development/test/production）
	environment string
}

func NewPolicyEngine(environment string) *PolicyEngine {
	if environment == "" {
		environment = "development"
	}
	return &PolicyEngine{environment: environment}
}

// PolicyCheck 是策略检查的结果。
type PolicyCheck struct {
	Allowed       bool
	RequireApproval bool
	Reason        string
}

// Check 检查 Action 是否符合策略。
func (p *PolicyEngine) Check(ctx context.Context, action Action) PolicyCheck {
	// 所有写操作都需要审批。
	result := PolicyCheck{
		Allowed:         true,
		RequireApproval: true,
		Reason:          "所有写操作必须经过人工审批",
	}

	// Critical 风险操作额外检查。
	if action.Risk == RiskCritical {
		result.Reason = "Critical 风险操作需要高级管理员审批"
	}

	// Production 环境更严格。
	if p.environment == "production" {
		if action.ActionType == ActionScaleDeployment {
			result.Reason = "生产环境扩容操作需要双人审批"
		}
		if action.ActionType == ActionArgoCDSync {
			result.Reason = "生产环境 ArgoCD Sync 需要双人审批"
		}
	}

	return result
}

// ValidateStateTransition 验证状态跳转是否合法。
func (p *PolicyEngine) ValidateStateTransition(from, to ActionStatus) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("非法状态跳转: %s → %s", from, to)
	}
	return nil
}

// CheckConcurrency 检查是否有并发冲突。
// P0-05.2: DB error 时 fail closed，禁止执行危险 Action。
func (p *PolicyEngine) CheckConcurrency(ctx context.Context, repo *ActionRepository, action Action) error {
	running, err := repo.FindRunningByTarget(ctx, action.TargetType, action.TargetName, action.Cluster)
	if err != nil {
		// gorm.ErrRecordNotFound 表示没有 running action，正常情况
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		// P0-05.2: DB error → fail closed，无法确认资源状态时禁止执行
		return fmt.Errorf("并发检查失败（数据库错误，fail closed）: %w", err)
	}
	if running != nil && running.ID != action.ID {
		return fmt.Errorf("目标资源 %s/%s 已有操作 #%d 正在执行", action.TargetType, action.TargetName, running.ID)
	}
	return nil
}
