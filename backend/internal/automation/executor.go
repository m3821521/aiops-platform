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

// JenkinsClientResolver 按 Connection ID 创建 JenkinsClient 的接口。
// 由 providers.Factory 实现，避免循环依赖。
type JenkinsClientResolver interface {
	BuildJenkinsClientByID(ctx context.Context, connectionID int64) (*JenkinsClient, error)
}

// ArgoCDClientResolver 按 Connection ID 创建 ArgoCDClient 的接口。
type ArgoCDClientResolver interface {
	BuildArgoCDClientByID(ctx context.Context, connectionID int64) (*ArgoCDClient, error)
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
	client    *JenkinsClient
	resolver  JenkinsClientResolver
	timeout   time.Duration
}

func NewJenkinsExecutor(client *JenkinsClient) *JenkinsExecutor {
	return &JenkinsExecutor{client: client, timeout: 60 * time.Second}
}

// SetResolver 设置 JenkinsClientResolver，用于按 ConnectionID 获取 Client。
func (e *JenkinsExecutor) SetResolver(resolver JenkinsClientResolver) {
	e.resolver = resolver
}

// getClient 根据 Action.ConnectionID 获取正确的 JenkinsClient。
// 如果 Action 有 ConnectionID，使用指定 Connection；否则使用默认 Client。
func (e *JenkinsExecutor) getClient(ctx context.Context, action Action) (*JenkinsClient, error) {
	if action.ConnectionID != nil && *action.ConnectionID > 0 {
		if e.resolver == nil {
			return nil, fmt.Errorf("jenkins client resolver not configured")
		}
		return e.resolver.BuildJenkinsClientByID(ctx, *action.ConnectionID)
	}
	if e.client == nil {
		return nil, fmt.Errorf("jenkins not configured")
	}
	return e.client, nil
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
	client, err := e.getClient(ctx, action)
	if err != nil {
		return nil, fmt.Errorf("Dry Run 失败: %w", err)
	}

	dryCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// 调用 ListBuilds 检查 Job 是否存在
	builds, err := client.ListBuilds(dryCtx, action.TargetName)
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
	client, err := e.getClient(ctx, action)
	if err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	params := action.GetParameters()
	_ = params // Jenkins Build 参数暂通过 URL 参数传递，当前版本支持无参数构建

	// 1. 触发 Build，获取 Queue Item URL
	queueURL, err := client.Build(execCtx, action.TargetName)
	if err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}

	// 2. 轮询 Queue Item，获取 Build Number
	buildNumber := 0
	for i := 0; i < 30; i++ {
		select {
		case <-execCtx.Done():
			return &ExecutionResult{Success: false, Error: "等待 Jenkins 分配 Build Number 超时"}, nil
		case <-time.After(2 * time.Second):
		}

		item, err := client.GetQueueItem(execCtx, queueURL)
		if err != nil {
			continue
		}
		if item.Executable != nil && item.Executable.Number > 0 {
			buildNumber = item.Executable.Number
			break
		}
	}

	if buildNumber == 0 {
		return &ExecutionResult{
			Success: false,
			Error:   "Jenkins Build 已触发但未能获取 Build Number（可能仍在 Queue 中等待）",
		}, nil
	}

	// 3. 轮询 Build 状态，直到构建完成
	var finalBuild *JenkinsBuild
	for i := 0; i < 60; i++ {
		select {
		case <-execCtx.Done():
			return &ExecutionResult{
				Success: false,
				Error:   fmt.Sprintf("Jenkins Build #%d 等待完成超时", buildNumber),
			}, nil
		case <-time.After(3 * time.Second):
		}

		build, err := client.GetBuild(execCtx, action.TargetName, buildNumber)
		if err != nil {
			continue
		}
		if !build.Building {
			finalBuild = build
			break
		}
	}

	if finalBuild == nil {
		return &ExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("Jenkins Build #%d 仍在构建中（超时）", buildNumber),
		}, nil
	}

	// 4. 根据 Build 结果返回
	switch finalBuild.Result {
	case "SUCCESS":
		return &ExecutionResult{
			Success:    true,
			Message:    fmt.Sprintf("Jenkins Job %s Build #%d 成功", action.TargetName, buildNumber),
			ExternalID: fmt.Sprintf("%d", buildNumber),
		}, nil
	case "FAILURE":
		return &ExecutionResult{
			Success:    false,
			Error:      fmt.Sprintf("Jenkins Job %s Build #%d 失败", action.TargetName, buildNumber),
			ExternalID: fmt.Sprintf("%d", buildNumber),
		}, nil
	case "ABORTED":
		return &ExecutionResult{
			Success:    false,
			Error:      fmt.Sprintf("Jenkins Job %s Build #%d 被中止", action.TargetName, buildNumber),
			ExternalID: fmt.Sprintf("%d", buildNumber),
		}, nil
	default:
		return &ExecutionResult{
			Success:    false,
			Error:      fmt.Sprintf("Jenkins Job %s Build #%d 未知状态: %s", action.TargetName, buildNumber, finalBuild.Result),
			ExternalID: fmt.Sprintf("%d", buildNumber),
		}, nil
	}
}

