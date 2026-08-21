import request from './client'
import type { Incident, IncidentSignal, IncidentListFilter, PageResult } from '@/types'

export const incidentApi = {
  list: (params: IncidentListFilter & { page?: number; page_size?: number }) =>
    request.get<any, PageResult<Incident>>('/api/v1/incidents', { params }),

  get: (id: number) =>
    request.get<any, Incident>(`/api/v1/incidents/${id}`),

  acknowledge: (id: number) =>
    request.post<any, { status: string }>(`/api/v1/incidents/${id}/acknowledge`),

  resolve: (id: number) =>
    request.post<any, { status: string }>(`/api/v1/incidents/${id}/resolve`),

  close: (id: number) =>
    request.post<any, { status: string }>(`/api/v1/incidents/${id}/close`),

  signals: (id: number) =>
    request.get<any, { items: IncidentSignal[]; total: number }>(
      `/api/v1/incidents/${id}/signals`,
    ),

  timeline: (id: number) =>
    request.get<any, { items: IncidentSignal[]; total: number }>(
      `/api/v1/incidents/${id}/timeline`,
    ),

  evidence: (id: number) =>
    request.get<any, EvidenceBundle>(`/api/v1/incidents/${id}/evidence`),
}

export interface EvidenceBundle {
  incident_id: number
  cluster?: string
  namespace?: string
  service?: string
  resource_type?: string
  resource_name?: string
  time_window: { start: string; end: string; before: string }
  sources: Record<string, string>
  alerts: EvidenceAlert[]
  anomalies: EvidenceAnomaly[]
  events: EvidenceEvent[]
  metrics: EvidenceMetric[]
  logs: EvidenceLog[]
  topology: { nodes: any[]; edges: any[] }
  timeline: EvidenceTimelineItem[]
  pod_resource_state?: PodResourceState
}

export interface PodResourceState {
  namespace: string
  pod: string
  phase: string
  ready: boolean
  restart_count: number
  node_name?: string
  pod_ip?: string
  host_ip?: string
  start_time?: string
  containers: PodContainerState[]
  conditions: PodCondition[]
}

export interface PodContainerState {
  name: string
  ready: boolean
  restart_count: number
  state: string
  reason?: string
  message?: string
  exit_code?: number | null
  signal?: number | null
  started_at?: string
  finished_at?: string
  last_state?: string
  last_reason?: string
  last_exit_code?: number | null
}

export interface PodCondition {
  type: string
  status: string
  reason?: string
  message?: string
}

export interface EvidenceAlert {
  id: number
  fingerprint: string
  alertname: string
  severity: string
  service?: string
  namespace?: string
  pod?: string
  node?: string
  starts_at: string
}

export interface EvidenceAnomaly {
  id: number
  metric: string
  resource_type: string
  resource_name: string
  namespace?: string
  timestamp: string
  value: number
  baseline: number
  anomaly_score: number
  severity: string
  algorithm: string
  reason: string
}

export interface EvidenceEvent {
  type: string
  reason: string
  message: string
  resource_type: string
  resource_name: string
  namespace: string
  timestamp: string
  count: number
}

export interface EvidenceMetric {
  metric: string
  value: number
  timestamp: string
  resource: string
}

export interface EvidenceLog {
  timestamp: string
  level: string
  message: string
  pod: string
  namespace: string
}

export interface EvidenceTimelineItem {
  timestamp: string
  type: string
  description: string
  severity?: string
  resource?: string
}
