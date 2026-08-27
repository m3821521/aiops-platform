package signals

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/aiops/aiops-platform/internal/rca"
)

// K8sReader 是 KubernetesSignalCollector 所需的最小 K8s 读接口。
// 真实实现为 *cluster.Service。定义在 signals 包内，避免修改 cluster.Service。
// 使用 List 方法而非 Get，因为 cluster.Service 只提供了 GetDeployment，
// 没有 GetStatefulSet/GetDaemonSet。List 后在内存中按 name 过滤。
type K8sReader interface {
	ListDeployments(ctx context.Context, cluster, namespace string) ([]appsv1.Deployment, error)
	ListStatefulSets(ctx context.Context, cluster, namespace string) ([]appsv1.StatefulSet, error)
	ListDaemonSets(ctx context.Context, cluster, namespace string) ([]appsv1.DaemonSet, error)
}

// KubernetesSignalCollector 从 Kubernetes Pod Status 和 Workload Status 采集 Health Signals。
//
// 支持的 Signal 类型：
//   - pod_readiness: Pod Ready 条件（正常/异常）
//   - pod_restart: 容器重启次数超过阈值
//   - crashloop: CrashLoopBackOff
//   - oomkilled: OOMKilled（lastState.terminated.reason）
//   - image_pull: ImagePullBackOff / ErrImagePull
//   - deployment_availability: Deployment ReadyReplicas / Replicas
//   - statefulset_availability: StatefulSet ReadyReplicas / Replicas
//   - daemonset_availability: DaemonSet NumberReady / DesiredNumberScheduled
//
// 原则：
//   - 正常 Pod 产生 level=context 的 readiness signal（不是 error）。
//   - 异常 Pod 产生 level=corroborating 或 direct 的 signal。
//   - OOMKilled / ImagePullBackOff 是 direct evidence（与 RCA Pipeline 一致）。
//   - CrashLoopBackOff 是 corroborating（不能单独确认根因）。
type KubernetesSignalCollector struct {
	k8s K8sReader
	// restartThreshold 是重启次数阈值，超过此值产生 pod_restart signal。
	restartThreshold int32
}

// NewKubernetesSignalCollector 创建 KubernetesSignalCollector。
// restartThreshold 默认为 3（<=0 时使用默认值）。
func NewKubernetesSignalCollector(k8s K8sReader, restartThreshold int32) *KubernetesSignalCollector {
	if restartThreshold <= 0 {
		restartThreshold = 3
	}
	return &KubernetesSignalCollector{k8s: k8s, restartThreshold: restartThreshold}
}

// Source 实现 SignalCollector 接口。
func (c *KubernetesSignalCollector) Source() string { return "kubernetes" }

// Collect 实现 SignalCollector 接口。
// 从 pods 中提取 Pod 级 signals，并根据 Workload 类型获取 Workload availability。
func (c *KubernetesSignalCollector) Collect(ctx context.Context, svc ServiceContext, pods []corev1.Pod) ([]rca.Evidence, error) {
	fetchedAt := time.Now()
	var evidences []rca.Evidence

	// 1. Pod 级 signals（从已获取的 pods 中提取，无额外 K8s API 调用）
	for i := range pods {
		pod := &pods[i]
		evidences = append(evidences, c.collectPodSignals(svc, pod, fetchedAt)...)
	}

	// 2. Workload availability（根据 Workload 类型获取，单次 Get 调用）
	wlEvidences, err := c.collectWorkloadAvailability(ctx, svc, fetchedAt)
	if err != nil {
		// Workload 获取失败不阻塞 Pod signals，但记录 error
		// 这里返回 error 会让 Manager 将 kubernetes 标记为 source_error
		// 但 Pod signals 已经采集到，应该是 partial success
		// 为简化：如果 Pod signals 存在，忽略 workload error；如果没有 Pod signals，返回 error
		if len(evidences) == 0 {
			return nil, fmt.Errorf("workload availability failed: %w", err)
		}
		// Pod signals 存在，workload error 降级为 warning（不进入 source_errors）
		// 后续可通过独立 signal 表达
	}
	evidences = append(evidences, wlEvidences...)

	return evidences, nil
}

