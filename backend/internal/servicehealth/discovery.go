package servicehealth

import (
	"context"
	"encoding/json"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// KubernetesDiscovery 是 Service Discovery 所需的最小 K8s 接口。
// 真实实现为 *cluster.Service，测试时可使用 mock。
// 定义在 servicehealth 包内，避免修改 cluster.Service。
type KubernetesDiscovery interface {
	ListServices(ctx context.Context, cluster, namespace string) ([]corev1.Service, error)
	ListDeployments(ctx context.Context, cluster, namespace string) ([]appsv1.Deployment, error)
	ListStatefulSets(ctx context.Context, cluster, namespace string) ([]appsv1.StatefulSet, error)
	ListDaemonSets(ctx context.Context, cluster, namespace string) ([]appsv1.DaemonSet, error)
	ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error)
}

// DiscoveryService 从 Kubernetes API 发现平台 Service，
// 建立 Service → Workload → Pod 映射。
type DiscoveryService struct {
	k8s KubernetesDiscovery
}

// NewDiscoveryService 创建 DiscoveryService。
func NewDiscoveryService(k8s KubernetesDiscovery) *DiscoveryService {
	return &DiscoveryService{k8s: k8s}
}

// ListPods 直接获取指定 cluster/namespace 的 Pod 列表。
// P2-01 Phase 3: 供 Signal Collector 使用，避免重复实现 K8s 调用。
func (d *DiscoveryService) ListPods(ctx context.Context, cluster, namespace string) ([]corev1.Pod, error) {
	return d.k8s.ListPods(ctx, cluster, namespace)
}

// DiscoveredService 是 Discovery 输出的结构化结果（未持久化）。
type DiscoveredService struct {
	Name             string            `json:"name"`
	Namespace        string            `json:"namespace"`
	Cluster          string            `json:"cluster"`
	ServiceType      string            `json:"service_type"` // ClusterIP/NodePort/LoadBalancer/ExternalName/Headless
	WorkloadType     WorkloadType      `json:"workload_type"`
	WorkloadName     string            `json:"workload_name,omitempty"`
	WorkloadSelector map[string]string `json:"workload_selector,omitempty"`
	PodCount         int               `json:"pod_count"`
	FetchedAt        time.Time         `json:"fetched_at"`
}

// Discover 从 K8s API 发现 Service，建立 Service→Workload→Pod 映射。
//
// K8s API 调用次数固定为 5 次（与 Service 数量无关）：
//  1. ListServices
//  2. ListDeployments
//  3. ListStatefulSets
//  4. ListDaemonSets
//  5. ListPods
//
// 全部资源先批量获取，然后在内存中建立 selector→pods、pod→owner、
// service→workload 映射，不存在 N+1 查询。
func (d *DiscoveryService) Discover(ctx context.Context, cluster, namespace string) ([]DiscoveredService, error) {
	fetchedAt := time.Now()

	// 1-5: 批量获取所有资源（固定 5 次 K8s API）
	services, err := d.k8s.ListServices(ctx, cluster, namespace)
	if err != nil {
		return nil, err
	}
	deployments, err := d.k8s.ListDeployments(ctx, cluster, namespace)
	if err != nil {
		return nil, err
	}
	statefulSets, err := d.k8s.ListStatefulSets(ctx, cluster, namespace)
	if err != nil {
		return nil, err
	}
	daemonSets, err := d.k8s.ListDaemonSets(ctx, cluster, namespace)
	if err != nil {
		return nil, err
	}
	pods, err := d.k8s.ListPods(ctx, cluster, namespace)
	if err != nil {
		return nil, err
	}

	// 构建内存映射
	depSet := buildNameSet(deployments, func(d appsv1.Deployment) string { return d.Name })
	stsSet := buildNameSet(statefulSets, func(s appsv1.StatefulSet) string { return s.Name })
	dsSet := buildNameSet(daemonSets, func(d appsv1.DaemonSet) string { return d.Name })

	// 对每个 Service 建立映射
	result := make([]DiscoveredService, 0, len(services))
	for i := range services {
		svc := &services[i]
		discovered := DiscoveredService{
			Name:             svc.Name,
			Namespace:        svc.Namespace,
			Cluster:          cluster,
			ServiceType:      detectServiceType(svc),
			WorkloadSelector: svc.Spec.Selector,
			FetchedAt:        fetchedAt,
		}

		// 无 selector 的 Service（ExternalName / 特殊 Headless）不绑定 workload
		if len(svc.Spec.Selector) == 0 {
			discovered.WorkloadType = WorkloadTypeUnknown
			result = append(result, discovered)
			continue
		}

		// 通过 selector 匹配 Pods，统计 PodCount 并确定 workload
		matchedPods := matchPodsBySelector(svc.Spec.Selector, pods)
		discovered.PodCount = len(matchedPods)

		// 从匹配的 Pods 中确定 workload 类型和名称
		wlType, wlName := resolveWorkload(matchedPods, depSet, stsSet, dsSet)
		discovered.WorkloadType = wlType
		discovered.WorkloadName = wlName

		result = append(result, discovered)
	}

	return result, nil
}

