package rca

import (
	"context"
	"time"
)

// ContextCollector 是 RCA 上下文收集器接口。
// 从各个数据源收集 IncidentContext 所需的数据。
// 通过接口解耦，便于测试和避免循环依赖。
type ContextCollector interface {
	CollectAlerts(ctx context.Context, incidentID int64) ([]AlertInfo, error)
	CollectAnomalies(ctx context.Context, cluster, namespace, resourceType, resourceName string, since time.Time) ([]AnomalyInfo, error)
	CollectEvents(ctx context.Context, cluster, namespace, resourceType, resourceName string, since time.Time) ([]EventInfo, error)
	CollectMetrics(ctx context.Context, cluster, namespace, resourceType, resourceName string, since, until time.Time) ([]MetricInfo, error)
	CollectLogs(ctx context.Context, cluster, namespace, pod string, since, until time.Time) ([]LogInfo, error)
	CollectTopology(ctx context.Context, cluster, namespace string) (TopologyInfo, error)
}

// NoopCollector 是一个空实现，用于测试或数据源不可用时。
type NoopCollector struct{}

func (n *NoopCollector) CollectAlerts(ctx context.Context, incidentID int64) ([]AlertInfo, error) {
	return nil, nil
}
func (n *NoopCollector) CollectAnomalies(ctx context.Context, cluster, namespace, resourceType, resourceName string, since time.Time) ([]AnomalyInfo, error) {
	return nil, nil
}
func (n *NoopCollector) CollectEvents(ctx context.Context, cluster, namespace, resourceType, resourceName string, since time.Time) ([]EventInfo, error) {
	return nil, nil
}
func (n *NoopCollector) CollectMetrics(ctx context.Context, cluster, namespace, resourceType, resourceName string, since, until time.Time) ([]MetricInfo, error) {
	return nil, nil
}
func (n *NoopCollector) CollectLogs(ctx context.Context, cluster, namespace, pod string, since, until time.Time) ([]LogInfo, error) {
	return nil, nil
}
func (n *NoopCollector) CollectTopology(ctx context.Context, cluster, namespace string) (TopologyInfo, error) {
	return TopologyInfo{}, nil
}
