package topology

import (
	"context"
	"fmt"
	"log/slog"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
)

// Builder 负责从 Kubernetes 资源构建拓扑图。
type Builder struct {
	provider Provider
}

func NewBuilder(provider Provider) *Builder {
	return &Builder{provider: provider}
}

// Build 构建指定集群和 namespace 的拓扑图。
// namespace 为空表示所有 namespace。
func (b *Builder) Build(ctx context.Context, cluster, namespace string) (*Graph, error) {
	// 批量获取所有资源（避免 N+1 查询）。
	nodes, err := b.provider.ListNodes(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	pods, err := b.provider.ListPods(ctx, cluster, namespace)
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	deployments, err := b.provider.ListDeployments(ctx, cluster, namespace)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	services, err := b.provider.ListServices(ctx, cluster, namespace)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	ingresses, err := b.provider.ListIngresses(ctx, cluster, namespace)
	if err != nil {
		// Ingress 是可选资源，某些集群可能没有 networking API。
		slog.Warn("topology: list ingresses failed, skipping", "err", err)
		ingresses = nil
	}

	graph := &Graph{Cluster: cluster}
	nodeMap := make(map[string]Node)
	edgeSet := make(map[string]bool)

	// 1. 构建 Node 节点。
	for _, n := range nodes {
		tn := b.buildNodeNode(cluster, &n)
		nodeMap[tn.ID] = tn
	}

	// 2. 构建 Pod 节点。
	for i := range pods {
		tp := b.buildPodNode(cluster, &pods[i])
		nodeMap[tp.ID] = tp
	}

	// 3. 构建 Deployment 节点。
	for i := range deployments {
		td := b.buildDeploymentNode(cluster, &deployments[i])
		nodeMap[td.ID] = td
	}

	// 4. 构建 Service 节点。
	for i := range services {
		ts := b.buildServiceNode(cluster, &services[i])
		nodeMap[ts.ID] = ts
	}

	// 5. 构建 Ingress 节点。
	for i := range ingresses {
		ti := b.buildIngressNode(cluster, &ingresses[i])
		nodeMap[ti.ID] = ti
	}

	// 6. 构建边：Deployment → Pod (owns)。
	b.buildDeploymentPodEdges(&deployments, &pods, cluster, nodeMap, edgeSet, graph)

	// 7. 构建边：Pod → Node (runs_on)。
	b.buildPodNodeEdges(&pods, cluster, nodeMap, edgeSet, graph)

	// 8. 构建边：Service → Pod (selects)。
	b.buildServicePodEdges(&services, &pods, cluster, nodeMap, edgeSet, graph)

	// 9. 构建边：Ingress → Service (routes_to)。
	b.buildIngressServiceEdges(&ingresses, &services, cluster, nodeMap, edgeSet, graph)

	// 收集所有节点。
	for _, n := range nodeMap {
		graph.Nodes = append(graph.Nodes, n)
	}

	return graph, nil
}

// buildNodeNode 构建 Kubernetes Node 拓扑节点。
func (b *Builder) buildNodeNode(cluster string, n *corev1.Node) Node {
	status := StatusHealthy
	for _, cond := range n.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
			status = StatusCritical
		}
	}
	return Node{
		ID:      NodeID(cluster, TypeNode, "", n.Name),
		Type:    TypeNode,
		Name:    n.Name,
		Cluster: cluster,
		Status:  status,
		Labels:  n.Labels,
		Metadata: map[string]any{
			"kubelet_version": n.Status.NodeInfo.KubeletVersion,
			"os_image":        n.Status.NodeInfo.OSImage,
			"architecture":    n.Status.NodeInfo.Architecture,
		},
	}
}

