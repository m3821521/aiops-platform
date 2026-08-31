# AIOps Platform Production Deployment Guide

**版本**: 1.0.0
**适用**: 生产环境部署
**架构**: Single-Port (Backend :8080 serves API + Frontend SPA)

---

## 1. Prerequisites

### 硬件要求
| 组件 | 最低 | 推荐 |
|------|------|------|
| CPU | 2 cores | 4+ cores |
| Memory | 4GB | 8GB+ |
| Disk | 20GB | 50GB+ (含日志/ES数据) |

### 软件要求
- Docker 20.10+ / Docker Compose v2
- 或 Kubernetes 1.24+ (推荐生产)
- MySQL 8.0+
- Redis 7+
- Kubernetes cluster (用于 K8s 监控功能)
- metrics-server (K8s 集群内)
- AI Provider API Key (DeepSeek / OpenAI-compatible)

### 网络要求
- Backend 端口 8080 可访问
- MySQL 3306 / Redis 6379 内部可达
- Kubernetes API Server 可达
- AI Provider API 可达 (https://api.deepseek.com 或自定义)
- 可选: Prometheus 9090 / Alertmanager 9093 / Elasticsearch 9200

---

## 2. Infrastructure

### 推荐架构
```
                    ┌─────────────┐
                    │   Nginx/LB  │ (可选 TLS 终止)
                    └──────┬──────┘
                           │ :8080
                    ┌──────▼──────┐
                    │ AIOps Backend│ (API + Frontend SPA)
                    └──┬───┬───┬──┘
                       │   │   │
              ┌────────┘   │   └────────┐
              ▼            ▼            ▼
         ┌────────┐  ┌────────┐  ┌──────────┐
         │ MySQL  │  │ Redis  │  │ K8s API  │
         └────────┘  └────────┘  └──────────┘
              │            │            │
         ┌────┴────┐  ┌───┴─────┐  ┌──┴──────────┐
         │ aiops db│  │ cache   │  │ metrics-srv │
         └─────────┘  └─────────┘  └─────────────┘
```

### 外部依赖（可选，通过 External Connections 配置）
- Prometheus (历史指标)
- Alertmanager (告警管理)
- Elasticsearch (日志搜索)
- Grafana (仪表盘嵌入)
- Jenkins (CI/CD 自动化)
- ArgoCD (GitOps 自动化)

---

## 3. Database

### MySQL 初始化
docker-compose 自动执行 migration（挂载 `backend/migrations/` 到 `/docker-entrypoint-initdb.d`）。

手动初始化:
```bash
# 创建数据库
mysql -h <host> -u root -p -e "CREATE DATABASE aiops CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

# 创建用户
mysql -h <host> -u root -p -e "CREATE USER 'aiops'@'%' IDENTIFIED BY '<strong-password>'; GRANT ALL PRIVILEGES ON aiops.* TO 'aiops'@'%'; FLUSH PRIVILEGES;"

# 执行 migration
mysql -h <host> -u aiops -p aiops < backend/migrations/001_init.sql
```

### 环境变量
```bash
MYSQL_HOST=mysql
MYSQL_PORT=3306
MYSQL_DATABASE=aiops
MYSQL_USER=aiops
MYSQL_PASSWORD=<strong-password>
```

---

## 4. Redis

```bash
REDIS_ADDR=redis:6379
REDIS_PASSWORD=<redis-password>  # 可选
REDIS_DB=0
```

Redis 用途:
- Session / Token 黑名单
- Rate limiting
- 临时缓存
- AI request 状态

---

## 5. Kubernetes

### kubeconfig 配置
通过 AIOps Platform UI → 系统管理 → 外部连接 → 添加 Kubernetes 连接:
- 名称: 例如 `prod-cluster`
- API Endpoint: `https://<k8s-api-host>:6443`
- Auth Type: kubeconfig / token / cert

或通过环境变量配置默认集群（开发用）:
```bash
KUBECONFIG=/path/to/kubeconfig
```

### RBAC 要求
AIOps Platform ServiceAccount 需要以下权限:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aiops-reader
rules:
- apiGroups: [""]
  resources: ["nodes", "pods", "namespaces", "services", "endpoints", "events"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["metrics.k8s.io"]
  resources: ["nodes", "pods"]
  verbs: ["get", "list"]
- apiGroups: [""]
  resources: ["pods/exec", "pods/log"]
  verbs: ["get", "create"]  # 仅 Automation 功能需要
```

---

## 6. Metrics Server

生产 Kubernetes 集群必须部署 metrics-server 才能显示 Node/Pod CPU/Memory。

### 部署
```bash
# Helm (推荐)
helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
helm upgrade --install metrics-server metrics-server/metrics-server \
  --namespace kube-system \
  --set args={--kubelet-insecure-tls}  # 仅自签名证书环境

# 或 YAML
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

### 验证
```bash
kubectl get apiservice | grep metrics
# v1beta1.metrics.k8s.io   kube-system/metrics-server   True

kubectl top nodes
kubectl top pods -A
```

---

## 7. Prometheus (可选)

通过 External Connections 配置，无隐式默认。

```bash
PROMETHEUS_URL=http://prometheus:9090
PROMETHEUS_TIMEOUT=30
```

用途:
- 历史指标查询
- 异常检测 (Anomaly Detection)
- RCA 历史证据
- 趋势图

---

## 8. Alertmanager (可选)

```bash
ALERTMANAGER_URL=http://alertmanager:9093
```

用途:
- 告警查询
- Silence 管理
- 告警路由配置查看

---

## 9. Elasticsearch (可选)

```bash
ELASTICSEARCH_URL=http://elasticsearch:9200
ELASTICSEARCH_INDEX=filebeat-*
ELASTICSEARCH_USERNAME=elastic
ELASTICSEARCH_PASSWORD=<password>
```

用途:
- 日志搜索 (AI search_logs tool)
- RCA 日志证据
- 日志中心页面

---

## 10. AI Provider

### 必需配置
```bash
AI_PROVIDER=deepseek
AI_BASE_URL=https://api.deepseek.com
AI_API_KEY=<your-api-key>
AI_MODEL=deepseek-v4-flash
```

### 支持的 Provider
- DeepSeek (默认)
- OpenAI-compatible (自定义 base_url)

### 超时配置
- Backend overall timeout: 60s
- Provider HTTP timeout: 55s (streaming) / 60s (non-streaming)
- Tool timeout: 5s
- MaxToolCalls: 8

### 验证
```bash
curl -X POST http://localhost:8080/api/v1/ai/ask \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"question":"你好"}'
```

---

## 11. Secrets

### 必需 Secret
| Secret | 用途 | 示例 |
|--------|------|------|
| MYSQL_PASSWORD | 数据库密码 | 强密码，16+ 字符 |
| REDIS_PASSWORD | Redis 密码（可选） | 强密码 |
| JWT_SECRET | JWT 签名密钥 | 随机 32+ 字节 |
| AI_API_KEY | AI Provider API Key | sk-xxx |

### 生产环境 Secret 管理
- **Docker**: 使用 Docker Secrets / `.env` 文件（chmod 600）
- **Kubernetes**: 使用 Kubernetes Secrets + envFrom
- **云平台**: 使用云厂商 Secret Manager（AWS Secrets Manager / GCP Secret Manager / Azure Key Vault）

### 禁止
- ❌ 硬编码 Secret 到代码
- ❌ 提交 `.env` 到 Git
- ❌ 在日志中打印 API Key / Password / Authorization Header
- ❌ 在 SSE event 中传递敏感信息

---

## 12. Build

### Docker Build (推荐)
```bash
# 从项目根目录构建（build context 必须包含 frontend/ 和 backend/）
docker build -f backend/Dockerfile -t aiops-platform:1.0.0 .

# 验证镜像
docker image inspect aiops-platform:1.0.0 | grep -E "Size|Created"
```

### 手动 Build
```bash
# Frontend
cd frontend
npm ci
npm run build
cd ..

# Backend
cd backend
go mod download
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o aiops-server ./cmd/server
cd ..
```

---

## 13. Deploy

### Docker Compose (快速部署 / 测试)
```bash
# 1. 复制环境变量模板
cp .env.example .env
# 编辑 .env，填入真实密码和 API Key

# 2. 启动所有服务
docker compose up -d

# 3. 查看状态
docker compose ps
docker compose logs -f backend
```

### Docker Run (仅 Backend，外部 DB)
```bash
docker run -d \
  --name aiops-platform \
  -p 8080:8080 \
  -e MYSQL_HOST=mysql-host \
  -e MYSQL_PORT=3306 \
  -e MYSQL_DATABASE=aiops \
  -e MYSQL_USER=aiops \
  -e MYSQL_PASSWORD=<password> \
  -e REDIS_ADDR=redis-host:6379 \
  -e JWT_SECRET=<jwt-secret> \
  -e AI_API_KEY=<api-key> \
  -v /path/to/kubeconfig:/root/.kube/config:ro \
  --restart unless-stopped \
  aiops-platform:1.0.0
```

### Kubernetes Deployment (生产推荐)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aiops-platform
  namespace: aiops
spec:
  replicas: 2
  selector:
    matchLabels:
      app: aiops-platform
  template:
    metadata:
      labels:
        app: aiops-platform
    spec:
      containers:
      - name: backend
        image: aiops-platform:1.0.0
        ports:
        - containerPort: 8080
        envFrom:
        - secretRef:
            name: aiops-secrets
        - configMapRef:
            name: aiops-config
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2
            memory: 2Gi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: kubeconfig
          mountPath: /root/.kube/config
          subPath: config
          readOnly: true
      volumes:
      - name: kubeconfig
        secret:
          secretName: aiops-kubeconfig
---
apiVersion: v1
kind: Service
metadata:
  name: aiops-platform
  namespace: aiops
spec:
  selector:
    app: aiops-platform
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: aiops-platform
  namespace: aiops
  annotations:
    nginx.ingress.kubernetes.io/proxy-read-timeout: "300"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "300"
spec:
  rules:
  - host: aiops.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: aiops-platform
            port:
              number: 80
```

**注意**: SSE Streaming 需要 Nginx 配置 `proxy-buffering: off` 和较长的 read timeout。

---

## 14. Health Check

```bash
# Liveness
curl -s http://localhost:8080/health
# {"code":0,"data":{"status":"ok"}}

# Readiness (检查 MySQL + Redis)
curl -s http://localhost:8080/ready
# {"code":200,"data":{"status":"ok","checks":{"mysql":"up","redis":"up"}}}
```

### Kubernetes Probe
- Liveness: `GET /health` (应用进程存活)
- Readiness: `GET /ready` (MySQL + Redis 连接正常，可接收流量)

---

## 15. Smoke Test

部署后执行完整冒烟测试，详见 [PRODUCTION_SMOKE_TEST.md](./PRODUCTION_SMOKE_TEST.md)。

快速验证:
```bash
# 1. Health
curl -s http://localhost:8080/health | python3 -m json.tool

# 2. Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<admin-password>"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")

# 3. Kubernetes Nodes
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/nodes?cluster=<cluster-name>" | python3 -m json.tool

# 4. AI Ask
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"question":"你好"}' \
  http://localhost:8080/api/v1/ai/ask | python3 -m json.tool
```

---

## 16. Monitoring

### 应用日志
```bash
# Docker
docker compose logs -f backend

# Kubernetes
kubectl logs -f deployment/aiops-platform -n aiops
```

日志格式: 结构化 JSON (slog)，包含 request_id / user_id / method / path / status / cost_ms。

### 关键指标监控
建议配置 Prometheus 监控以下指标:
- `http_requests_total` (按 method/path/status)
- `http_request_duration_seconds` (histogram)
- `ai_request_duration_seconds` (AI 请求耗时)
- `ai_sse_ttf_seconds` (首 token 时间)
- `db_connection_pool_*`
- `redis_*`

### 告警建议
- Backend 5xx 率 > 5% 持续 5 分钟
- AI 请求 P95 > 50s
- MySQL 连接池耗尽
- Redis 连接失败
- Pod 重启次数 > 3 次/小时

---

## 17. Rollback

详见 [PRODUCTION_ROLLBACK.md](./PRODUCTION_ROLLBACK.md)。

快速回滚:
```bash
# Docker Compose
docker compose down
docker compose up -d backend  # 使用上一个镜像 tag

# Kubernetes
kubectl rollout undo deployment/aiops-platform -n aiops
kubectl rollout status deployment/aiops-platform -n aiops
```

---

## 18. Troubleshooting

详见 [PRODUCTION_TROUBLESHOOTING.md](./PRODUCTION_TROUBLESHOOTING.md)。

### 常见问题
1. **Backend 启动失败**: 检查 MySQL/Redis 连接配置
2. **AI 返回 503**: 检查 AI_API_KEY 是否配置
3. **Kubernetes API 失败**: 检查 kubeconfig / RBAC 权限
4. **Metrics 显示 `—`**: 检查 K8s 集群 metrics-server 是否部署
5. **SSE 连接中断**: 检查 Nginx proxy-buffering / proxy-read-timeout 配置

---

## 19. Backup

### MySQL 备份
```bash
# 定时备份（cron）
0 2 * * * mysqldump -h <host> -u aiops -p aiops | gzip > /backup/aiops_$(date +\%Y\%m\%d).sql.gz

# 保留 30 天
find /backup -name "aiops_*.sql.gz" -mtime +30 -delete
```

### Redis 备份
```bash
# RDB 快照（redis.conf 配置 save 规则）
redis-cli BGSAVE
cp /data/dump.rdb /backup/redis_$(date +%Y%m%d).rdb
```

### 配置备份
- `.env` 文件
- kubeconfig
- External Connections 配置（数据库中，随 MySQL 备份）

---

## 20. Security Checklist

部署前确认:
- [ ] MySQL 使用强密码，非 root 用户
- [ ] Redis 设置密码（生产环境）
- [ ] JWT_SECRET 使用随机 32+ 字节
- [ ] AI_API_KEY 不硬编码，通过环境变量/Secret 注入
- [ ] `.env` 文件权限 chmod 600
- [ ] 不提交 `.env` / kubeconfig / secret 到 Git
- [ ] Backend 不监听 0.0.0.0（除非需要），通过 Nginx/LB 反向代理
- [ ] 启用 HTTPS（Nginx/LB TLS 终止）
- [ ] 配置 Rate Limiting（Nginx / 应用层）
- [ ] 审计日志开启（Audit Log 功能）
- [ ] RBAC 权限正确配置（普通用户无管理员权限）
- [ ] SSE endpoint 不泄露敏感信息（仅 request_id / text / error_type）
- [ ] 日志不打印 API Key / Password / Authorization / 完整 Prompt