// collectPodSignals 从单个 Pod 提取所有 Pod 级 Health Signals。
func (c *KubernetesSignalCollector) collectPodSignals(svc ServiceContext, pod *corev1.Pod, fetchedAt time.Time) []rca.Evidence {
	var evidences []rca.Evidence

	// Pod readiness
	ready := isPodReady(pod)
	readinessLevel := rca.EvidenceLevelContext
	readinessCausal := "contextual"
	readinessScore := 0.1
	if !ready {
		readinessLevel = rca.EvidenceLevelCorroborating
		readinessCausal = "supporting"
		readinessScore = 0.5
	}
	evidences = append(evidences, rca.Evidence{
		ID:            fmt.Sprintf("pod-readiness-%s-%s-%s", svc.Cluster, svc.Namespace, pod.Name),
		Type:          "pod_readiness",
		Level:         readinessLevel,
		Source:        "kubernetes",
		SourceType:    "provider",
		Timestamp:     pod.CreationTimestamp.Time,
		ResourceType:  "pod",
		ResourceName:  pod.Name,
		Namespace:     pod.Namespace,
		Description:   fmt.Sprintf("Pod %s readiness=%v phase=%s", pod.Name, ready, pod.Status.Phase),
		Score:         readinessScore,
		FetchedAt:     &fetchedAt,
		DataTimestamp: timePtr(pod.CreationTimestamp.Time),
		TimestampAvailable: !pod.CreationTimestamp.IsZero(),
		TrustStatus:        "fresh",
		CausalRelevance:    readinessCausal,
	})

	// Container 级 signals
	for _, cs := range pod.Status.ContainerStatuses {
		// Restart count
		if cs.RestartCount >= c.restartThreshold {
			evidences = append(evidences, rca.Evidence{
				ID:            fmt.Sprintf("pod-restart-%s-%s-%s", svc.Cluster, svc.Namespace, pod.Name),
				Type:          "pod_restart",
				Level:         rca.EvidenceLevelCorroborating,
				Source:        "kubernetes",
				SourceType:    "provider",
				Timestamp:     pod.CreationTimestamp.Time,
				ResourceType:  "pod",
				ResourceName:  pod.Name,
				Namespace:     pod.Namespace,
				Value:         float64(cs.RestartCount),
				Description:   fmt.Sprintf("Container %s restarted %d times", cs.Name, cs.RestartCount),
				Score:         0.4,
				FetchedAt:     &fetchedAt,
				DataTimestamp: timePtr(pod.CreationTimestamp.Time),
				TimestampAvailable: !pod.CreationTimestamp.IsZero(),
				TrustStatus:        "fresh",
				CausalRelevance:    "supporting",
			})
		}

		// CrashLoopBackOff
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
			evidences = append(evidences, rca.Evidence{
				ID:            fmt.Sprintf("crashloop-%s-%s-%s-%s", svc.Cluster, svc.Namespace, pod.Name, cs.Name),
				Type:          "crashloop",
				Level:         rca.EvidenceLevelCorroborating,
				Source:        "kubernetes",
				SourceType:    "provider",
				Timestamp:     pod.CreationTimestamp.Time,
				ResourceType:  "pod",
				ResourceName:  pod.Name,
				Namespace:     pod.Namespace,
				Description:   fmt.Sprintf("Container %s in CrashLoopBackOff: %s", cs.Name, cs.State.Waiting.Message),
				Score:         0.6,
				FetchedAt:     &fetchedAt,
				DataTimestamp: timePtr(pod.CreationTimestamp.Time),
				TimestampAvailable: !pod.CreationTimestamp.IsZero(),
				TrustStatus:        "fresh",
				CausalRelevance:    "supporting",
			})
		}

		// OOMKilled（lastTerminationState.terminated.reason）
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
			term := cs.LastTerminationState.Terminated
			evidences = append(evidences, rca.Evidence{
				ID:            fmt.Sprintf("oomkilled-%s-%s-%s-%s-%d", svc.Cluster, svc.Namespace, pod.Name, cs.Name, term.FinishedAt.Unix()),
				Type:          "oomkilled",
				Level:         rca.EvidenceLevelDirect,
				Source:        "kubernetes",
				SourceType:    "provider",
				Timestamp:     term.FinishedAt.Time,
				ResourceType:  "pod",
				ResourceName:  pod.Name,
				Namespace:     pod.Namespace,
				Value:         float64(term.ExitCode),
				Description:   fmt.Sprintf("Container %s was OOMKilled (exit code %d)", cs.Name, term.ExitCode),
				Score:         0.9,
				FetchedAt:     &fetchedAt,
				DataTimestamp: timePtr(term.FinishedAt.Time),
				TimestampAvailable: !term.FinishedAt.IsZero(),
				TrustStatus:        "fresh",
				CausalRelevance:    "direct_causal",
			})
		}

		// ImagePullBackOff / ErrImagePull
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			if reason == "ImagePullBackOff" || reason == "ErrImagePull" {
				evidences = append(evidences, rca.Evidence{
					ID:            fmt.Sprintf("image-pull-%s-%s-%s-%s", svc.Cluster, svc.Namespace, pod.Name, cs.Name),
					Type:          "image_pull",
					Level:         rca.EvidenceLevelDirect,
					Source:        "kubernetes",
					SourceType:    "provider",
					Timestamp:     pod.CreationTimestamp.Time,
					ResourceType:  "pod",
					ResourceName:  pod.Name,
					Namespace:     pod.Namespace,
					Description:   fmt.Sprintf("Container %s image pull failed: %s - %s", cs.Name, reason, cs.State.Waiting.Message),
					Score:         0.85,
					FetchedAt:     &fetchedAt,
					DataTimestamp: timePtr(pod.CreationTimestamp.Time),
					TimestampAvailable: !pod.CreationTimestamp.IsZero(),
					TrustStatus:        "fresh",
					CausalRelevance:    "direct_causal",
				})
			}
		}
	}

	return evidences
}