// buildPodNode 构建 Pod 拓扑节点。
func (b *Builder) buildPodNode(cluster string, p *corev1.Pod) Node {
	status := StatusHealthy
	switch p.Status.Phase {
	case corev1.PodFailed:
		status = StatusCritical
	case corev1.PodPending:
		status = StatusWarning
	case corev1.PodRunning:
		// 检查容器是否都 ready。
		for _, cs := range p.Status.ContainerStatuses {
			if !cs.Ready {
				status = StatusWarning
			}
		}
	}
	return Node{
		ID:        NodeID(cluster, TypePod, p.Namespace, p.Name),
		Type:      TypePod,
		Name:      p.Name,
		Namespace: p.Namespace,
		Cluster:   cluster,
		Status:    status,
		Labels:    p.Labels,
		Metadata: map[string]any{
			"pod_ip":       p.Status.PodIP,
			"node_name":    p.Spec.NodeName,
			"restart_count": totalRestarts(p),
			"phase":        string(p.Status.Phase),
		},
	}
}

// buildDeploymentNode 构建 Deployment 拓扑节点。
func (b *Builder) buildDeploymentNode(cluster string, d *appsv1.Deployment) Node {
	status := StatusHealthy
	if d.Status.UnavailableReplicas > 0 {
		status = StatusWarning
	}
	if d.Status.ReadyReplicas == 0 && *d.Spec.Replicas > 0 {
		status = StatusCritical
	}
	return Node{
		ID:        NodeID(cluster, TypeDeployment, d.Namespace, d.Name),
		Type:      TypeDeployment,
		Name:      d.Name,
		Namespace: d.Namespace,
		Cluster:   cluster,
		Status:    status,
		Labels:    d.Labels,
		Metadata: map[string]any{
			"replicas":          d.Status.Replicas,
			"ready_replicas":    d.Status.ReadyReplicas,
			"updated_replicas":  d.Status.UpdatedReplicas,
			"unavailable":       d.Status.UnavailableReplicas,
		},
	}
}

// buildServiceNode 构建 Service 拓扑节点。
func (b *Builder) buildServiceNode(cluster string, s *corev1.Service) Node {
	return Node{
		ID:        NodeID(cluster, TypeService, s.Namespace, s.Name),
		Type:      TypeService,
		Name:      s.Name,
		Namespace: s.Namespace,
		Cluster:   cluster,
		Status:    StatusHealthy,
		Labels:    s.Labels,
		Metadata: map[string]any{
			"type":       string(s.Spec.Type),
			"cluster_ip": s.Spec.ClusterIP,
		},
	}
}

// buildIngressNode 构建 Ingress 拓扑节点。
func (b *Builder) buildIngressNode(cluster string, ing *networkingv1.Ingress) Node {
	return Node{
		ID:        NodeID(cluster, TypeIngress, ing.Namespace, ing.Name),
		Type:      TypeIngress,
		Name:      ing.Name,
		Namespace: ing.Namespace,
		Cluster:   cluster,
		Status:    StatusHealthy,
		Labels:    ing.Labels,
	}
}

// buildDeploymentPodEdges 构建 Deployment → Pod (owns) 边。
// 通过 Pod OwnerReferences 匹配。
func (b *Builder) buildDeploymentPodEdges(
	deployments *[]appsv1.Deployment, pods *[]corev1.Pod,
	cluster string, nodeMap map[string]Node, edgeSet map[string]bool, graph *Graph,
) {
	// 建立 Deployment UID → 拓扑节点 ID 的映射。
	depUIDMap := make(map[string]string)
	for _, d := range *deployments {
		depUIDMap[string(d.UID)] = NodeID(cluster, TypeDeployment, d.Namespace, d.Name)
	}

	for _, p := range *pods {
		for _, owner := range p.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				// Pod 的 OwnerReference 是 ReplicaSet，不是直接 Deployment。
				// ReplicaSet 的 OwnerReference 才是 Deployment。
				// 简化处理：通过 ReplicaSet 名称前缀匹配 Deployment。
				// 更准确的方式是查询 ReplicaSet，但为了性能这里用名称匹配。
				depName := deploymentNameFromReplicaSet(owner.Name)
				depID := NodeID(cluster, TypeDeployment, p.Namespace, depName)
				if _, exists := nodeMap[depID]; exists {
					podID := NodeID(cluster, TypePod, p.Namespace, p.Name)
					b.addEdge(graph, edgeSet, depID, podID, RelationOwns, cluster)
				}
			}
		}
	}
}

