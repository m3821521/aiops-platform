# AIOps Platform Production Rollback Procedure

**版本**: 1.0.0
**用途**: 生产环境部署失败或出现严重问题时的回滚操作指南

---

## 1. 回滚触发条件

立即触发回滚的情况:
- Backend 无法启动 / CrashLoopBackOff
- /health 或 /ready 持续失败 > 2 分钟
- 核心 API 5xx 率 > 20% 持续 5 分钟
- 数据库连接失败 / 数据损坏
- AI 功能完全不可用且影响核心业务
- 安全漏洞被利用
- 数据丢失 / 数据不一致
- 用户无法登录

评估后回滚的情况:
- 非核心功能异常（如某个可选集成）
- 性能下降但仍可接受
- P3 级 UI 问题

---

## 2. 回滚前检查

### 2.1 确认当前版本
```bash
# Docker Compose
docker compose images backend
docker inspect <container> | grep -E "Image|Created"

# Kubernetes
kubectl get deployment aiops-platform -n aiops -o jsonpath='{.spec.template.spec.containers[0].image}'
kubectl rollout history deployment/aiops-platform -n aiops
```

### 2.2 确认上一个稳定版本
```bash
# Docker 镜像历史
docker images | grep aiops-platform

# Kubernetes rollout 历史
kubectl rollout history deployment/aiops-platform -n aiops
```

### 2.3 备份当前状态
```bash
# 数据库备份（回滚前必须）
mysqldump -h <host> -u aiops -p aiops > /backup/pre_rollback_$(date +%Y%m%d_%H%M%S).sql

# Kubernetes 资源导出
kubectl get deployment aiops-platform -n aiops -o yaml > /backup/aiops_deployment_$(date +%Y%m%d).yaml
```

---

## 3. Docker Compose 回滚

### 3.1 回滚到上一个镜像
```bash
# 1. 停止当前服务
docker compose stop backend

# 2. 修改 docker-compose.yaml 中的镜像 tag 为上一个稳定版本
#    image: aiops-platform:<previous-stable-tag>

# 3. 拉取上一个版本镜像（如果本地没有）
docker pull aiops-platform:<previous-stable-tag>

# 4. 启动上一个版本
docker compose up -d backend

# 5. 验证
docker compose ps
docker compose logs -f backend
curl -s http://localhost:8080/health
```

### 3.2 快速回滚（使用 docker tag）
```bash
# 如果上一个版本镜像仍在本地
docker tag aiops-platform:<previous-tag> aiops-platform:latest
docker compose up -d --force-recreate backend
```

---

## 4. Kubernetes 回滚

### 4.1 使用 kubectl rollout undo
```bash
# 1. 查看 rollout 历史
kubectl rollout history deployment/aiops-platform -n aiops

# 2. 回滚到上一个版本
kubectl rollout undo deployment/aiops-platform -n aiops

# 3. 查看回滚状态
kubectl rollout status deployment/aiops-platform -n aiops

# 4. 验证
kubectl get pods -n aiops -l app=aiops-platform
kubectl describe deployment aiops-platform -n aiops | grep Image
```

### 4.2 回滚到指定版本
```bash
# 回滚到 revision 2
kubectl rollout undo deployment/aiops-platform -n aiops --to-revision=2
```

### 4.3 手动回滚（修改 image）
```bash
kubectl set image deployment/aiops-platform backend=aiops-platform:<previous-tag> -n aiops
kubectl rollout status deployment/aiops-platform -n aiops
```

---

## 5. 数据库回滚考虑

### 5.1 重要原则
**如果新版本包含数据库 schema 变更（migration），不能简单通过应用镜像回滚解决。**

### 5.2 评估数据库变更
```bash
# 检查新版本是否有新的 migration 文件
ls -la backend/migrations/
git log --oneline backend/migrations/

# 检查当前数据库 migration 版本
mysql -u aiops -p aiops -e "SELECT * FROM schema_migrations ORDER BY version DESC LIMIT 5;"
```

### 5.3 数据库回滚场景

**场景 A: 无 schema 变更（最常见）**
- 仅回滚应用镜像即可
- 数据库无需操作

**场景 B: 新增表/列（向后兼容）**
- 回滚应用镜像
- 新增的表/列保留（不影响旧版本）
- 无需数据库回滚

**场景 C: 删除表/列 / 数据迁移（不兼容）**
- **必须从备份恢复数据库**
- 回滚应用镜像到上一个版本
- 恢复数据库备份:
  ```bash
  mysql -u aiops -p aiops < /backup/pre_upgrade_<timestamp>.sql
  ```

### 5.4 数据库回滚步骤
```bash
# 1. 停止应用（防止写入）
kubectl scale deployment aiops-platform -n aiops --replicas=0

# 2. 恢复数据库备份
mysql -h <host> -u root -p aiops < /backup/<backup-file>.sql

# 3. 回滚应用镜像
kubectl rollout undo deployment/aiops-platform -n aiops
kubectl scale deployment aiops-platform -n aiops --replicas=2

# 4. 验证
kubectl rollout status deployment/aiops-platform -n aiops
curl -s http://<endpoint>/health
```

---

## 6. 配置回滚

