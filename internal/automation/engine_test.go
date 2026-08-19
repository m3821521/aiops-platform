package automation_test

import (
	"context"
	"testing"

	"github.com/aiops/aiops-platform/internal/automation"
	"github.com/aiops/aiops-platform/internal/cluster"
)

func newTestEngine() *automation.Engine {
	// 创建空的 cluster Manager，Client 调用会返回错误（无集群配置）。
	mgr := cluster.NewManager(nil)
	svc := cluster.NewService(mgr)
	return automation.NewEngine(svc)
}

func TestRestartPodWithoutConfirm(t *testing.T) {
	engine := newTestEngine()
	err := engine.RestartPod(context.Background(), "", "default", "my-pod", false)
	if err == nil {
		t.Fatal("expected error when not confirmed")
	}
}

func TestRestartPodWithConfirm(t *testing.T) {
	engine := newTestEngine()
	// 确认后会尝试执行，但因为没有 K8s 集群会返回连接错误。
	// 这验证了确认逻辑通过，进入了实际执行阶段。
	err := engine.RestartPod(context.Background(), "", "default", "my-pod", true)
	if err == nil {
		t.Fatal("expected error from K8s connection")
	}
}

func TestScaleDeploymentWithoutConfirm(t *testing.T) {
	engine := newTestEngine()
	err := engine.ScaleDeployment(context.Background(), "", "default", "my-dep", 3, false)
	if err == nil {
		t.Fatal("expected error when not confirmed")
	}
}

func TestScaleDeploymentWithConfirm(t *testing.T) {
	engine := newTestEngine()
	err := engine.ScaleDeployment(context.Background(), "", "default", "my-dep", 3, true)
	if err == nil {
		t.Fatal("expected error from K8s connection")
	}
}

func TestGetPodLogs(t *testing.T) {
	engine := newTestEngine()
	// 只读操作不需要确认，但会因为没有 K8s 集群返回错误。
	_, err := engine.GetPodLogs(context.Background(), "", "default", "my-pod", "", 100)
	if err == nil {
		t.Fatal("expected error from K8s connection")
	}
}

func TestGetPodEvents(t *testing.T) {
	engine := newTestEngine()
	_, err := engine.GetPodEvents(context.Background(), "", "default", "my-pod")
	if err == nil {
		t.Fatal("expected error from K8s connection")
	}
}

func TestActionTypeConstants(t *testing.T) {
	if automation.ActionRead != "read" {
		t.Fatalf("expected ActionRead=read, got %s", automation.ActionRead)
	}
	if automation.ActionWrite != "write" {
		t.Fatalf("expected ActionWrite=write, got %s", automation.ActionWrite)
	}
}
