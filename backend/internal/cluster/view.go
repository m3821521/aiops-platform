package cluster

import (
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type ClusterView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AuthType    string `json:"auth_type"`
	Enabled     bool   `json:"enabled"`
}

func ToClusterViews(items []Cluster) []ClusterView {
	out := make([]ClusterView, 0, len(items))
	for _, c := range items {
		out = append(out, ClusterView{
			Name:        c.Name,
			Description: c.Description,
			AuthType:    c.AuthType,
			Enabled:     c.Enabled,
		})
	}
	return out
}

// NodeView 节点列表视图
type NodeView struct {
	Name             string            `json:"name"`
	Status           string            `json:"status"`
	Version          string            `json:"version"`
	InternalIP       string            `json:"internal_ip"`
	Age              string            `json:"age"`
	CreationTimestamp time.Time       `json:"creation_timestamp"`
	OS               string            `json:"os"`
	Kernel           string            `json:"kernel"`
	ContainerRuntime string            `json:"container_runtime"`
	Labels           map[string]string `json:"labels"`
	PodCount         int               `json:"pod_count"`
}

func ToNodeViews(items []corev1.Node) []NodeView {
	out := make([]NodeView, 0, len(items))
	for _, n := range items {
		status := "Unknown"
		for _, cond := range n.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				if cond.Status == corev1.ConditionTrue {
					status = "Ready"
				} else {
					status = "NotReady"
				}
			}
		}
		internalIP := ""
		for _, addr := range n.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				internalIP = addr.Address
				break
			}
		}
		out = append(out, NodeView{
			Name:              n.Name,
			Status:            status,
			Version:           n.Status.NodeInfo.KubeletVersion,
			InternalIP:        internalIP,
			Age:               formatAge(n.CreationTimestamp.Time),
			CreationTimestamp: n.CreationTimestamp.Time,
			OS:                n.Status.NodeInfo.OSImage,
			Kernel:            n.Status.NodeInfo.KernelVersion,
			ContainerRuntime:  n.Status.NodeInfo.ContainerRuntimeVersion,
			Labels:            n.Labels,
		})
	}
	return out
}

// NodeDetail 节点详情
type NodeDetail struct {
	NodeView
	Conditions []NodeCondition `json:"conditions"`
	Taints     []corev1.Taint  `json:"taints"`
}

type NodeCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func ToNodeDetail(n *corev1.Node) NodeDetail {
	view := ToNodeViews([]corev1.Node{*n})[0]
	conditions := make([]NodeCondition, 0, len(n.Status.Conditions))
	for _, c := range n.Status.Conditions {
		conditions = append(conditions, NodeCondition{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}
	return NodeDetail{
		NodeView:   view,
		Conditions: conditions,
		Taints:     n.Spec.Taints,
	}
}

type NamespaceView struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Age    string `json:"age"`
}

func ToNamespaceViews(items []corev1.Namespace) []NamespaceView {
	out := make([]NamespaceView, 0, len(items))
	for _, ns := range items {
		out = append(out, NamespaceView{
			Name:   ns.Name,
			Status: string(ns.Status.Phase),
			Age:    formatAge(ns.CreationTimestamp.Time),
		})
	}
	return out
}

// ContainerView 容器信息
type ContainerView struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	Ready        bool   `json:"ready"`
	State        string `json:"state"`
	RestartCount int32  `json:"restart_count"`
}

// PodView Pod 列表视图
type PodView struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Status            string            `json:"status"`
	Node              string            `json:"node"`
	IP                string            `json:"ip"`
	Age               string            `json:"age"`
	CreationTimestamp time.Time         `json:"creation_timestamp"`
	RestartCount      int32             `json:"restart_count"`
	Labels            map[string]string `json:"labels"`
	Containers        []ContainerView   `json:"containers"`
}

