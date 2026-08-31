# AIOps Platform Production Smoke Test

**版本**: 1.0.0
**用途**: 部署后验证所有核心功能正常

---

## 前置条件

```bash
# 设置环境变量
export AIOPS_URL=http://localhost:8080
export ADMIN_USER=admin
export ADMIN_PASSWORD=<admin-password>
export CLUSTER_NAME=<k8s-cluster-name-in-connections>
```

---

## 1. Health Check

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 1.1 | Liveness | `curl -s $AIOPS_URL/health` | code=0, status=ok | | |
| 1.2 | Readiness | `curl -s $AIOPS_URL/ready` | code=200, mysql=up, redis=up | | |

---

## 2. Authentication

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 2.1 | Login | `curl -X POST $AIOPS_URL/api/v1/auth/login -H 'Content-Type: application/json' -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASSWORD\"}"` | 200, access_token | | |
| 2.2 | 未认证访问 | `curl -s $AIOPS_URL/api/v1/nodes` | 401 | | |
| 2.3 | Token 验证 | 使用 token 访问受保护 API | 200 | | |

```bash
# 获取 Token
export TOKEN=$(curl -s -X POST $AIOPS_URL/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASSWORD\"}" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['data']['access_token'])")

echo "Token: ${TOKEN:0:20}..."
```

---

## 3. Dashboard

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 3.1 | Dashboard 数据 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/dashboard` | 200, 真实统计数据 | | |

---

## 4. Kubernetes

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 4.1 | Nodes 列表 | `curl -s -H "Authorization: Bearer $TOKEN" "$AIOPS_URL/api/v1/nodes?cluster=$CLUSTER_NAME"` | 200, node 列表 | | |
| 4.2 | Pods 列表 | `curl -s -H "Authorization: Bearer $TOKEN" "$AIOPS_URL/api/v1/pods?cluster=$CLUSTER_NAME"` | 200, pod 列表 | | |
| 4.3 | Namespaces | `curl -s -H "Authorization: Bearer $TOKEN" "$AIOPS_URL/api/v1/namespaces?cluster=$CLUSTER_NAME"` | 200 | | |
| 4.4 | Deployments | `curl -s -H "Authorization: Bearer $TOKEN" "$AIOPS_URL/api/v1/deployments?cluster=$CLUSTER_NAME"` | 200 | | |
| 4.5 | Node 详情 | `curl -s -H "Authorization: Bearer $TOKEN" "$AIOPS_URL/api/v1/nodes/<node-name>?cluster=$CLUSTER_NAME"` | 200 | | |

---

## 5. Metrics Server

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 5.1 | Node Metrics | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/nodes/metrics` | 200, cpu_percent, memory_percent, timestamp | | |
| 5.2 | Pod Metrics | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/pods/metrics` | 200, 多个 pod metrics | | |
| 5.3 | Pod Metrics (namespace) | `curl -s -H "Authorization: Bearer $TOKEN" "$AIOPS_URL/api/v1/pods/metrics?namespace=kube-system"` | 200 | | |
| 5.4 | Timestamp 真实 | 检查返回的 timestamp 字段 | 非空，RFC3339 格式 | | |

---

## 6. Alerts & Incidents

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 6.1 | Alerts 列表 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/alerts` | 200 | | |
| 6.2 | Incidents 列表 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/incidents` | 200 | | |
| 6.3 | Incident 详情 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/incidents/<id>` | 200 | | |

---

## 7. RCA

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 7.1 | RCA 详情 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/incidents/<id>/rca` | 200, status, confidence, root_cause | | |
| 7.2 | RCA 历史 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/incidents/<id>/rca/history` | 200 | | |

---

## 8. AI Assistant

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 8.1 | 普通 AI 请求 | `curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"question":"你好，请用一句话回答"}' $AIOPS_URL/api/v1/ai/ask` | 200, answer 非空 | | |
| 8.2 | AI 配置状态 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/ai/config` | 200, api_key_configured=true | | |
| 8.3 | 对话历史 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/ai/conversations` | 200 | | |

