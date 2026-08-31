# AIOps Platform Production Troubleshooting Guide

**版本**: 1.0.0
**用途**: 生产环境常见问题排查指南

---

## 1. 快速诊断流程

```
问题出现
  ↓
1. 检查 /health 和 /ready
  ↓
2. 检查 Backend 日志
  ↓
3. 检查 MySQL / Redis 连接
  ↓
4. 检查 Kubernetes / 外部服务连接
  ↓
5. 定位根因 → 修复 / 回滚
```

---

## 2. Backend 无法启动

### 症状
- Container/Pod CrashLoopBackOff
- /health 无响应
- 日志显示启动错误

### 排查步骤
```bash
# 1. 查看启动日志
docker compose logs backend
# 或
kubectl logs deployment/aiops-platform -n aiops --previous

# 2. 检查环境变量
docker compose config | grep -E "MYSQL|REDIS|JWT|AI_"
# 或
kubectl describe deployment aiops-platform -n aiops | grep -A 20 "Environment"

# 3. 测试 MySQL 连接
mysql -h <host> -u aiops -p -e "SELECT 1;"

# 4. 测试 Redis 连接
redis-cli -h <host> -p 6379 ping
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| MySQL 连接失败 | 检查 MYSQL_HOST/PORT/USER/PASSWORD，确认 MySQL 运行 |
| Redis 连接失败 | 检查 REDIS_ADDR/PASSWORD，确认 Redis 运行 |
| JWT_SECRET 未设置 | 设置 JWT_SECRET 环境变量（随机 32+ 字节） |
| 端口被占用 | 检查 8080 端口是否被占用: `lsof -i :8080` |
| 数据库 migration 失败 | 手动执行 migration，检查 migration 文件 |
| 配置文件格式错误 | 检查 config.yaml / 环境变量格式 |

---

## 3. /ready 返回失败

### 症状
- `GET /ready` 返回 503
- checks 中 mysql 或 redis 显示 down

### 排查
```bash
# 查看详细错误
curl -s http://localhost:8080/ready | python3 -m json.tool

# MySQL 连接测试
mysql -h <host> -u aiops -p -e "SELECT 1;"

# Redis 连接测试
redis-cli -h <host> -p 6379 ping

# 检查连接池状态
# 日志中搜索 "mysql" / "redis" 错误
docker compose logs backend | grep -iE "mysql|redis|error" | tail -20
```

---

## 4. AI 功能不可用

### 症状
- AI 请求返回 503 "AI 服务不可用"
- AI SSE 连接立即断开
- AI 回答为空

### 排查
```bash
# 1. 检查 AI 配置
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/ai/config | python3 -m json.tool

# 2. 检查环境变量
docker compose config | grep -E "AI_"

# 3. 测试 AI Provider 连通性
curl -s -X POST https://api.deepseek.com/chat/completions \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"max_tokens":10}'

# 4. 查看 AI 相关日志
docker compose logs backend | grep -iE "ai|provider|deepseek|openai" | tail -30
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| AI_API_KEY 未配置 | 设置 AI_API_KEY 环境变量 |
| API Key 无效/过期 | 检查 API Key 有效性，更新 Key |
| Provider 网络不通 | 检查网络/防火墙/DNS，确认可访问 api.deepseek.com |
| Provider 限流 (429) | 等待限流恢复，或升级 API 配额 |
| Provider 服务异常 (5xx) | 检查 Provider 状态页，等待恢复 |
| AI 模型名错误 | 检查 AI_MODEL 配置，使用 Provider 支持的模型名 |

---

## 5. AI SSE Streaming 异常

### 症状
- SSE 连接建立后无 token
- token 断断续续 / 延迟很大
- 连接在 28s 左右断开
- 前端显示超时