// buildPodNodeEdges 构建 Pod → Node (runs_on) 边。
func (b *Builder) buildPodNodeEdges(
	pods *[]corev1.Pod, cluster string,
	nodeMap map[string]Node, edgeSet map[string]bool, graph *Graph,
) {
	for _, p := range *pods {
		if p.Spec.NodeName == "" {
			continue
		}
		nodeID := NodeID(cluster, TypeNode, "", p.Spec.NodeName)
		if _, exists := nodeMap[nodeID]; exists {
			podID := NodeID(cluster, TypePod, p.Namespace, p.Name)
			b.addEdge(graph, edgeSet, podID, nodeID, RelationRunsOn, cluster)
		}
	}
}

// buildServicePodEdges 构建 Service → Pod (selects) 边。
// 通过 Service Selector 匹配 Pod Labels，在内存中匹配，避免 N+1 查询。
func (b *Builder) buildServicePodEdges(
	services *[]corev1.Service, pods *[]corev1.Pod,
	cluster string, nodeMap map[string]Node, edgeSet map[string]bool, graph *Graph,
) {
	for _, svc := range *services {
		if len(svc.Spec.Selector) == 0 {
			continue // 没有 selector 的 Service（如 ExternalName）不关联 Pod。
		}
		svcID := NodeID(cluster, TypeService, svc.Namespace, svc.Name)
		for _, p := range *pods {
			if p.Namespace != svc.Namespace {
				continue // 不同 namespace 不能关联。
			}
			if labelsMatch(svc.Spec.Selector, p.Labels) {
				podID := NodeID(cluster, TypePod, p.Namespace, p.Name)
				b.addEdge(graph, edgeSet, svcID, podID, RelationSelects, cluster)
			}
		}
	}
}

// buildIngressServiceEdges 构建 Ingress → Service (routes_to) 边。
func (b *Builder) buildIngressServiceEdges(
	ingresses *[]networkingv1.Ingress, services *[]corev1.Service,
	cluster string, nodeMap map[string]Node, edgeSet map[string]bool, graph *Graph,
) {
	// 建立 Service 名称 → 拓扑节点 ID 的映射（同 namespace）。
	svcMap := make(map[string]string)
	for _, s := range *services {
		svcMap[s.Namespace+"/"+s.Name] = NodeID(cluster, TypeService, s.Namespace, s.Name)
	}

	for _, ing := range *ingresses {
		ingID := NodeID(cluster, TypeIngress, ing.Namespace, ing.Name)

		// defaultBackend。
		if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
			svcName := ing.Spec.DefaultBackend.Service.Name
			if svcID, ok := svcMap[ing.Namespace+"/"+svcName]; ok {
				b.addEdge(graph, edgeSet, ingID, svcID, RelationRoutes, cluster)
			}
		}

		// rules。
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					svcName := path.Backend.Service.Name
					if svcID, ok := svcMap[ing.Namespace+"/"+svcName]; ok {
						b.addEdge(graph, edgeSet, ingID, svcID, RelationRoutes, cluster)
					}
				}
			}
		}
	}
}

// addEdge 添加边，去重。
func (b *Builder) addEdge(graph *Graph, edgeSet map[string]bool, source, target string, relation RelationType, cluster string) {
	id := EdgeID(source, target, relation)
	if edgeSet[id] {
		return
	}
	edgeSet[id] = true
	graph.Edges = append(graph.Edges, Edge{
		ID:       id,
		Source:   source,
		Target:   target,
		Relation: relation,
		Cluster:  cluster,
	})
}

// labelsMatch 检查 Pod labels 是否匹配 Service selector。
func labelsMatch(selector, labels map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// deploymentNameFromReplicaSet 从 ReplicaSet 名称推导 Deployment 名称。
// ReplicaSet 名称格式：<deployment>-<replica-set-hash>
func deploymentNameFromReplicaSet(rsName string) string {
	// 找到最后一个 "-"，前面的部分就是 Deployment 名称。
	for i := len(rsName) - 1; i >= 0; i-- {
		if rsName[i] == '-' {
			return rsName[:i]
		}
	}
	return rsName
}

// totalRestarts 计算 Pod 总重启次数。
func totalRestarts(p *corev1.Pod) int32 {
	var total int32
	for _, cs := range p.Status.ContainerStatuses {
		total += cs.RestartCount
	}
	return total
}
