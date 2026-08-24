// 通用响应结构
export interface ApiResponse<T = any> {
  code: number
  message: string
  data: T
  request_id?: string
}

// 分页响应
export interface PageResult<T> {
  items: T[]
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
  status: string // active / disabled
  roles: Role[]
  last_login_at?: string
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
  description?: string
  auth_type: 'kubeconfig' | 'serviceaccount' | 'incluster'
  enabled: boolean
  api_server?: string
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
  version?: string
  internal_ip?: string
  age?: string
  creation_timestamp?: string
  os?: string
  kernel?: string
  container_runtime?: string
  pod_count?: number
  cpu_usage?: number
  memory_usage?: number
}

export interface NodeCondition {
  type: string
  status: string
  reason?: string
  message?: string
}

export interface NodeTaint {
  key: string
  value?: string
  effect: string
}

export interface NodeDetail extends Node {
  conditions?: NodeCondition[]
  taints?: NodeTaint[]
}

// Container
export interface Container {
  name: string
  image: string
  ready: boolean
  state: string
  restart_count: number
}

// Pod
export interface Pod extends K8sResource {
  namespace: string
  node?: string
  status: string
  ip?: string
  age?: string
  creation_timestamp?: string
  restart_count?: number
  cpu_usage?: number
  memory_usage?: number
  containers?: Container[]
}

export interface PodDetail extends Pod {
  yaml?: string
}

// Deployment
export interface Deployment extends K8sResource {
  namespace: string
  ready?: string
  replicas: number
  available?: number
  updated?: number
  strategy?: string
  images?: string[]
  age?: string
  creation_timestamp?: string
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
  // 关联的 Incident ID 列表（通过 incident_signals 表关联，一个 Alert 可能关联多个 Incident）
  incident_ids?: number[]
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
  // 组内告警详情（后端返回，用于关联 Incident 查询）
  alerts?: Alert[]
}