### 排查
```bash
# 1. 直接测试 SSE endpoint
curl -s -N -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"question":"hi"}' \
  http://localhost:8080/api/v1/ai/ask/stream

# 2. 检查 Nginx 配置（如果使用 Nginx）
# 必须设置:
# proxy_buffering off;
# proxy_read_timeout 300s;
# proxy_send_timeout 300s;

# 3. 检查 Backend 日志中的 TTFT 和 total duration
docker compose logs backend | grep -iE "stream|ttft|first.token|ai.request" | tail -20
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| Nginx proxy buffering | 设置 `proxy_buffering off` |
| Nginx read timeout 太短 | 设置 `proxy_read_timeout 300s` |
| Provider 首 token 延迟高 | 正常现象（DeepSeek 约 20-30s），等待即可 |
| Frontend Axios timeout | Streaming 使用 fetch，不受 Axios timeout 影响 |
| Backend 60s timeout | 正常边界，Provider 响应超过 60s 会超时 |
| Heartbeat 未发送 | 检查 heartbeat ticker 是否正常（10s 间隔） |

---

## 6. Kubernetes API 失败

### 症状
- Nodes/Pods 页面报错
- API 返回 503 "集群不存在" 或 "Kubernetes 服务不可用"
- 无法获取 K8s 资源

### 排查
```bash
# 1. 检查 Connections 中的 K8s 配置
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/connections | python3 -m json.tool

# 2. 测试 kubeconfig 连通性
kubectl --kubeconfig=/path/to/kubeconfig cluster-info
kubectl --kubeconfig=/path/to/kubeconfig get nodes

# 3. 检查 RBAC 权限
kubectl auth can-i get nodes --as=aiops-serviceaccount
kubectl auth can-i list pods --all-namespaces --as=aiops-serviceaccount

# 4. 查看 K8s 相关日志
docker compose logs backend | grep -iE "kubernetes|k8s|cluster" | tail -20
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| 集群名称错误 | 使用 Connections 中配置的正确 cluster 名称 |
| kubeconfig 无效 | 更新 kubeconfig / token，确认证书未过期 |
| API Server 不可达 | 检查网络/防火墙，确认 API Server 地址和端口 |
| RBAC 权限不足 | 给 ServiceAccount 绑定正确的 ClusterRole |
| TLS 证书错误 | 检查 kubeconfig 中的 CA 证书，或使用 --insecure-skip-tls-verify（不推荐） |

---

## 7. Metrics 显示 `—` 或不可用

### 症状
- Nodes/Pods 页面 CPU/Memory 列显示 `—`
- Metrics API 返回 503
- NodeDetail/PodDetail 资源监控显示 "暂无 Metrics 数据"

### 排查
```bash
# 1. 检查 K8s 集群 metrics-server
kubectl get apiservice | grep metrics
kubectl get pods -n kube-system | grep metrics-server
kubectl top nodes
kubectl top pods -A

# 2. 检查 Backend Metrics API
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/nodes/metrics | python3 -m json.tool

# 3. 查看 metrics 相关日志
docker compose logs backend | grep -iE "metrics|metrics-server" | tail -20
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| metrics-server 未部署 | 在 K8s 集群部署 metrics-server |
| metrics-server Pod 异常 | `kubectl describe pod -n kube-system metrics-server-xxx`，查看日志 |
| APIService 不可用 | `kubectl get apiservice v1beta1.metrics.k8s.io -o yaml`，检查 status |
| kubelet TLS 证书问题 | metrics-server 添加 `--kubelet-insecure-tls` 参数（自签名环境） |
| metrics-server 刚启动，数据未生成 | 等待 30-60 秒后重试 |
| RBAC 权限不足 | 确保 ServiceAccount 有 `get nodes.metrics.k8s.io` / `get pods.metrics.k8s.io` 权限 |

---

## 8. 数据库连接问题

### 症状
- /ready 显示 mysql=down
- API 返回 500 "数据库连接失败"
- Backend 启动时 panic

### 排查
```bash
# 1. 测试 MySQL 连接
mysql -h <host> -P <port> -u aiops -p -e "SELECT 1, NOW(), VERSION();"

