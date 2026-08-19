package monitoring

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

// 预定义 PromQL。依赖 node_exporter（节点）和 cAdvisor/kubelet（Pod）采集。
const (
	// 节点 CPU 使用率（百分比），按 instance 分组。
	promqlNodeCPU = `100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`

	// 节点内存使用率（百分比），按 instance 分组。
	promqlNodeMemory = `100 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes * 100)`

	// Pod CPU 使用量（cores），按 namespace/pod 分组，排除 POD 容器和空容器。
	promqlPodCPU = `sum by(namespace, pod)(rate(container_cpu_usage_seconds_total{container!="",container!="POD"}[5m]))`

	// Pod 内存使用量（working set bytes），按 namespace/pod 分组。
	promqlPodMemory = `sum by(namespace, pod)(container_memory_working_set_bytes{container!="",container!="POD"})`
)

// namespaceRe 校验 Kubernetes namespace 格式，防止用户输入拼进 PromQL 时产生异常。
var namespaceRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// NodeMetrics 节点 CPU + 内存组合结果。
type NodeMetrics struct {
	CPU    *QueryResult `json:"cpu"`
	Memory *QueryResult `json:"memory"`
}

// PodMetrics Pod CPU + 内存组合结果。
type PodMetrics struct {
	CPU    *QueryResult `json:"cpu"`
	Memory *QueryResult `json:"memory"`
}

// NodeMetrics 查询所有节点的 CPU 和内存使用率（即时查询）。
func (c *Client) NodeMetrics(ctx context.Context) (*NodeMetrics, error) {
	cpu, err := c.Query(ctx, promqlNodeCPU, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("查询节点 CPU 失败: %w", err)
	}
	mem, err := c.Query(ctx, promqlNodeMemory, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("查询节点内存失败: %w", err)
	}
	return &NodeMetrics{CPU: cpu, Memory: mem}, nil
}

// PodMetrics 查询 Pod 的 CPU 和内存使用量。
// namespace 为空时查询所有命名空间；非空时只查询指定 namespace。
func (c *Client) PodMetrics(ctx context.Context, namespace string) (*PodMetrics, error) {
	cpuQL := promqlPodCPU
	memQL := promqlPodMemory

	if namespace != "" {
		if !namespaceRe.MatchString(namespace) {
			return nil, fmt.Errorf("namespace 格式不合法: %s", namespace)
		}
		// 在已有 label matcher 中追加 namespace 过滤。
		cpuQL = `sum by(namespace, pod)(rate(container_cpu_usage_seconds_total{namespace="` + namespace + `",container!="",container!="POD"}[5m]))`
		memQL = `sum by(namespace, pod)(container_memory_working_set_bytes{namespace="` + namespace + `",container!="",container!="POD"})`
	}

	cpu, err := c.Query(ctx, cpuQL, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("查询 Pod CPU 失败: %w", err)
	}
	mem, err := c.Query(ctx, memQL, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("查询 Pod 内存失败: %w", err)
	}
	return &PodMetrics{CPU: cpu, Memory: mem}, nil
}