// detectServiceType 识别 Kubernetes Service 类型。
// Headless Service: ClusterIP == "None"（且 Type 为 ClusterIP）。
func detectServiceType(svc *corev1.Service) string {
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		return "ExternalName"
	}
	if svc.Spec.ClusterIP == "None" {
		return "Headless"
	}
	switch svc.Spec.Type {
	case corev1.ServiceTypeNodePort:
		return "NodePort"
	case corev1.ServiceTypeLoadBalancer:
		return "LoadBalancer"
	default:
		return "ClusterIP"
	}
}

// matchPodsBySelector 通过 Service Selector 与 Pod Labels 匹配。
// selector 的所有 key/value 必须全部匹配才算匹配。
func matchPodsBySelector(selector map[string]string, pods []corev1.Pod) []corev1.Pod {
	if len(selector) == 0 {
		return nil
	}
	var matched []corev1.Pod
	for i := range pods {
		if labelsMatch(selector, pods[i].Labels) {
			matched = append(matched, pods[i])
		}
	}
	return matched
}

// labelsMatch 检查 Pod labels 是否匹配 Service selector。
// selector 的所有 key/value 必须全部匹配。
func labelsMatch(selector, labels map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// resolveWorkload 从匹配的 Pods 中确定 Service 关联的工作负载。
//
// 规则：
//   - Pod OwnerReference.Kind == "ReplicaSet" → 通过 ReplicaSet 名称前缀推导 Deployment
//   - Pod OwnerReference.Kind == "StatefulSet" → 直接匹配
//   - Pod OwnerReference.Kind == "DaemonSet" → 直接匹配
//   - 取出现频率最高的 workload 作为结果
//   - 无法确定 → WorkloadTypeUnknown
//
// 绝对禁止通过 Service 名称猜测 Deployment 名称。
func resolveWorkload(pods []corev1.Pod, depSet, stsSet, dsSet map[string]bool) (WorkloadType, string) {
	if len(pods) == 0 {
		return WorkloadTypeUnknown, ""
	}

	type wlKey struct {
		typ  WorkloadType
		name string
	}
	counts := make(map[wlKey]int)

	for i := range pods {
		for _, owner := range pods[i].OwnerReferences {
			switch owner.Kind {
			case "ReplicaSet":
				depName := deploymentNameFromReplicaSet(owner.Name)
				if depName != "" && depSet[depName] {
					counts[wlKey{WorkloadTypeDeployment, depName}]++
				}
			case "StatefulSet":
				if stsSet[owner.Name] {
					counts[wlKey{WorkloadTypeStatefulSet, owner.Name}]++
				}
			case "DaemonSet":
				if dsSet[owner.Name] {
					counts[wlKey{WorkloadTypeDaemonSet, owner.Name}]++
				}
			}
		}
	}

	if len(counts) == 0 {
		return WorkloadTypeUnknown, ""
	}

	// 取出现频率最高的 workload
	var best wlKey
	var bestCount int
	for k, c := range counts {
		if c > bestCount {
			best = k
			bestCount = c
		}
	}
	return best.typ, best.name
}

// deploymentNameFromReplicaSet 从 ReplicaSet 名称推导 Deployment 名称。
// ReplicaSet 名称格式：<deployment>-<replica-set-hash>
// 找到最后一个 "-"，前面的部分就是 Deployment 名称。
func deploymentNameFromReplicaSet(rsName string) string {
	for i := len(rsName) - 1; i >= 0; i-- {
		if rsName[i] == '-' {
			return rsName[:i]
		}
	}
	return rsName
}

// buildNameSet 从资源列表构建名称集合，用于 O(1) 存在性检查。
func buildNameSet[T any](items []T, getName func(T) string) map[string]bool {
	set := make(map[string]bool, len(items))
	for i := range items {
		set[getName(items[i])] = true
	}
	return set
}

// selectorToJSON 将 selector map 序列化为 JSON 字符串，用于持久化。
func selectorToJSON(selector map[string]string) string {
	if len(selector) == 0 {
		return ""
	}
	b, err := json.Marshal(selector)
	if err != nil {
		return ""
	}
	return string(b)
}

// jsonToSelector 反序列化 selector JSON 字符串。
func jsonToSelector(s string) map[string]string {
	if s == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	return m
}