# 2. 检查 MySQL 状态
mysql -h <host> -u root -p -e "SHOW STATUS LIKE 'Threads_connected'; SHOW VARIABLES LIKE 'max_connections';"

# 3. 检查慢查询
mysql -h <host> -u aiops -p -e "SHOW FULL PROCESSLIST;" | head -20

# 4. 查看数据库错误日志
docker compose logs mysql | tail -30
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| MySQL 未运行 | 启动 MySQL: `docker compose up -d mysql` |
| 连接信息错误 | 检查 MYSQL_HOST/PORT/USER/PASSWORD |
| 连接池耗尽 | 增加连接池大小，检查是否有连接泄漏 |
| 慢查询阻塞 | 优化查询，添加索引，kill 慢查询 |
| 磁盘满 | 清理磁盘空间，检查 binlog 大小 |
| 认证插件不兼容 | MySQL 8.0 使用 `mysql_native_password` 或更新客户端 |

---

## 9. Redis 连接问题

### 症状
- /ready 显示 redis=down
- Session 失效频繁
- Rate limiting 异常

### 排查
```bash
# 1. 测试 Redis 连接
redis-cli -h <host> -p 6379 -a <password> ping
# 返回 PONG

# 2. 检查 Redis 状态
redis-cli -h <host> -p 6379 info server | grep -E "redis_version|uptime_in_seconds|connected_clients"
redis-cli -h <host> -p 6379 info memory | grep used_memory_human

# 3. 查看 Redis 日志
docker compose logs redis | tail -20
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| Redis 未运行 | 启动 Redis: `docker compose up -d redis` |
| 连接信息错误 | 检查 REDIS_ADDR/PASSWORD |
| 密码错误 | 检查 REDIS_PASSWORD，确认 Redis requirepass 配置 |
| 内存不足 | 增加 Redis 内存，设置 maxmemory-policy |
| 持久化阻塞 | 检查 RDB/AOF 配置，调整 save 策略 |

---

## 10. 前端页面空白 / 加载失败

### 症状
- 浏览器显示空白页面
- 静态资源 404
- React 渲染错误

### 排查
```bash
# 1. 检查前端构建产物
docker exec <backend-container> ls -la /app/frontend/dist/
# 或检查本地 dist 目录
ls -la frontend/dist/

# 2. 检查静态资源是否可访问
curl -s -I http://localhost:8080/ | head -5
curl -s -I http://localhost:8080/assets/index-xxx.js | head -5

# 3. 浏览器开发者工具
# - Console: 查看 JS 错误
# - Network: 检查资源加载状态
# - Application: 检查缓存/Service Worker

# 4. 重新构建前端
cd frontend && npm run build
# 重启 Backend
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| 前端未构建 | 执行 `npm run build`，确认 dist/ 目录存在 |
| 静态资源路径错误 | 检查 Backend 静态文件服务配置，确认 SPA fallback |
| 浏览器缓存 | 强制刷新 (Ctrl+Shift+R)，清除浏览器缓存 |
| JS 运行时错误 | 浏览器 Console 查看错误，修复后重新构建 |
| Backend 未托管前端 | 确认 Backend 启动时包含 frontend/dist，Single-Port 架构 |

---

## 11. 登录失败 / 401

### 症状
- 登录页面提示用户名或密码错误
- API 返回 401
- Token 很快过期

### 排查
```bash
# 1. 测试登录 API
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<password>"}' | python3 -m json.tool

# 2. 检查用户是否存在
mysql -u aiops -p aiops -e "SELECT id, username, status FROM users;"

# 3. 检查 JWT 配置
docker compose config | grep -E "JWT"

# 4. 检查用户状态
mysql -u aiops -p aiops -e "SELECT username, status, failed_login_count FROM users WHERE username='admin';"
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| 用户名或密码错误 | 确认凭据，重置密码 |
| 用户被禁用 | 启用用户: `UPDATE users SET status='active' WHERE username='admin';` |
| JWT_SECRET 不一致 | 确保所有实例使用相同的 JWT_SECRET |
| Token 过期时间太短 | 检查 JWT_EXPIRE_HOURS 配置 |
| 登录失败次数锁定 | 解锁用户: 重置 failed_login_count |
| 密码哈希算法不兼容 | 重置密码，使用当前哈希算法 |

---

## 12. 性能问题

### 症状
- API 响应慢
- AI 请求经常超时
- 页面加载慢
- CPU/内存使用率高

### 排查
```bash
# 1. 检查系统资源
docker stats
# 或
kubectl top pods -n aiops