func ToPodViews(items []corev1.Pod) []PodView {
	out := make([]PodView, 0, len(items))
	for _, p := range items {
		containers := make([]ContainerView, 0, len(p.Status.ContainerStatuses))
		var totalRestart int32
		for _, cs := range p.Status.ContainerStatuses {
			state := "Unknown"
			if cs.State.Running != nil {
				state = "Running"
			} else if cs.State.Waiting != nil {
				state = "Waiting: " + cs.State.Waiting.Reason
			} else if cs.State.Terminated != nil {
				state = "Terminated: " + cs.State.Terminated.Reason
			}
			containers = append(containers, ContainerView{
				Name:         cs.Name,
				Image:        cs.Image,
				Ready:        cs.Ready,
				State:        state,
				RestartCount: cs.RestartCount,
			})
			totalRestart += cs.RestartCount
		}
		out = append(out, PodView{
			Name:              p.Name,
			Namespace:         p.Namespace,
			Status:            string(p.Status.Phase),
			Node:              p.Spec.NodeName,
			IP:                p.Status.PodIP,
			Age:               formatAge(p.CreationTimestamp.Time),
			CreationTimestamp: p.CreationTimestamp.Time,
			RestartCount:      totalRestart,
			Labels:            p.Labels,
			Containers:        containers,
		})
	}
	return out
}

// PodDetail Pod 详情（包含 YAML）
type PodDetail struct {
	PodView
	YAML string `json:"yaml"`
}

// WorkloadView Deployment/StatefulSet/DaemonSet 列表视图
type WorkloadView struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Ready             string            `json:"ready"`
	Replicas          int32             `json:"replicas"`
	Available         int32             `json:"available"`
	Updated           int32             `json:"updated"`
	Strategy          string            `json:"strategy"`
	Images            []string          `json:"images"`
	Age               string            `json:"age"`
	CreationTimestamp time.Time         `json:"creation_timestamp"`
	Labels            map[string]string `json:"labels"`
}

func ToDeploymentViews(items []appsv1.Deployment) []WorkloadView {
	out := make([]WorkloadView, 0, len(items))
	for _, d := range items {
		replicas := int32(0)
		if d.Spec.Replicas != nil {
			replicas = *d.Spec.Replicas
		}
		images := extractImages(d.Spec.Template.Spec.Containers)
		out = append(out, WorkloadView{
			Name:              d.Name,
			Namespace:         d.Namespace,
			Ready:             formatReady(d.Status.ReadyReplicas, replicas),
			Replicas:          replicas,
			Available:         d.Status.AvailableReplicas,
			Updated:           d.Status.UpdatedReplicas,
			Strategy:          string(d.Spec.Strategy.Type),
			Images:            images,
			Age:               formatAge(d.CreationTimestamp.Time),
			CreationTimestamp: d.CreationTimestamp.Time,
			Labels:            d.Labels,
		})
	}
	return out
}

func ToStatefulSetViews(items []appsv1.StatefulSet) []WorkloadView {
	out := make([]WorkloadView, 0, len(items))
	for _, d := range items {
		replicas := int32(0)
		if d.Spec.Replicas != nil {
			replicas = *d.Spec.Replicas
		}
		images := extractImages(d.Spec.Template.Spec.Containers)
		out = append(out, WorkloadView{
			Name:              d.Name,
			Namespace:         d.Namespace,
			Ready:             formatReady(d.Status.ReadyReplicas, replicas),
			Replicas:          replicas,
			Available:         d.Status.CurrentReplicas,
			Updated:           d.Status.UpdatedReplicas,
			Strategy:          string(d.Spec.UpdateStrategy.Type),
			Images:            images,
			Age:               formatAge(d.CreationTimestamp.Time),
			CreationTimestamp: d.CreationTimestamp.Time,
			Labels:            d.Labels,
		})
	}
	return out
}