// collectWorkloadAvailability 根据 Workload 类型获取 Workload availability signal。
func (c *KubernetesSignalCollector) collectWorkloadAvailability(ctx context.Context, svc ServiceContext, fetchedAt time.Time) ([]rca.Evidence, error) {
	if c.k8s == nil || svc.WorkloadName == "" {
		return nil, nil
	}

	switch svc.WorkloadType {
	case "deployment":
		deps, err := c.k8s.ListDeployments(ctx, svc.Cluster, svc.Namespace)
		if err != nil {
			return nil, err
		}
		var dep *appsv1.Deployment
		for i := range deps {
			if deps[i].Name == svc.WorkloadName {
				dep = &deps[i]
				break
			}
		}
		if dep == nil {
			return nil, nil
		}
		desired := dep.Status.Replicas
		ready := dep.Status.ReadyReplicas
		available := dep.Status.AvailableReplicas
		level := rca.EvidenceLevelContext
		causal := "contextual"
		score := 0.1
		if desired > 0 && ready < desired {
			level = rca.EvidenceLevelCorroborating
			causal = "supporting"
			score = 0.5
		}
		return []rca.Evidence{{
			ID:            fmt.Sprintf("deployment-availability-%s-%s-%s", svc.Cluster, svc.Namespace, svc.WorkloadName),
			Type:          "deployment_availability",
			Level:         level,
			Source:        "kubernetes",
			SourceType:    "provider",
			Timestamp:     fetchedAt,
			ResourceType:  "deployment",
			ResourceName:  svc.WorkloadName,
			Namespace:     svc.Namespace,
			Value:         float64(ready),
			Expected:      fmt.Sprintf("%d/%d replicas ready (%d available)", ready, desired, available),
			Description:   fmt.Sprintf("Deployment %s: %d/%d replicas ready, %d available", svc.WorkloadName, ready, desired, available),
			Score:         score,
			FetchedAt:     &fetchedAt,
			DataTimestamp: &fetchedAt,
			TimestampAvailable: true,
			TrustStatus:        "fresh",
			CausalRelevance:    causal,
		}}, nil

	case "statefulset":
		stsList, err := c.k8s.ListStatefulSets(ctx, svc.Cluster, svc.Namespace)
		if err != nil {
			return nil, err
		}
		var sts *appsv1.StatefulSet
		for i := range stsList {
			if stsList[i].Name == svc.WorkloadName {
				sts = &stsList[i]
				break
			}
		}
		if sts == nil {
			return nil, nil
		}
		desired := sts.Status.Replicas
		ready := sts.Status.ReadyReplicas
		level := rca.EvidenceLevelContext
		causal := "contextual"
		score := 0.1
		if desired > 0 && ready < desired {
			level = rca.EvidenceLevelCorroborating
			causal = "supporting"
			score = 0.5
		}
		return []rca.Evidence{{
			ID:            fmt.Sprintf("statefulset-availability-%s-%s-%s", svc.Cluster, svc.Namespace, svc.WorkloadName),
			Type:          "statefulset_availability",
			Level:         level,
			Source:        "kubernetes",
			SourceType:    "provider",
			Timestamp:     fetchedAt,
			ResourceType:  "statefulset",
			ResourceName:  svc.WorkloadName,
			Namespace:     svc.Namespace,
			Value:         float64(ready),
			Expected:      fmt.Sprintf("%d/%d replicas ready", ready, desired),
			Description:   fmt.Sprintf("StatefulSet %s: %d/%d replicas ready", svc.WorkloadName, ready, desired),
			Score:         score,
			FetchedAt:     &fetchedAt,
			DataTimestamp: &fetchedAt,
			TimestampAvailable: true,
			TrustStatus:        "fresh",
			CausalRelevance:    causal,
		}}, nil

	case "daemonset":
		dsList, err := c.k8s.ListDaemonSets(ctx, svc.Cluster, svc.Namespace)
		if err != nil {
			return nil, err
		}
		var ds *appsv1.DaemonSet
		for i := range dsList {
			if dsList[i].Name == svc.WorkloadName {
				ds = &dsList[i]
				break
			}
		}
		if ds == nil {
			return nil, nil
		}
		desired := ds.Status.DesiredNumberScheduled
		ready := ds.Status.NumberReady
		level := rca.EvidenceLevelContext
		causal := "contextual"
		score := 0.1
		if desired > 0 && ready < desired {
			level = rca.EvidenceLevelCorroborating
			causal = "supporting"
			score = 0.5
		}
		return []rca.Evidence{{
			ID:            fmt.Sprintf("daemonset-availability-%s-%s-%s", svc.Cluster, svc.Namespace, svc.WorkloadName),
			Type:          "daemonset_availability",
			Level:         level,
			Source:        "kubernetes",
			SourceType:    "provider",
			Timestamp:     fetchedAt,
			ResourceType:  "daemonset",
			ResourceName:  svc.WorkloadName,
			Namespace:     svc.Namespace,
			Value:         float64(ready),
			Expected:      fmt.Sprintf("%d/%d nodes ready", ready, desired),
			Description:   fmt.Sprintf("DaemonSet %s: %d/%d nodes ready", svc.WorkloadName, ready, desired),
			Score:         score,
			FetchedAt:     &fetchedAt,
			DataTimestamp: &fetchedAt,
			TimestampAvailable: true,
			TrustStatus:        "fresh",
			CausalRelevance:    causal,
		}}, nil
	}

	return nil, nil
}

// isPodReady 检查 Pod 是否处于 Ready 状态。
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// timePtr 返回 time.Time 的指针（用于 Evidence 中的可选时间字段）。
func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