// Verify 验证 Jenkins 构建是否成功。
func (e *JenkinsExecutor) Verify(ctx context.Context, action Action, result *ExecutionResult) (*VerificationResult, error) {
	if result == nil || !result.Success {
		return &VerificationResult{
			Verified: false,
			Status:   "failed",
			Message:  "执行失败，无需验证",
		}, nil
	}
	if result.ExternalID == "" {
		return &VerificationResult{
			Verified: false,
			Status:   "unverified",
			Message:  "无法验证：缺少 Build Number",
		}, nil
	}
	client, err := e.getClient(ctx, action)
	if err != nil {
		return &VerificationResult{
			Verified: false,
			Status:   "unverified",
			Message:  fmt.Sprintf("无法验证：%v", err),
		}, nil
	}

	// 从 ExternalID 解析 Build Number
	var buildNumber int
	fmt.Sscanf(result.ExternalID, "%d", &buildNumber)
	if buildNumber == 0 {
		return &VerificationResult{
			Verified: false,
			Status:   "unverified",
			Message:  "无法解析 Build Number",
		}, nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	build, err := client.GetBuild(verifyCtx, action.TargetName, buildNumber)
	if err != nil {
		return &VerificationResult{
			Verified: false,
			Status:   "error",
			Message:  fmt.Sprintf("查询 Jenkins Build 失败: %v", err),
		}, nil
	}

	if build.Building {
		return &VerificationResult{
			Verified: false,
			Status:   "building",
			Message:  fmt.Sprintf("Jenkins Build #%d 仍在构建中", buildNumber),
		}, nil
	}

	if build.Result == "SUCCESS" {
		return &VerificationResult{
			Verified: true,
			Status:   "success",
			Message:  fmt.Sprintf("Jenkins Build #%d 成功，耗时 %dms", buildNumber, build.Duration),
		}, nil
	}

	return &VerificationResult{
		Verified: false,
		Status:   build.Result,
		Message:  fmt.Sprintf("Jenkins Build #%d 状态: %s", buildNumber, build.Result),
	}, nil
}

// ===== ArgoCD Executor =====

type ArgoCDExecutor struct {
	client   *ArgoCDClient
	resolver ArgoCDClientResolver
	timeout  time.Duration
}

func NewArgoCDExecutor(client *ArgoCDClient) *ArgoCDExecutor {
	return &ArgoCDExecutor{client: client, timeout: 120 * time.Second}
}

// SetResolver 设置 ArgoCDClientResolver，用于按 ConnectionID 获取 Client。
func (e *ArgoCDExecutor) SetResolver(resolver ArgoCDClientResolver) {
	e.resolver = resolver
}

// getClient 根据 Action.ConnectionID 获取正确的 ArgoCDClient。
func (e *ArgoCDExecutor) getClient(ctx context.Context, action Action) (*ArgoCDClient, error) {
	if action.ConnectionID != nil && *action.ConnectionID > 0 {
		if e.resolver == nil {
			return nil, fmt.Errorf("argocd client resolver not configured")
		}
		return e.resolver.BuildArgoCDClientByID(ctx, *action.ConnectionID)
	}
	if e.client == nil {
		return nil, fmt.Errorf("argocd not configured")
	}
	return e.client, nil
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
	client, err := e.getClient(ctx, action)
	if err != nil {
		return nil, fmt.Errorf("Dry Run 失败: %w", err)
	}

	dryCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// 调用 GetApplication 检查 Application 是否存在
	app, err := client.GetApplication(dryCtx, action.TargetName)
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
	client, err := e.getClient(ctx, action)
	if err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// 1. 触发 Sync
	if err := client.Sync(execCtx, action.TargetName); err != nil {
		return &ExecutionResult{Success: false, Error: err.Error()}, nil
	}

	// 2. 轮询 Application 状态，直到 Sync 完成且 Health 恢复
	var finalApp *ArgoApplication
	for i := 0; i < 40; i++ {
		select {
		case <-execCtx.Done():
			return &ExecutionResult{
				Success: false,
				Error:   fmt.Sprintf("ArgoCD Application %s Sync 等待超时", action.TargetName),
			}, nil
		case <-time.After(3 * time.Second):
		}

		app, err := client.GetApplication(execCtx, action.TargetName)
		if err != nil {
			continue
		}

		syncStatus := app.Status.Sync.Status
		healthStatus := app.Status.Health.Status

		// Sync 完成且 Health Healthy 或 Progressing（允许 Progressing 作为中间状态）
		if syncStatus == "Synced" && (healthStatus == "Healthy" || healthStatus == "Progressing") {
			finalApp = app
			break
		}

		// 如果 Health Degraded 且已经过了足够时间，可能 Sync 失败
		if healthStatus == "Degraded" && i > 10 {
			finalApp = app
			break
		}
	}

	if finalApp == nil {
		return &ExecutionResult{
			Success: false,
			Error:   fmt.Sprintf("ArgoCD Application %s Sync 后状态未稳定（超时）", action.TargetName),
		}, nil
	}

	syncStatus := finalApp.Status.Sync.Status
	healthStatus := finalApp.Status.Health.Status

	// 3. 根据最终状态返回
	if syncStatus == "Synced" && healthStatus == "Healthy" {
		return &ExecutionResult{
			Success:    true,
			Message:    fmt.Sprintf("ArgoCD Application %s Sync 成功，Health: Healthy", action.TargetName),
			ExternalID: action.TargetName,
		}, nil
	}

	if syncStatus == "Synced" && healthStatus == "Progressing" {
		return &ExecutionResult{
			Success:    true,
			Message:    fmt.Sprintf("ArgoCD Application %s Sync 成功，Health: Progressing（部署进行中）", action.TargetName),
			ExternalID: action.TargetName,
		}, nil
	}

	return &ExecutionResult{
		Success:    false,
		Error:      fmt.Sprintf("ArgoCD Application %s Sync 后状态异常: Sync=%s, Health=%s", action.TargetName, syncStatus, healthStatus),
		ExternalID: action.TargetName,
	}, nil
}

// Verify 验证 ArgoCD 同步是否成功。
func (e *ArgoCDExecutor) Verify(ctx context.Context, action Action, result *ExecutionResult) (*VerificationResult, error) {
	if result == nil || !result.Success {
		return &VerificationResult{
			Verified: false,
			Status:   "failed",
			Message:  "执行失败，无需验证",
		}, nil
	}
	client, err := e.getClient(ctx, action)
	if err != nil {
		return &VerificationResult{
			Verified: false,
			Status:   "unverified",
			Message:  fmt.Sprintf("无法验证：%v", err),
		}, nil
	}

	verifyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	app, err := client.GetApplication(verifyCtx, action.TargetName)
	if err != nil {
		return &VerificationResult{
			Verified: false,
			Status:   "error",
			Message:  fmt.Sprintf("查询 ArgoCD Application 失败: %v", err),
		}, nil
	}

	syncStatus := app.Status.Sync.Status
	healthStatus := app.Status.Health.Status

	if syncStatus == "Synced" && healthStatus == "Healthy" {
		return &VerificationResult{
			Verified: true,
			Status:   "success",
			Message:  fmt.Sprintf("ArgoCD Application %s: Synced + Healthy", action.TargetName),
		}, nil
	}

	if syncStatus == "Synced" && healthStatus == "Progressing" {
		return &VerificationResult{
			Verified: false,
			Status:   "progressing",
			Message:  fmt.Sprintf("ArgoCD Application %s: Synced, Health Progressing（部署进行中）", action.TargetName),
		}, nil
	}

	return &VerificationResult{
		Verified: false,
		Status:   healthStatus,
		Message:  fmt.Sprintf("ArgoCD Application %s: Sync=%s, Health=%s", action.TargetName, syncStatus, healthStatus),
	}, nil
}