// 告警降噪结果
export interface NoiseResult {
  groups: AlertGroup[]
  total_before: number
  total_after: number
  is_storm: boolean
  storm_reason?: string
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

// Incident 事件
export type IncidentStatus = 'open' | 'acknowledged' | 'resolved' | 'closed'
export type IncidentSeverity = 'critical' | 'warning' | 'info'
export type SignalType = 'alert' | 'anomaly' | 'log' | 'k8s_event' | 'metric'
export type ResourceType = 'pod' | 'deployment' | 'service' | 'node' | 'namespace' | 'cluster'

export interface IncidentSignal {
  id: number
  incident_id: number
  signal_type: SignalType
  signal_id: string
  title: string
  severity: IncidentSeverity
  cluster?: string
  namespace?: string
  service?: string
  resource_type?: string
  resource_name?: string
  timestamp: string
  resolved: boolean
  resolved_at?: string
  metadata?: Record<string, any>
  created_at: string
  updated_at: string
}

export interface Incident {
  id: number
  title: string
  severity: IncidentSeverity
  status: IncidentStatus
  cluster?: string
  namespace?: string
  service?: string
  resource_type?: string
  resource_name?: string
  root_cause?: string
  confidence?: number
  impact?: string
  summary?: string
  signal_count: number
  start_time: string
  end_time?: string
  duration?: string
  created_at: string
  updated_at: string
  signals?: IncidentSignal[]
}

export interface IncidentListFilter {
  status?: string
  severity?: string
  namespace?: string
  service?: string
  cluster?: string
  keyword?: string
  start_time?: string
  end_time?: string
}

// Anomaly 异常检测记录
export type AnomalyStatus = 'detected' | 'active' | 'resolved'
export type AnomalyAlgorithm = 'static_threshold' | 'moving_average' | 'ewma' | 'z_score'

export interface AnomalyRecord {
  id: number
  metric: string
  resource_type?: string
  resource_name?: string
  namespace?: string
  cluster?: string
  timestamp: string
  value: number
  baseline?: number
  expected_min?: number
  expected_max?: number
  anomaly_score: number
  severity: IncidentSeverity
  algorithm: AnomalyAlgorithm
  reason?: string
  status: AnomalyStatus
  incident_id?: number
  resolved_at?: string
  created_at: string
  updated_at: string
}

export interface AnomalyListFilter {
  cluster?: string
  namespace?: string
  resource_type?: string
  resource_name?: string
  severity?: string
  algorithm?: string
  status?: string
  metric?: string
  start_time?: string
  end_time?: string
}

// Topology 拓扑
export type TopologyNodeType = 'node' | 'pod' | 'deployment' | 'service' | 'ingress'
export type TopologyRelation = 'owns' | 'runs_on' | 'selects' | 'routes_to'
export type TopologyNodeStatus = 'healthy' | 'warning' | 'critical' | 'unknown'

export interface TopologyNode {
  id: string
  type: TopologyNodeType
  name: string
  namespace?: string
  cluster: string
  status: TopologyNodeStatus
  labels?: Record<string, string>
  metadata?: Record<string, any>
  incident_ids?: number[]
  alert_count?: number
  anomaly_count?: number
}

export interface TopologyEdge {
  id: string
  source: string
  target: string
  relation: TopologyRelation
  cluster?: string
}

export interface TopologyGraph {
  cluster: string
  nodes: TopologyNode[]
  edges: TopologyEdge[]
}

export interface TopologyDependency {
  node: TopologyNode
  upstream: TopologyNode[]
  downstream: TopologyNode[]
  parents: TopologyNode[]
  children: TopologyNode[]
}

export interface TopologyImpact {
  node: TopologyNode
  affected_nodes: TopologyNode[]
  edge_count: number
}

// RCA 根因分析
export type RCAStatus = 'analyzing' | 'completed' | 'insufficient_evidence' | 'failed'
export type EvidenceType = 'alert' | 'anomaly' | 'metric' | 'log' | 'event' | 'topology'

export interface RCAEvidence {
  id: string
  type: EvidenceType
  source: string
  timestamp: string
  resource_type?: string
  resource_name?: string
  namespace?: string
  metric?: string
  value?: number
  expected?: string
  severity?: string
  description: string
  score: number
  related_signal?: string
}

export interface RCACandidate {
  resource_type: string
  resource_name: string
  namespace?: string
  root_cause: string
  score: number
  confidence: number
  evidence: RCAEvidence[]
  impact?: string[]
  explanation: string
}

export interface RCATimelineItem {
  timestamp: string
  type: string
  description: string
  severity?: string
  resource?: string
}

export interface RCAResult {
  incident_id: number
  status: RCAStatus
  root_cause: string
  confidence: number
  candidates: RCACandidate[]
  evidence: RCAEvidence[]
  impact: string[]
  timeline: RCATimelineItem[]
  explanation: string
  generated_at: string
}

// AI 分析结果
export interface AIAnalysisResult {
  summary: string
  root_cause_explanation: string
  confidence: number
  evidence: AIEvidence[]
  impact: AIImpactItem[]
  recommendations: AIRecommendation[]
  risks: AIRisk[]
  next_actions: AIAction[]
  data_sources: AIDataSourceStatus
  generated_at: string
  model?: string
}

export interface AIEvidence {
  id?: string
  type: string
  source: string
  resource?: string
  timestamp?: string
  description: string
  importance: string
}

export interface AIImpactItem {
  resource_type: string
  resource_name: string
  namespace?: string
  impact_level: string
}

export interface AIRecommendation {
  priority: string
  title: string
  description: string
  reason: string
  risk: string
  action_type: string
}

export interface AIRisk {
  level: string
  description: string
}

export interface AIAction {
  order: number
  title: string
  description: string
  reason: string
}

export interface AIDataSourceStatus {
  alerts_available: boolean
  anomalies_available: boolean
  metrics_available: boolean
  logs_available: boolean
  events_available: boolean
  topology_available: boolean
  rca_available: boolean
}
