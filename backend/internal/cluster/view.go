package cluster

import (
	"strconv"

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

type NodeView struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version"`
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
		out = append(out, NodeView{
			Name:    n.Name,
			Status:  status,
			Version: n.Status.NodeInfo.KubeletVersion,
		})
	}
	return out
}

type NamespaceView struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func ToNamespaceViews(items []corev1.Namespace) []NamespaceView {
	out := make([]NamespaceView, 0, len(items))
	for _, ns := range items {
		out = append(out, NamespaceView{Name: ns.Name, Status: string(ns.Status.Phase)})
	}
	return out
}

type PodView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Node      string `json:"node"`
}

func ToPodViews(items []corev1.Pod) []PodView {
	out := make([]PodView, 0, len(items))
	for _, p := range items {
		out = append(out, PodView{
			Name:      p.Name,
			Namespace: p.Namespace,
			Status:    string(p.Status.Phase),
			Node:      p.Spec.NodeName,
		})
	}
	return out
}

type WorkloadView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"`
}

func ToDeploymentViews(items []appsv1.Deployment) []WorkloadView {
	out := make([]WorkloadView, 0, len(items))
	for _, d := range items {
		replicas := int32(0)
		if d.Spec.Replicas != nil {
			replicas = *d.Spec.Replicas
		}
		out = append(out, WorkloadView{
			Name:      d.Name,
			Namespace: d.Namespace,
			Ready:     formatReady(d.Status.ReadyReplicas, replicas),
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
		out = append(out, WorkloadView{
			Name:      d.Name,
			Namespace: d.Namespace,
			Ready:     formatReady(d.Status.ReadyReplicas, replicas),
		})
	}
	return out
}

func ToDaemonSetViews(items []appsv1.DaemonSet) []WorkloadView {
	out := make([]WorkloadView, 0, len(items))
	for _, d := range items {
		out = append(out, WorkloadView{
			Name:      d.Name,
			Namespace: d.Namespace,
			Ready:     formatReady(d.Status.NumberReady, d.Status.DesiredNumberScheduled),
		})
	}
	return out
}

type ServiceView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	ClusterIP string `json:"cluster_ip"`
}

func ToServiceViews(items []corev1.Service) []ServiceView {
	out := make([]ServiceView, 0, len(items))
	for _, svc := range items {
		out = append(out, ServiceView{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			ClusterIP: svc.Spec.ClusterIP,
		})
	}
	return out
}

type ConfigMapView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Keys      int    `json:"keys"`
}

func ToConfigMapViews(items []corev1.ConfigMap) []ConfigMapView {
	out := make([]ConfigMapView, 0, len(items))
	for _, cm := range items {
		out = append(out, ConfigMapView{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Keys:      len(cm.Data) + len(cm.BinaryData),
		})
	}
	return out
}

type SecretView struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Keys      int    `json:"keys"`
}

func ToSecretViews(items []corev1.Secret) []SecretView {
	out := make([]SecretView, 0, len(items))
	for _, sec := range items {
		out = append(out, SecretView{
			Name:      sec.Name,
			Namespace: sec.Namespace,
			Type:      string(sec.Type),
			Keys:      len(sec.Data),
		})
	}
	return out
}

func formatReady(ready, total int32) string {
	return strconv.Itoa(int(ready)) + "/" + strconv.Itoa(int(total))
}
