package providers

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/aiops/aiops-platform/internal/connection"
)

// KubernetesProvider 实现 connection.KubernetesProvider 接口。
//
// 功能：
//   - 从 Connection 获取 endpoint 和 credential
//   - 通过 Credential Manager 解密 kubeconfig/token
//   - 创建 client-go 客户端
//   - 提供 Kubernetes 资源访问能力
//
// 注意：
//   - 这是 Phase B 的实现，当前只提供基础能力
//   - 业务代码迁移在后续阶段逐步进行
//   - 保持与现有 cluster.Manager 的兼容性
type KubernetesProvider struct {
	credentialService *connection.CredentialService
}

// NewKubernetesProvider 创建 Kubernetes Provider。
func NewKubernetesProvider(credentialService *connection.CredentialService) *KubernetesProvider {
	return &KubernetesProvider{
		credentialService: credentialService,
	}
}

// Type 返回 Provider 类型。
func (p *KubernetesProvider) Type() connection.ConnectionType {
	return connection.TypeKubernetes
}

// Test 测试 Kubernetes 连接。
// 执行：创建 client → 调用 /version API → 返回结果。
func (p *KubernetesProvider) Test(ctx context.Context, conn *connection.Connection) (*connection.TestConnectionResult, error) {
	start := time.Now()

	client, err := p.Connect(ctx, conn)
	if err != nil {
		return &connection.TestConnectionResult{
			Status:       connection.StatusUnavailable,
			LatencyMs:    time.Since(start).Milliseconds(),
			ErrorCode:    "CONNECT_FAILED",
			ErrorMessage: err.Error(),
			CheckedAt:    time.Now(),
		}, nil
	}

	k8sClient, ok := client.(kubernetes.Interface)
	if !ok {
		return &connection.TestConnectionResult{
			Status:       connection.StatusUnavailable,
			LatencyMs:    time.Since(start).Milliseconds(),
			ErrorCode:    "INVALID_CLIENT",
			ErrorMessage: "创建的客户端类型无效",
			CheckedAt:    time.Now(),
		}, nil
	}

	// 调用 Kubernetes Version API 测试连通性
	version, err := k8sClient.Discovery().ServerVersion()
	if err != nil {
		return &connection.TestConnectionResult{
			Status:       connection.StatusUnavailable,
			LatencyMs:    time.Since(start).Milliseconds(),
			ErrorCode:    "API_CALL_FAILED",
			ErrorMessage: fmt.Sprintf("Kubernetes API 调用失败: %v", err),
			CheckedAt:    time.Now(),
		}, nil
	}

	return &connection.TestConnectionResult{
		Status:    connection.StatusAvailable,
		LatencyMs: time.Since(start).Milliseconds(),
		CheckedAt: time.Now(),
		ErrorMessage: fmt.Sprintf("Kubernetes %s", version.GitVersion),
	}, nil
}