# 2. 检查慢 API
# 日志中搜索 cost_ms > 阈值
docker compose logs backend | grep -E "cost_ms=[0-9]{4,}" | tail -20

# 3. 检查数据库慢查询
mysql -u aiops -p aiops -e "SHOW VARIABLES LIKE 'slow_query%'; SHOW VARIABLES LIKE 'long_query_time';"

# 4. 检查 AI 请求耗时
docker compose logs backend | grep -iE "ai.request.completed|ai.request.failed" | tail -10
```

### 常见原因
| 原因 | 解决方案 |
|------|---------|
| AI Provider 响应慢 | 正常现象（DeepSeek 20-30s），使用 SSE streaming 改善体验 |
| 数据库慢查询 | 优化 SQL，添加索引，检查慢查询日志 |
| 连接池过小 | 增加 MySQL/Redis 连接池大小 |
| K8s API 延迟高 | 检查 K8s API Server 负载，考虑缓存 |
| 前端 bundle 过大 | 启用代码分割 (React.lazy)，gzip 压缩 |
| 资源不足 | 增加 CPU/内存，水平扩展 Backend 实例 |

---

## 13. 日志查看指南

### Backend 日志
```bash
# Docker Compose
docker compose logs -f backend
docker compose logs backend --tail=100
docker compose logs backend | grep -i error

# Kubernetes
kubectl logs -f deployment/aiops-platform -n aiops
kubectl logs -f deployment/aiops-platform -n aiops --previous  # 上一个容器
kubectl logs -f <pod-name> -n aiops -c backend
```

### 日志字段说明
结构化日志 (slog) 包含:
- `method`: HTTP 方法
- `path`: 请求路径
- `status`: HTTP 状态码
- `cost_ms`: 请求耗时（毫秒）
- `request_id`: 请求唯一 ID
- `user_id`: 用户 ID（已认证）
- `error`: 错误信息（如有）

### AI 请求日志
- `ai: request started`: 请求开始，含 question_len, has_incident_context
- `ai: first token`: 首 token 到达（SSE），含 ttft_ms
- `ai: request completed`: 请求完成，含 total_duration_ms, provider_duration_ms, tool_call_count
- `ai: request failed`: 请求失败，含 error_type, error

---

## 14. 紧急联系

如遇到无法解决的问题:
1. 收集所有相关日志（Backend / MySQL / Redis / K8s）
2. 记录问题时间线和复现步骤
3. 记录环境信息（版本 / 配置 / 资源）
4. 提交 Issue 或联系开发团队
5. 如影响生产业务，先执行回滚

---

## 15. 常用命令速查

```bash
# 健康检查
curl -s http://localhost:8080/health | python3 -m json.tool
curl -s http://localhost:8080/ready | python3 -m json.tool

# 登录获取 Token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<password>"}' | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")

# 测试 AI
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"question":"你好"}' \
  http://localhost:8080/api/v1/ai/ask | python3 -m json.tool

# 测试 AI SSE
curl -s -N -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"question":"hi"}' \
  http://localhost:8080/api/v1/ai/ask/stream

# K8s 资源
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/nodes?cluster=<name>" | python3 -m json.tool

# Metrics
curl -s -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/nodes/metrics | python3 -m json.tool

# 查看日志
docker compose logs -f backend
kubectl logs -f deployment/aiops-platform -n aiops
```
