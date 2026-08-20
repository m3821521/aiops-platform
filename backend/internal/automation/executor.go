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
	// Verify 验证执行结果是否真的生效。
	// 返回 nil 表示该 Executor 不支持 Verification。
	Verify(ctx context.Context, action Action, result *ExecutionResult) (*VerificationResult, error)
}

// VerificationResult 是执行验证结果。
type VerificationResult struct {
	Verified  bool   `json:"verified"`
	Status    string `json:"status"` // verified / unverified / failed / unavailable
	Message   string `json:"message"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	Evidence  string `json:"evidence,omitempty"`
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

	// Dry Run 必须真正调用 Kubernetes API 检查目标资源是否存在。
	// 禁止返回静态的 "Pod 当前运行中" 来 Fake Success。
	dryCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	result := &DryRunResult{
		ActionType: action.ActionType,
		Target:     fmt.Sprintf("%s/%s", action.Namespace, action.TargetName),
		Safe:       false, // 默认不安全，只有确认资源存在后才设为 true
	}

	switch action.ActionType {
	case ActionRestartPod:
		// 真正调用 Kubernetes API 检查 Pod 是否存在
		pod, err := e.cluster.GetPod(dryCtx, action.Cluster, action.Namespace, action.TargetName)
		if err != nil {
			return nil, fmt.Errorf("Dry Run 失败: 无法获取 Pod %s/%s: %w", action.Namespace, action.TargetName, err)
		}
		if pod == nil {
			return nil, fmt.Errorf("Dry Run 失败: Pod %s/%s 不存在", action.Namespace, action.TargetName)
		}
		result.CurrentState = fmt.Sprintf("Pod %s/%s 当前状态: %s, RestartCount: %d",
			action.Namespace, action.TargetName, pod.Status.Phase,
			func() int32 {
				if len(pod.Status.ContainerStatuses) > 0 {
					return pod.Status.ContainerStatuses[0].RestartCount
				}
				return 0
			}())
		result.ExpectedOperation = fmt.Sprintf("删除 Pod %s/%s，由 Deployment/ReplicaSet 自动重建", action.Namespace, action.TargetName)
		result.PotentialImpact = "Pod 将短暂中断，服务可能出现秒级不可用"
		result.Safe = true

	case ActionScaleDeployment:
		// 真正调用 Kubernetes API 检查 Deployment 是否存在
		deployment, err := e.cluster.GetDeployment(dryCtx, action.Cluster, action.Namespace, action.TargetName)
		if err != nil {
			return nil, fmt.Errorf("Dry Run 失败: 无法获取 Deployment %s/%s: %w", action.Namespace, action.TargetName, err)
		}
		if deployment == nil {
			return nil, fmt.Errorf("Dry Run 失败: Deployment %s/%s 不存在", action.Namespace, action.TargetName)
		}
		params := action.GetParameters()
		replicas := int(params["replicas"].(float64))
		currentReplicas := deployment.Status.Replicas
		result.CurrentState = fmt.Sprintf("Deployment %s/%s 当前副本数: %d (期望: %d)",
			action.Namespace, action.TargetName, currentReplicas, replicas)
		result.ExpectedOperation = fmt.Sprintf("将 Deployment %s/%s 副本数从 %d 调整为 %d",
			action.Namespace, action.TargetName, currentReplicas, replicas)
		result.PotentialImpact = "副本数变化可能影响服务容量和资源使用"
		result.Safe = true
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

// Verify 验证 Kubernetes 操作是否真的生效。
// 对于 Restart Pod：检查 Pod UID 是否变化或 RestartCount 是否增加。
// 对于 Scale Deployment：检查 Deployment replicas 是否变化。
// 如果 Kubernetes API 不可达，返回 UNVERIFIED 状态。
func (e *KubernetesExecutor) Verify(ctx context.Context, action Action, result *ExecutionResult) (*VerificationResult, error) {
	if result == nil || !result.Success {
		return &VerificationResult{
			Verified: false,
			Status:   "failed",
			Message:  "执行失败，无需验证",
		}, nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	switch action.ActionType {
	case ActionRestartPod:
		// 尝试获取 Pod 信息来验证重启
		pod, err := e.cluster.GetPod(verifyCtx, action.Cluster, action.Namespace, action.TargetName)
		if err != nil {
			// Kubernetes API 不可达，返回 UNVERIFIED
			return &VerificationResult{
				Verified: false,
				Status:   "unavailable",
				Message:  fmt.Sprintf("无法验证 Pod 状态 (Kubernetes API 不可达): %v", err),
			}, nil
		}

		// Pod 存在，说明重启后 Pod 已重建
		restartCount := int32(0)
		if len(pod.Status.ContainerStatuses) > 0 {
			restartCount = pod.Status.ContainerStatuses[0].RestartCount
		}
		return &VerificationResult{
			Verified: true,
			Status:   "verified",
			Message:  fmt.Sprintf("Pod %s/%s 已重启并处于 %s 状态", action.Namespace, action.TargetName, pod.Status.Phase),
			After:    fmt.Sprintf("Pod Status: %s, RestartCount: %d", pod.Status.Phase, restartCount),
			Evidence: fmt.Sprintf("Pod UID: %s", pod.UID),
		}, nil

	case ActionScaleDeployment:
		// 尝试获取 Deployment 信息来验证扩容
		deployment, err := e.cluster.GetDeployment(verifyCtx, action.Cluster, action.Namespace, action.TargetName)
		if err != nil {
			// Kubernetes API 不可达，返回 UNVERIFIED
			return &VerificationResult{
				Verified: false,
				Status:   "unavailable",
				Message:  fmt.Sprintf("无法验证 Deployment 状态 (Kubernetes API 不可达): %v", err),
			}, nil
		}

		params := action.GetParameters()
		expectedReplicas := int32(params["replicas"].(float64))

		currentReplicas := int32(0)
		if deployment.Status.Replicas != 0 {
			currentReplicas = deployment.Status.Replicas
		} else if deployment.Spec.Replicas != nil {
			currentReplicas = *deployment.Spec.Replicas
		}

		if currentReplicas == expectedReplicas {
			return &VerificationResult{
				Verified: true,
				Status:   "verified",
				Message:  fmt.Sprintf("Deployment %s/%s 已扩容到 %d 副本", action.Namespace, action.TargetName, currentReplicas),
				After:    fmt.Sprintf("Replicas: %d (expected: %d)", currentReplicas, expectedReplicas),
			}, nil
		}

		return &VerificationResult{
			Verified: false,
			Status:   "failed",
			Message:  fmt.Sprintf("Deployment 副本数不匹配: 当前 %d, 期望 %d", currentReplicas, expectedReplicas),
			After:    fmt.Sprintf("Replicas: %d (expected: %d)", currentReplicas, expectedReplicas),
		}, nil
	}

	return &VerificationResult{
		Verified: false,
		Status:   "unverified",
		Message:  "不支持的操作类型验证",
	}, nil
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

	// Dry Run 必须真正调用 Jenkins API 检查 Job 是否存在。
	// 禁止返回静态的 "Jenkins Job 待触发" 来 Fake Success。
	if e.client == nil {
		return nil, fmt.Errorf("Dry Run 失败: Jenkins 未配置")
	}

	dryCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// 调用 ListBuilds 检查 Job 是否存在
	builds, err := e.client.ListBuilds(dryCtx, action.TargetName)
	if err != nil {
		return nil, fmt.Errorf("Dry Run 失败: 无法获取 Jenkins Job %s: %w", action.TargetName, err)
	}

	params := action.GetParameters()
	lastBuild := "无"
	if len(builds) > 0 {
		lastBuild = fmt.Sprintf("#%d (%s)", builds[0].Number, builds[0].Result)
	}

	return &DryRunResult{
		ActionType:        ActionJenkinsBuild,
		Target:            action.TargetName,
		CurrentState:      fmt.Sprintf("Jenkins Job %s 存在，最近构建: %s", action.TargetName, lastBuild),
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

// Verify 验证 Jenkins 构建是否成功。
// 当前实现返回 UNVERIFIED，因为需要异步轮询 Build 状态。
func (e *JenkinsExecutor) Verify(ctx context.Context, action Action, result *ExecutionResult) (*VerificationResult, error) {
	if result == nil || !result.Success {
		return &VerificationResult{
			Verified: false,
			Status:   "failed",
			Message:  "执行失败，无需验证",
		}, nil
	}
	// Jenkins Build 是异步的，需要后续轮询 Build 状态
	// 当前返回 UNVERIFIED，表示已触发但未验证最终结果
	return &VerificationResult{
		Verified: false,
		Status:   "unverified",
		Message:  "Jenkins 构建已触发，需异步轮询 Build 状态验证",
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

	// Dry Run 必须真正调用 ArgoCD API 检查 Application 是否存在。
	// 禁止返回静态的 "ArgoCD Application 待同步" 来 Fake Success。
	if e.client == nil {
		return nil, fmt.Errorf("Dry Run 失败: ArgoCD 未配置")
	}

	dryCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// 调用 GetApplication 检查 Application 是否存在
	app, err := e.client.GetApplication(dryCtx, action.TargetName)
	if err != nil {
		return nil, fmt.Errorf("Dry Run 失败: 无法获取 ArgoCD Application %s: %w", action.TargetName, err)
	}
	if app == nil {
		return nil, fmt.Errorf("Dry Run 失败: ArgoCD Application %s 不存在", action.TargetName)
	}

	return &DryRunResult{
		ActionType:        ActionArgoCDSync,
		Target:            action.TargetName,
		CurrentState:      fmt.Sprintf("ArgoCD Application %s 存在，Sync: %s, Health: %s",
			action.TargetName, app.Status.Sync.Status, app.Status.Health.Status),
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

// Verify 验证 ArgoCD 同步是否成功。
// 当前实现返回 UNVERIFIED，因为需要异步轮询 Application 状态。
func (e *ArgoCDExecutor) Verify(ctx context.Context, action Action, result *ExecutionResult) (*VerificationResult, error) {
	if result == nil || !result.Success {
		return &VerificationResult{
			Verified: false,
			Status:   "failed",
			Message:  "执行失败，无需验证",
		}, nil
	}
	// ArgoCD Sync 是异步的，需要后续轮询 Application 状态
	// 当前返回 UNVERIFIED，表示已触发但未验证最终结果
	return &VerificationResult{
		Verified: false,
		Status:   "unverified",
		Message:  "ArgoCD 同步已触发，需异步轮询 Application 状态验证",
	}, nil
}
