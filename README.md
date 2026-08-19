# AIOps Platform

企业级智能运维平台后端（Go）。当前完成：

- 第一阶段：可运行的基础骨架（配置、日志、HTTP、MySQL、Redis、健康检查、Swagger、Docker、K8s 清单）
- 第二阶段：Kubernetes 多集群只读查询（Node / Namespace / Pod / Deployment / StatefulSet / DaemonSet / Service / ConfigMap / Secret 元数据）

本 README 按「Java 开发者学 Go」来写：每个目录干什么、怎么跑、怎么验证。

## 为什么目录是这样

推荐的 `aiops-platform/` 结构大体保留。做了 3 处调整：

1. **入口在 `cmd/server/main.go`，而不是项目根目录 `main.go`**  
   这是 Go 官方推荐布局。以后如果加 `cmd/worker`（告警消费进程），不会和 API 进程挤在一起。

2. **`internal/` 里暂时没有写满 ai / rca / anomaly 代码**  
   那些目录只放了 README 占位。原因：你要求「第一步先能跑」，空实现会让人误以为已经有 AI 能力。

3. **新增 `internal/infra` 和 `internal/cluster`**  
   MySQL/Redis 连接属于基础设施，Kubernetes 客户端属于集群模块。比全部塞进 `handler` 更接近 Spring 的 `config` + `service` 分层。

对应关系（方便你对照 Spring Boot）：

| Spring Boot | 本项目 |
|-------------|--------|
| `application.yml` | `configs/config.yaml` |
| `@Configuration` | `internal/config` |
| `DataSource` / RedisTemplate | `internal/infra` |
| `Controller` | `internal/handler` |
| `Service` | `internal/cluster` |
| `Filter` | `internal/middleware` |
| 统一返回体 | `pkg/response` |

## 本地运行（逐步验证）

### 0. 准备

- Go 1.24+
- MySQL 8.0
- Redis
- （第二阶段）本机 `kubectl` 能连上的集群，kubeconfig 一般在 `~/.kube/config`

Windows PowerShell：

```powershell
cd C:\Users\Administrator\Desktop\aiops-platform
copy configs\config.example.yaml configs\config.yaml
copy configs\clusters.example.yaml configs\clusters.yaml
```

修改 `configs/config.yaml` 里的 MySQL 密码。这两个文件已在 `.gitignore`，不要提交。

创建数据库：

```sql
CREATE DATABASE IF NOT EXISTS aiops DEFAULT CHARACTER SET utf8mb4;
CREATE USER IF NOT EXISTS 'aiops'@'%' IDENTIFIED BY 'aiops123';
GRANT ALL ON aiops.* TO 'aiops'@'%';
```

### 1. 拉依赖并跑测试

```powershell
go mod tidy
go fmt ./...
go vet ./...
go test ./...
```

测试不连真实 MySQL / 真实集群，用的是 Kubernetes `fake` 客户端。

### 2. 启动 API

```powershell
go run ./cmd/server
```

看到 `aiops-platform started` 后：

```powershell
curl http://127.0.0.1:8080/health
curl http://127.0.0.1:8080/ready
```

- `/health`：进程活着（类似 liveness）
- `/ready`：MySQL、Redis 都能 ping（类似 readiness）

Swagger UI：浏览器打开 `http://127.0.0.1:8080/swagger/index.html`

### 3. 验证 Kubernetes 查询

```powershell
curl http://127.0.0.1:8080/api/v1/clusters
curl "http://127.0.0.1:8080/api/v1/nodes?cluster=local"
curl "http://127.0.0.1:8080/api/v1/pods?cluster=local&namespace=default"
curl "http://127.0.0.1:8080/api/v1/deployments?cluster=local&namespace=default"
curl "http://127.0.0.1:8080/api/v1/services?cluster=local&namespace=default"
```

`cluster` 不传时，使用配置里第一个 `enabled: true` 的集群。  
`namespace` 不传时，查询所有命名空间。

Secret 接口只返回名称、类型、key 数量，**不会返回明文**。

## 多集群怎么配（不要写死 kubeconfig）

编辑 `configs/clusters.yaml`（已 gitignore）：

```yaml
clusters:
  - name: local
    enabled: true
    auth_type: kubeconfig
    kubeconfig_path: "~/.kube/config"

  - name: prod
    enabled: true
    auth_type: serviceaccount
    api_server: https://your-apiserver:6443
    token_file: secrets/prod/token
    ca_file: secrets/prod/ca.crt
```

三种认证：

| auth_type | 什么时候用 |
|-----------|------------|
| `kubeconfig` | 开发机，指向 kubeconfig **文件路径** |
| `serviceaccount` | 远程集群，Token / CA 都用 **文件**，不要把内容贴进 YAML 仓库 |
| `incluster` | 本程序跑在 Kubernetes 里，用 Pod 自带的 ServiceAccount |

密钥禁止进 Git：`.gitignore` 已忽略 `configs/config.yaml`、`configs/clusters.yaml`、`secrets/`、`*.kubeconfig`。

## Docker / Kubernetes

```powershell
docker compose up -d --build
kubectl apply -f deployments/kubernetes/aiops.yaml
```

集群内 Deployment 默认 `auth_type: incluster`，并绑定只读 ClusterRole。

## 下一步（还没做，避免一次堆太多）

1. JWT + RBAC + 审计日志  
2. Prometheus / Grafana 指标接入  
3. Alertmanager 告警聚合  
4. ELK 日志查询  
5. 异常检测 / RCA / AI 助手  

你确认第一、二阶段能在本机跑通后，我们再做其中一块。