// Connect 创建 Kubernetes 客户端。
// 支持的认证方式：
//   - kubeconfig: Credential 中包含 kubeconfig 字段
//   - token: Credential 中包含 token 字段
//   - in-cluster: Connection config 中设置 in_cluster=true
func (p *KubernetesProvider) Connect(ctx context.Context, conn *connection.Connection) (interface{}, error) {
	if conn == nil {
		return nil, fmt.Errorf("connection 不能为空")
	}

	// 检查是否启用
	if !conn.Enabled {
		return nil, fmt.Errorf("connection 已禁用: %s", conn.Name)
	}

	// 检查是否使用 in-cluster 配置
	if conn.Config != nil {
		if inCluster, ok := conn.Config["in_cluster"]; ok && inCluster == "true" {
			cfg, err := rest.InClusterConfig()
			if err != nil {
				return nil, fmt.Errorf("获取 in-cluster 配置失败: %w", err)
			}
			return kubernetes.NewForConfig(cfg)
		}
	}

	// 从 Credential 获取认证信息
	if conn.CredentialID == nil {
		// 没有 Credential，尝试使用 endpoint 作为 API Server 地址，匿名访问
		cfg := &rest.Config{
			Host: conn.Endpoint,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		}
		return kubernetes.NewForConfig(cfg)
	}

	// 解密 Credential
	data, err := p.credentialService.GetDecryptedData(ctx, *conn.CredentialID)
	if err != nil {
		return nil, fmt.Errorf("解密 credential 失败: %w", err)
	}

	// 优先使用 kubeconfig
	if kubeconfig, ok := data["kubeconfig"]; ok && kubeconfig != "" {
		// 尝试解码 base64（如果是 base64 编码的）
		kubeconfigBytes := []byte(kubeconfig)
		if decoded, err := base64.StdEncoding.DecodeString(kubeconfig); err == nil {
			kubeconfigBytes = decoded
		}

		cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
		if err != nil {
			return nil, fmt.Errorf("解析 kubeconfig 失败: %w", err)
		}
		// 如果 Connection 中指定了 endpoint，覆盖 kubeconfig 中的地址
		if conn.Endpoint != "" {
			cfg.Host = conn.Endpoint
		}
		return kubernetes.NewForConfig(cfg)
	}

	// 使用 token 认证
	if token, ok := data["token"]; ok && token != "" {
		cfg := &rest.Config{
			Host:        conn.Endpoint,
			BearerToken: token,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		}
		// 如果有 CA 证书
		if ca, ok := data["ca_cert"]; ok && ca != "" {
			cfg.TLSClientConfig.CAData = []byte(ca)
			cfg.TLSClientConfig.Insecure = false
		}
		return kubernetes.NewForConfig(cfg)
	}

	// 使用 username/password 认证
	username, hasUser := data["username"]
	password, hasPass := data["password"]
	if hasUser && hasPass {
		cfg := &rest.Config{
			Host:     conn.Endpoint,
			Username: username,
			Password: password,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		}
		return kubernetes.NewForConfig(cfg)
	}

	return nil, fmt.Errorf("credential 中没有有效的认证信息 (需要 kubeconfig/token/username+password)")
}

// ListNodes 获取节点列表。
func (p *KubernetesProvider) ListNodes(ctx context.Context, conn *connection.Connection) ([]interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}

	k8sClient, ok := client.(kubernetes.Interface)
	if !ok {
		return nil, fmt.Errorf("无效的 Kubernetes 客户端")
	}

	nodes, err := k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取节点列表失败: %w", err)
	}

	result := make([]interface{}, len(nodes.Items))
	for i := range nodes.Items {
		result[i] = nodes.Items[i]
	}
	return result, nil
}

// ListPods 获取 Pod 列表。
func (p *KubernetesProvider) ListPods(ctx context.Context, conn *connection.Connection, namespace string) ([]interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}

	k8sClient, ok := client.(kubernetes.Interface)
	if !ok {
		return nil, fmt.Errorf("无效的 Kubernetes 客户端")
	}

	pods, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取 Pod 列表失败: %w", err)
	}

	result := make([]interface{}, len(pods.Items))
	for i := range pods.Items {
		result[i] = pods.Items[i]
	}
	return result, nil
}

// GetPod 获取单个 Pod 详情。
func (p *KubernetesProvider) GetPod(ctx context.Context, conn *connection.Connection, namespace, name string) (interface{}, error) {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return nil, err
	}

	k8sClient, ok := client.(kubernetes.Interface)
	if !ok {
		return nil, fmt.Errorf("无效的 Kubernetes 客户端")
	}

	pod, err := k8sClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取 Pod 详情失败: %w", err)
	}
	return pod, nil
}

// RestartPod 重启 Pod（高风险操作，需要审批）。
// 实现：删除 Pod，让 Deployment/StatefulSet 自动重建。
func (p *KubernetesProvider) RestartPod(ctx context.Context, conn *connection.Connection, namespace, name string) error {
	client, err := p.Connect(ctx, conn)
	if err != nil {
		return err
	}

	k8sClient, ok := client.(kubernetes.Interface)
	if !ok {
		return fmt.Errorf("无效的 Kubernetes 客户端")
	}

	// 删除 Pod 以触发重启
	err = k8sClient.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("重启 Pod 失败: %w", err)
	}
	return nil
}