func ToDaemonSetViews(items []appsv1.DaemonSet) []WorkloadView {
	out := make([]WorkloadView, 0, len(items))
	for _, d := range items {
		images := extractImages(d.Spec.Template.Spec.Containers)
		out = append(out, WorkloadView{
			Name:              d.Name,
			Namespace:         d.Namespace,
			Ready:             formatReady(d.Status.NumberReady, d.Status.DesiredNumberScheduled),
			Replicas:          d.Status.DesiredNumberScheduled,
			Available:         d.Status.NumberAvailable,
			Updated:           d.Status.UpdatedNumberScheduled,
			Strategy:          string(d.Spec.UpdateStrategy.Type),
			Images:            images,
			Age:               formatAge(d.CreationTimestamp.Time),
			CreationTimestamp: d.CreationTimestamp.Time,
			Labels:            d.Labels,
		})
	}
	return out
}

type ServiceView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	ClusterIP string `json:"cluster_ip"`
	Age       string `json:"age"`
	Ports     []string `json:"ports"`
}

func ToServiceViews(items []corev1.Service) []ServiceView {
	out := make([]ServiceView, 0, len(items))
	for _, svc := range items {
		ports := make([]string, 0, len(svc.Spec.Ports))
		for _, p := range svc.Spec.Ports {
			ports = append(ports, strconv.Itoa(int(p.Port))+"/"+string(p.Protocol))
		}
		out = append(out, ServiceView{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			ClusterIP: svc.Spec.ClusterIP,
			Age:       formatAge(svc.CreationTimestamp.Time),
			Ports:     ports,
		})
	}
	return out
}

type ConfigMapView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Keys      int    `json:"keys"`
	Age       string `json:"age"`
}

func ToConfigMapViews(items []corev1.ConfigMap) []ConfigMapView {
	out := make([]ConfigMapView, 0, len(items))
	for _, cm := range items {
		out = append(out, ConfigMapView{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Keys:      len(cm.Data) + len(cm.BinaryData),
			Age:       formatAge(cm.CreationTimestamp.Time),
		})
	}
	return out
}

type SecretView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Keys      int    `json:"keys"`
	Age       string `json:"age"`
}

func ToSecretViews(items []corev1.Secret) []SecretView {
	out := make([]SecretView, 0, len(items))
	for _, sec := range items {
		out = append(out, SecretView{
			Name:      sec.Name,
			Namespace: sec.Namespace,
			Type:      string(sec.Type),
			Keys:      len(sec.Data),
			Age:       formatAge(sec.CreationTimestamp.Time),
		})
	}
	return out
}

// EventView 事件视图
type EventView struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Count     int32     `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

func ToEventViews(items []corev1.Event) []EventView {
	out := make([]EventView, 0, len(items))
	for _, e := range items {
		out = append(out, EventView{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Count:     e.Count,
			FirstSeen: e.FirstTimestamp.Time,
			LastSeen:  e.LastTimestamp.Time,
		})
	}
	return out
}

func formatReady(ready, total int32) string {
	return strconv.Itoa(int(ready)) + "/" + strconv.Itoa(int(total))
}

func extractImages(containers []corev1.Container) []string {
	images := make([]string, 0, len(containers))
	for _, c := range containers {
		images = append(images, c.Image)
	}
	return images
}

// formatAge 计算年龄，返回简洁字符串如 "2d", "3h", "15m"
func formatAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	if d < 0 {
		return "-"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return strconv.Itoa(days) + "d" + strconv.Itoa(hours) + "h"
	}
	if hours > 0 {
		return strconv.Itoa(hours) + "h" + strconv.Itoa(minutes) + "m"
	}
	if minutes > 0 {
		return strconv.Itoa(minutes) + "m"
	}
	return strconv.Itoa(int(d.Seconds())) + "s"
}

// formatAgeVerbose 详细年龄
func formatAgeVerbose(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	if d < 0 {
		return "-"
	}
	parts := []string{}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		parts = append(parts, strconv.Itoa(days)+"天")
	}
	if hours > 0 {
		parts = append(parts, strconv.Itoa(hours)+"小时")
	}
	if minutes > 0 {
		parts = append(parts, strconv.Itoa(minutes)+"分钟")
	}
	if len(parts) == 0 {
		return strconv.Itoa(int(d.Seconds())) + "秒"
	}
	return strings.Join(parts, "")
}
