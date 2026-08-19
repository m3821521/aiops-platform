// 通用响应结构
export interface ApiResponse<T = any> {
  code: number
  message: string
  data: T
}

// 分页响应
export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

// 用户
export interface User {
  id: number
  username: string
  email: string
  full_name: string
  status: number
  roles: Role[]
  created_at: string
  updated_at: string
}

// 角色
export interface Role {
  id: number
  name: string
  description: string
  permissions: Permission[]
}

// 权限
export interface Permission {
  id: number
  name: string
  code: string
  resource: string
  action: string
}

// 集群
export interface Cluster {
  name: string
  endpoint: string
  version?: string
  status?: string
  node_count?: number
  pod_count?: number
}

// Kubernetes 资源通用
export interface K8sResource {
  name: string
  namespace?: string
  status?: string
  age?: string
  labels?: Record<string, string>
}

// Node
export interface Node extends K8sResource {
  status: string
  cpu_usage?: number
  memory_usage?: number
  pod_count?: number
  version?: string
  internal_ip?: string
}

// Pod
export interface Pod extends K8sResource {
  namespace: string
  node?: string
  status: string
  ip?: string
  restart_count?: number
  cpu_usage?: number
  memory_usage?: number
  containers?: Container[]
}

export interface Container {
  name: string
  image: string
  ready: boolean
  restart_count: number
}

// Deployment
export interface Deployment extends K8sResource {
  namespace: string
  replicas: number
  ready_replicas: number
  available_replicas: number
  updated_replicas: number
}

// Service
export interface Service extends K8sResource {
  namespace: string
  type: string
  cluster_ip: string
  ports: { port: number; protocol: string; target_port: string }[]
}

// 告警
export interface Alert {
  id: number
  fingerprint: string
  alertname: string
  severity: 'critical' | 'warning' | 'info'
  status: 'firing' | 'acknowledged' | 'resolved'
  instance?: string
  pod?: string
  namespace?: string
  service?: string
  node?: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
  starts_at: string
  ends_at?: string
  created_at: string
  updated_at: string
}

// 告警聚合
export interface AlertGroup {
  key: string
  service?: string
  namespace?: string
  node?: string
  count: number
  severity: string
  alertnames: string[]
  starts_at: string
  ends_at?: string
}

// 异常检测
export interface Anomaly {
  metric: string
  timestamp: string
  anomaly_score: number
  severity: 'critical' | 'warning' | 'info'
  reason: string
  value?: number
  baseline?: number
}

// RCA 结果
export interface RCAResult {
  root_cause: string
  confidence: number
  affected_services: string[]
  evidence: { description: string; metric?: string; value?: string }[]
  timeline: { time: string; event: string; type: string }[]
}

// 日志
export interface LogEntry {
  timestamp: string
  level: string
  namespace?: string
  pod?: string
  container?: string
  message: string
  trace_id?: string
  request_id?: string
  raw?: Record<string, any>
}

// AI 对话
export interface AIMessage {
  role: 'user' | 'assistant' | 'system'
  content: string
  timestamp?: string
  tools?: { name: string; status: 'running' | 'done' | 'error'; result?: string }[]
}

// Jenkins
export interface JenkinsJob {
  name: string
  url: string
  color: string
  last_build?: { number: number; result: string; timestamp: number; duration: number }
}

export interface JenkinsBuild {
  number: number
  result: string
  timestamp: number
  duration: number
  url: string
}

// ArgoCD
export interface ArgoApp {
  name: string
  project: string
  health_status: string
  sync_status: string
  revision: string
  repo: string
  namespace: string
}

// 审计日志
export interface AuditLog {
  id: number
  username: string
  action: string
  resource: string
  resource_id?: string
  request?: string
  result: string
  ip: string
  created_at: string
}