### 6.1 环境变量回滚
```bash
# Kubernetes ConfigMap / Secret
kubectl rollout undo configmap aiops-config -n aiops  # 不支持，需手动恢复
kubectl get configmap aiops-config -n aiops -o yaml > /backup/current_config.yaml
# 手动修改为上一个版本配置后 apply
kubectl apply -f /backup/previous_config.yaml

# 重启 Pod 使配置生效
kubectl rollout restart deployment/aiops-platform -n aiops
```

### 6.2 External Connections 回滚
如果新版本修改了 Connections 配置:
```bash
# 通过 UI 或 API 恢复上一个配置
# 或从数据库备份恢复 connections 表
mysql -u aiops -p aiops -e "SELECT * FROM connections;"
```

---

## 7. 回滚后验证

### 7.1 基础验证
```bash
# Health
curl -s http://<endpoint>/health | python3 -m json.tool

# Ready
curl -s http://<endpoint>/ready | python3 -m json.tool

# Login
curl -s -X POST http://<endpoint>/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<password>"}' | python3 -m json.tool
```

### 7.2 核心功能验证
| 功能 | 验证命令 | 预期 |
|------|---------|------|
| Kubernetes Nodes | `GET /api/v1/nodes?cluster=<name>` | 200 |
| Metrics | `GET /api/v1/nodes/metrics` | 200, 真实数据 |
| AI | `POST /api/v1/ai/ask` | 200, answer 非空 |
| AI SSE | `POST /api/v1/ai/ask/stream` | start/token/done |
| Alerts | `GET /api/v1/alerts` | 200 |
| Incidents | `GET /api/v1/incidents` | 200 |
| RCA | `GET /api/v1/incidents/<id>/rca` | 200 |

### 7.3 数据一致性验证
```bash
# 检查关键表数据
mysql -u aiops -p aiops -e "
SELECT 'users' as tbl, COUNT(*) as cnt FROM users
UNION ALL SELECT 'incidents', COUNT(*) FROM incidents
UNION ALL SELECT 'connections', COUNT(*) FROM connections
UNION ALL SELECT 'audit_logs', COUNT(*) FROM audit_logs;
"
```

### 7.4 前端验证
- 浏览器访问，确认页面正常加载
- 登录功能正常
- 核心页面数据正常显示
- AI 对话功能正常
- DataTrust 指示器状态正确

---

## 8. 回滚后监控

回滚后至少监控 30 分钟:

```bash
# 持续监控 Pod 状态
kubectl get pods -n aiops -w

# 监控日志
kubectl logs -f deployment/aiops-platform -n aiops

# 监控 API 错误率
# 通过 Prometheus / 日志分析
```

### 关键指标
- Pod 重启次数 = 0
- 5xx 错误率 < 1%
- AI 请求成功率 > 95%
- 数据库连接正常
- Redis 连接正常

---

## 9. 回滚失败处理

如果回滚后仍然异常:

### 9.1 完全停止服务
```bash
# Kubernetes
kubectl scale deployment aiops-platform -n aiops --replicas=0

# Docker Compose
docker compose down
```

### 9.2 从完整备份恢复
```bash
# 1. 恢复数据库
mysql -u root -p < /backup/full_backup_<date>.sql

# 2. 恢复配置
kubectl apply -f /backup/config_<date>.yaml

# 3. 使用已知稳定版本启动
kubectl set image deployment/aiops-platform backend=aiops-platform:<known-stable-tag> -n aiops
kubectl scale deployment aiops-platform -n aiops --replicas=2
```

### 9.3 联系支持
- 收集所有日志
- 记录时间线
- 记录所有执行的命令
- 提交 Issue / 联系开发团队

---

## 10. 回滚时间线模板

```
T+0:   发现严重问题，决定回滚
T+1:   确认当前版本和上一个稳定版本
T+2:   备份当前数据库和配置
T+3:   执行回滚（kubectl rollout undo / docker tag）
T+5:   验证 health / ready
T+7:   验证核心 API（Login, K8s, AI, Metrics）
T+10:  验证前端 UI
T+15:  监控关键指标
T+30:  确认回滚成功，问题记录
```

**目标: 从决定回滚到服务恢复正常 < 15 分钟**

---

## 11. 回滚后必须做的事

1. **记录事件**: 记录问题原因、影响范围、回滚步骤、恢复时间
2. **根因分析**: 分析新版本为什么失败
3. **修复问题**: 在开发/测试环境修复后重新验证
4. **更新文档**: 更新部署文档、回滚文档、故障排查文档
5. **预防措施**: 增加测试覆盖、增加监控告警、改进发布流程
6. **复盘会议**: 团队复盘，总结经验教训

---

## 12. 回滚检查清单

回滚前:
- [ ] 确认问题严重程度需要回滚
- [ ] 确认上一个稳定版本 tag
- [ ] 备份当前数据库
- [ ] 备份当前配置
- [ ] 通知相关团队

回滚中:
- [ ] 执行回滚命令
- [ ] 监控回滚进度
- [ ] 确认 Pod/容器正常启动

回滚后:
- [ ] /health 返回 200
- [ ] /ready 返回 200 (mysql=up, redis=up)
- [ ] Login 正常
- [ ] Kubernetes API 正常
- [ ] Metrics API 正常
- [ ] AI 功能正常
- [ ] AI SSE 正常
- [ ] 前端页面正常
- [ ] 数据一致性验证通过
- [ ] 监控指标正常（30 分钟）
- [ ] 记录事件和时间线