---

## 9. AI SSE Streaming

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 9.1 | SSE 连接 | `curl -s -N -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"question":"hi"}' $AIOPS_URL/api/v1/ai/ask/stream` | event: start, token, done | | |
| 9.2 | request_id 一致 | 检查 start 和 done 的 request_id | 相同 | | |
| 9.3 | TTFT | 记录首 token 到达时间 | < 30s（正常网络） | | |
| 9.4 | Token 完整性 | 拼接所有 token 文本 | 与预期回答一致 | | |

```bash
# SSE 测试脚本
curl -s -N -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"question":"用一句话介绍你自己"}' \
  $AIOPS_URL/api/v1/ai/ask/stream 2>&1 | head -30
```

---

## 10. Connections

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 10.1 | Connections 列表 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/connections` | 200, 无隐式默认连接 | | |
| 10.2 | Kubernetes 连接 | 检查 type=kubernetes 的连接 | enabled=true, status=available | | |

---

## 11. Automation

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 11.1 | Actions 列表 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/automation/actions` | 200 | | |
| 11.2 | Workflows 列表 | `curl -s -H "Authorization: Bearer $TOKEN" $AIOPS_URL/api/v1/workflows` | 200 | | |

---

## 12. Audit Log

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 12.1 | Audit Log | `curl -s -H "Authorization: Bearer $TOKEN" "$AIOPS_URL/api/v1/audit/logs?limit=10"` | 200, 日志列表 | | |

---

## 13. Frontend UI

通过浏览器手动验证:

| # | 测试 | 预期 | 实际 | 状态 |
|---|------|------|------|------|
| 13.1 | 登录页面 | 正常显示，可登录 | | |
| 13.2 | Dashboard | 真实统计数据，无 mock | | |
| 13.3 | Kubernetes Nodes | CPU/Memory 列显示真实值 | | |
| 13.4 | Kubernetes Pods | CPU/Memory 列显示真实值 | | |
| 13.5 | NodeDetail | 资源监控显示真实 metrics | | |
| 13.6 | PodDetail | 资源监控显示真实 metrics | | |
| 13.7 | Alerts | 告警列表正常 | | |
| 13.8 | Incidents | 事件列表正常 | | |
| 13.9 | RCA | 根因分析显示 | | |
| 13.10 | AI Assistant | 正常对话，SSE 流式输出 | | |
| 13.11 | DataTrust 指示器 | ● Live / Stale / Error 状态正确 | | |
| 13.12 | Connections | 外部连接管理正常 | | |
| 13.13 | Audit Log | 审计日志正常 | | |

---

## 14. Error Handling

| # | 测试 | 命令 | 预期 | 实际 | 状态 |
|---|------|------|------|------|------|
| 14.1 | AI 未配置 | 移除 AI_API_KEY 后请求 | 503, "AI 服务不可用" | | |
| 14.2 | K8s 集群不存在 | `?cluster=nonexistent` | 503, "集群不存在" | | |
| 14.3 | Metrics Server 不可用 | scale metrics-server to 0 | 503, 不返回 0% | | |
| 14.4 | 无效 Token | 使用错误 token | 401 | | |
| 14.5 | AI SSE Client Cancel | curl --max-time 3 | 连接关闭，Backend 停止 | | |

---

## 15. Performance Baseline

| # | 测试 | 预期 | 实际 | 状态 |
|---|------|------|------|------|
| 15.1 | /health 响应时间 | < 100ms | | |
| 15.2 | Login 响应时间 | < 500ms | | |
| 15.3 | Nodes 列表响应时间 | < 2s | | |
| 15.4 | AI 普通请求 | < 60s | | |
| 15.5 | AI SSE TTFT | < 30s | | |
| 15.6 | Metrics API 响应时间 | < 5s | | |

---

## 16. 最终确认

部署完成后，所有测试项必须标记为 PASS。

如有 FAIL 项:
1. 记录失败详情
2. 排查根因
3. 修复或回滚
4. 重新执行冒烟测试

**全部 PASS 后，部署才算成功。**
