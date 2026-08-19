package automation

import (
	"context"
	"fmt"
	"time"

	"github.com/aiops/aiops-platform/internal/cluster"
)

// Executor 是操作执行器的统一接口。
type Executor interface {
	Type() string
	Validate(ctx context.Context, action Action) error
	DryRun(ctx context.Context, action Action) (*DryRunResult, error)
	Execute(ctx context.Context, action Action) (*ExecutionResult, error)
}

// ===== Kubernetes Executor =====

type KubernetesExecutor struct {
	cluster *cluster.Service
	timeout time.Duration
}

func NewKubernetesExecutor(c *cluster.Service) *KubernetesExecutor {
	return &KubernetesExecutor{cluster: c, timeout: 30 * time.Second}
}

func (e *KubernetesExecutor) Type() string { return "kubernetes" }

func (e *KubernetesExecutor) Validate(ctx context.Context, action Action) error {
	if action.Cluster == "" {
		return fmt.Errorf("cluster 不能为空")
	}
	if action.TargetName == "" {
		return fmt.Errorf("target_name 不能为空")
	}
	switch action.ActionType {
	case ActionRestartPod:
		if action.Namespace == "" {
			return fmt.Errorf("restart_pod 需要 namespace")
		}
	case ActionScaleDeployment:
		if action.Namespace == "" {
			return fmt.Errorf("scale_deployment 需要 namespace")
		}
		params := action.GetParameters()
		if params == nil || params["replicas"] == nil {
			return fmt.Errorf("scale_deployment 需要 replicas 参数")
		}
	default:
		return fmt.Errorf("KubernetesExecutor 不支持操作类型: %s", action.ActionType)
	}
	return nil
}

func (e *KubernetesExecutor) DryRun(ctx context.Context, action Action) (*DryRunResult, error) {
	if err := e.Validate(ctx, action); err != nil {
		return nil, err
	}
	result := &DryRunResult{
		ActionType: action.ActionType,
		Target:     fmt.Sprintf("%s/%s", action.Namespace, action.TargetName),
		Safe:       true,
	}
	switch action.ActionType {
	case ActionRestartPod:
		result.CurrentState = fmt.Sprintf("Pod %s/%s 当前运行中", action.Namespace, action.TargetName)
		result.ExpectedOperation = fmt.Sprintf("删除 Pod %s/%s，由 Deployment/ReplicaSet 自动重建", action.Namespace, action.TargetName)
		result.PotentialImpact = "Pod 将短暂中断，服务可能出现秒级不可用"
	case ActionScaleDeployment:
		params := action.GetParameters()
		replicas := int(params["replicas"].(float64))
		result.CurrentState = fmt.Sprintf("Deployment %s/%s 当前副本数待查询", action.Namespace, action.TargetName)
		result.ExpectedOperation = fmt.Sprintf("将 Deployment %s/%s 副本数调整为 %d", action.Namespace, action.TargetName, replicas)
		result.PotentialImpact = "副本数变化可能影响服务容量和资源使用"
	}
	return result, nil
}

func (e *KubernetesExecutor) Execute(ctx context.Context, action Action) (*ExecutionResult, error) {
	if err := e.Validate(ctx, action); err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	switch action.ActionType {
	case ActionRestartPod:
		err := e.cluster.RestartPod(execCtx, action.Cluster, action.Namespace, action.TargetName)
		if err != nil {
			return &ExecutionResult{Success: false, Error: err.Error()}, nil
		}
		return &ExecutionResult{Success: true, Message: fmt.Sprintf("Pod %s/%s 已重启", action.Namespace, action.TargetName)}, nil
	case ActionScaleDeployment:
		params := action.GetParameters()
		replicas := int32(params["replicas"].(float64))
		err := e.cluster.ScaleDeployment(execCtx, action.Cluster, action.Namespace, action.TargetName, replicas)
		if err != nil {
			return &ExecutionResult{Success: false, Error: err.Error()}, nil
		}
		return &ExecutionResult{Success: true, Message: fmt.Sprintf("Deployment %s/%s 已扩容到 %d 副本", action.Namespace, action.TargetName, replicas)}, nil
	}
	return &ExecutionResult{Success: false, Error: "不支持的操作类型"}, nil
}

