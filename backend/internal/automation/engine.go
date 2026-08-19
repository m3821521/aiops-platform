package automation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aiops/aiops-platform/internal/cluster"
	corev1 "k8s.io/api/core/v1"
)

// ActionType 操作类型。
type ActionType string

const (
	ActionRead  ActionType = "read"  // 只读操作
	ActionWrite ActionType = "write" // 写操作（需要确认）
)

// ActionRecord 是一次操作的记录。
type ActionRecord struct {
	Action     ActionType `json:"action"`
	Resource   string     `json:"resource"` // pod / deployment / ...
	Name       string     `json:"name"`     // 资源名称
	Namespace  string     `json:"namespace"`
	Cluster    string     `json:"cluster"`
	Params     string     `json:"params"` // 操作参数摘要
	Result     string     `json:"result"` // 执行结果
	Error      string     `json:"error,omitempty"`
	Operator   string     `json:"operator"` // 操作者（后续从 JWT 获取）
	ExecutedAt time.Time  `json:"executed_at"`
	Confirmed  bool       `json:"confirmed"` // 是否经过确认
}

// Engine 是自动化运维引擎。
// 所有写操作必须显式确认才能执行，防止误操作。
type Engine struct {
	cluster *cluster.Service
}

// NewEngine 创建自动化引擎。
func NewEngine(c *cluster.Service) *Engine {
	return &Engine{cluster: c}
}

// requireConfirm 检查写操作是否已确认。
func (e *Engine) requireConfirm(confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("此操作需要确认，请在请求中设置 confirm=true")
	}
	return nil
}

// record 记录操作日志。
func (e *Engine) record(rec ActionRecord) {
	slog.Info("automation action",
		"action", rec.Action,
		"resource", rec.Resource,
		"name", rec.Name,
		"namespace", rec.Namespace,
		"cluster", rec.Cluster,
		"confirmed", rec.Confirmed,
		"result", rec.Result,
		"error", rec.Error,
	)
}

// GetPodLogs 获取 Pod 日志（只读）。
func (e *Engine) GetPodLogs(ctx context.Context, clusterName, namespace, pod, container string, tailLines int64) (string, error) {
	logs, err := e.cluster.GetPodLogs(ctx, clusterName, namespace, pod, container, tailLines)
	e.record(ActionRecord{
		Action:     ActionRead,
		Resource:   "pod_logs",
		Name:       pod,
		Namespace:  namespace,
		Cluster:    clusterName,
		Params:     fmt.Sprintf("container=%s,tail=%d", container, tailLines),
		Result:     "success",
		ExecutedAt: time.Now(),
	})
	return logs, err
}

// GetPodEvents 获取 Pod Event（只读）。
func (e *Engine) GetPodEvents(ctx context.Context, clusterName, namespace, pod string) ([]corev1.Event, error) {
	events, err := e.cluster.GetPodEvents(ctx, clusterName, namespace, pod)
	e.record(ActionRecord{
		Action:     ActionRead,
		Resource:   "pod_events",
		Name:       pod,
		Namespace:  namespace,
		Cluster:    clusterName,
		Result:     "success",
		ExecutedAt: time.Now(),
	})
	return events, err
}

// RestartPod 重启 Pod（写操作，需确认）。
func (e *Engine) RestartPod(ctx context.Context, clusterName, namespace, pod string, confirmed bool) error {
	if err := e.requireConfirm(confirmed); err != nil {
		e.record(ActionRecord{
			Action:     ActionWrite,
			Resource:   "pod",
			Name:       pod,
			Namespace:  namespace,
			Cluster:    clusterName,
			Result:     "rejected",
			Error:      err.Error(),
			Confirmed:  false,
			ExecutedAt: time.Now(),
		})
		return err
	}

	err := e.cluster.RestartPod(ctx, clusterName, namespace, pod)
	rec := ActionRecord{
		Action:     ActionWrite,
		Resource:   "pod",
		Name:       pod,
		Namespace:  namespace,
		Cluster:    clusterName,
		Params:     "restart",
		Confirmed:  true,
		ExecutedAt: time.Now(),
	}
	if err != nil {
		rec.Result = "failed"
		rec.Error = err.Error()
	} else {
		rec.Result = "success"
	}
	e.record(rec)
	return err
}

// ScaleDeployment 扩容/缩容 Deployment（写操作，需确认）。
func (e *Engine) ScaleDeployment(ctx context.Context, clusterName, namespace, deployment string, replicas int32, confirmed bool) error {
	if err := e.requireConfirm(confirmed); err != nil {
		e.record(ActionRecord{
			Action:     ActionWrite,
			Resource:   "deployment",
			Name:       deployment,
			Namespace:  namespace,
			Cluster:    clusterName,
			Result:     "rejected",
			Error:      err.Error(),
			Confirmed:  false,
			ExecutedAt: time.Now(),
		})
		return err
	}

	err := e.cluster.ScaleDeployment(ctx, clusterName, namespace, deployment, replicas)
	rec := ActionRecord{
		Action:     ActionWrite,
		Resource:   "deployment",
		Name:       deployment,
		Namespace:  namespace,
		Cluster:    clusterName,
		Params:     fmt.Sprintf("replicas=%d", replicas),
		Confirmed:  true,
		ExecutedAt: time.Now(),
	}
	if err != nil {
		rec.Result = "failed"
		rec.Error = err.Error()
	} else {
		rec.Result = "success"
	}
	e.record(rec)
	return err
}
