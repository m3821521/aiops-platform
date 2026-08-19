package topology

import (
	"context"
	"time"

	"github.com/aiops/aiops-platform/internal/anomaly"
	"github.com/aiops/aiops-platform/internal/incident"
)

// DefaultStatusProvider 是 topology.StatusProvider 的默认实现。
// 从 Incident 和 Anomaly 数据计算资源状态。
type DefaultStatusProvider struct {
	incidentRepo *incident.Repository
	anomalyRepo  *anomaly.Repository
}

func NewDefaultStatusProvider(incidentRepo *incident.Repository, anomalyRepo *anomaly.Repository) *DefaultStatusProvider {
	return &DefaultStatusProvider{
		incidentRepo: incidentRepo,
		anomalyRepo:  anomalyRepo,
	}
}

// ResourceStatus 返回指定资源的状态。
// 逻辑：
// - 存在 Critical Incident → critical
// - 存在 Warning Incident → warning
// - 存在 Active Anomaly → warning
// - 否则 → healthy（由 K8s 自身状态决定）
func (p *DefaultStatusProvider) ResourceStatus(
	ctx context.Context, cluster, resourceType, namespace, name string,
) (NodeStatus, []int64, int, int) {
	var incidentIDs []int64
	alertCount := 0
	anomalyCount := 0
	status := StatusHealthy

	// 查询活跃 Incident（按资源匹配）。
	if p.incidentRepo != nil {
		since := time.Now().Add(-24 * time.Hour)
		incidents, err := p.incidentRepo.FindActiveByResource(ctx, cluster, namespace, name, since)
		if err == nil {
			for _, inc := range incidents {
				// 精确匹配 resource_name 或 service。
				if inc.ResourceName == name || inc.Service == name {
					incidentIDs = append(incidentIDs, inc.ID)
					if inc.Severity == "critical" {
						status = StatusCritical
					} else if inc.Severity == "warning" && status != StatusCritical {
						status = StatusWarning
					}
				}
			}
		}
	}

	// 查询活跃 Anomaly（按资源匹配）。
	if p.anomalyRepo != nil {
		since := time.Now().Add(-1 * time.Hour)
		anomalies, err := p.anomalyRepo.FindByResource(ctx, cluster, resourceType, name, since)
		if err == nil {
			anomalyCount = len(anomalies)
			if anomalyCount > 0 && status == StatusHealthy {
				status = StatusWarning
			}
		}
	}

	return status, incidentIDs, alertCount, anomalyCount
}