// ===== Jenkins Executor =====

type JenkinsExecutor struct {
	client  *JenkinsClient
	timeout time.Duration
}

func NewJenkinsExecutor(client *JenkinsClient) *JenkinsExecutor {
	return &JenkinsExecutor{client: client, timeout: 60 * time.Second}
}

func (e *JenkinsExecutor) Type() string { return "jenkins" }

func (e *JenkinsExecutor) Validate(ctx context.Context, action Action) error {
	if action.ActionType != ActionJenkinsBuild {
		return fmt.Errorf("JenkinsExecutor 只支持 jenkins_build")
	}
	if action.TargetName == "" {
		return fmt.Errorf("job 名称不能为空")
	}
	return nil
}

func (e *JenkinsExecutor) DryRun(ctx context.Context, action Action) (*DryRunResult, error) {
	if err := e.Validate(ctx, action); err != nil {
		return nil, err
	}
	params := action.GetParameters()
	return &DryRunResult{
		ActionType:        ActionJenkinsBuild,
		Target:            action.TargetName,
		CurrentState:      fmt.Sprintf("Jenkins Job %s 待触发", action.TargetName),
		ExpectedOperation: fmt.Sprintf("触发 Jenkins Job %s，参数: %v", action.TargetName, params),
		PotentialImpact:   "将启动一次新的构建，可能占用构建资源",
		Safe:              true,
	}, nil
}

func (e *JenkinsExecutor) Execute(ctx context.Context, action Action) (*ExecutionResult, error) {
	if err := e.Validate(ctx, action); err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}
	if e.client == nil {
		return &ExecutionResult{Success: false, Error: "Jenkins 未配置"}, nil
	}
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	params := action.GetParameters()
	_ = params // Jenkins Build 暂不支持参数传递

	err := e.client.Build(execCtx, action.TargetName)
	if err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}
	return &ExecutionResult{
		Success: true,
		Message: fmt.Sprintf("Jenkins Job %s 已触发构建", action.TargetName),
	}, nil
}

// ===== ArgoCD Executor =====

type ArgoCDExecutor struct {
	client  *ArgoCDClient
	timeout time.Duration
}

func NewArgoCDExecutor(client *ArgoCDClient) *ArgoCDExecutor {
	return &ArgoCDExecutor{client: client, timeout: 120 * time.Second}
}

func (e *ArgoCDExecutor) Type() string { return "argocd" }

func (e *ArgoCDExecutor) Validate(ctx context.Context, action Action) error {
	if action.ActionType != ActionArgoCDSync {
		return fmt.Errorf("ArgoCDExecutor 只支持 argocd_sync")
	}
	if action.TargetName == "" {
		return fmt.Errorf("application 名称不能为空")
	}
	return nil
}

func (e *ArgoCDExecutor) DryRun(ctx context.Context, action Action) (*DryRunResult, error) {
	if err := e.Validate(ctx, action); err != nil {
		return nil, err
	}
	return &DryRunResult{
		ActionType:        ActionArgoCDSync,
		Target:            action.TargetName,
		CurrentState:      fmt.Sprintf("ArgoCD Application %s 待同步", action.TargetName),
		ExpectedOperation: fmt.Sprintf("同步 ArgoCD Application %s 到最新 Git 修订版本", action.TargetName),
		PotentialImpact:   "将触发 Kubernetes 资源更新，可能导致服务滚动重启",
		Safe:              true,
	}, nil
}

func (e *ArgoCDExecutor) Execute(ctx context.Context, action Action) (*ExecutionResult, error) {
	if err := e.Validate(ctx, action); err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}
	if e.client == nil {
		return &ExecutionResult{Success: false, Error: "ArgoCD 未配置"}, nil
	}
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	if err := e.client.Sync(execCtx, action.TargetName); err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}
	return &ExecutionResult{
		Success: true,
		Message: fmt.Sprintf("ArgoCD Application %s 同步已触发", action.TargetName),
	}, nil
}
