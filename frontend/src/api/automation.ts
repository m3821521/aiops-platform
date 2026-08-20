import request from './client'

export interface AutomationAction {
  id: number
  incident_id?: number
  user_id: number
  action_type: string
  target_type: string
  target_name: string
  cluster: string
  connection_id?: number
  namespace: string
  parameters: string
  reason: string
  risk: string
  status: string
  approved_by?: number
  approved_at?: string
  rejected_by?: number
  rejected_at?: string
  reject_reason?: string
  created_at: string
  updated_at: string
}

export interface DryRunResult {
  action_type: string
  target: string
  current_state: string
  expected_operation: string
  potential_impact: string
  safe: boolean
}

export interface ExecutionResult {
  success: boolean
  message: string
  data?: any
  error?: string
}

export interface ActionExecution {
  id: number
  action_id: number
  executor: string
  started_at: string
  finished_at?: string
  duration_ms: number
  status: string
  result_json?: string
  error?: string
  created_at: string
}

export const automationApi = {
  create: (data: {
    incident_id?: number
    action_type: string
    target_type: string
    target_name: string
    cluster: string
    connection_id?: number
    namespace?: string
    parameters?: Record<string, any>
    reason: string
    risk?: string
  }) => request.post<any, AutomationAction>('/api/v1/actions', data),

  list: (params?: {
    status?: string
    risk?: string
    action_type?: string
    incident_id?: number
    cluster?: string
    page?: number
    page_size?: number
  }) => request.get<any, { items: AutomationAction[]; total: number; page: number; page_size: number }>('/api/v1/actions', { params }),

  pendingApproval: (params?: { page?: number; page_size?: number }) =>
    request.get<any, { items: AutomationAction[]; total: number; page: number; page_size: number }>('/api/v1/actions/pending-approval', { params }),

  createFromIncident: (incidentId: number, data: {
    action_type: string
    target_type: string
    target_name: string
    cluster: string
    connection_id?: number
    namespace?: string
    parameters?: Record<string, any>
    reason: string
    risk?: string
  }) => request.post<any, AutomationAction>(`/api/v1/incidents/${incidentId}/actions`, data),

  get: (id: number) => request.get<any, AutomationAction>(`/api/v1/actions/${id}`),

  approve: (id: number) => request.post<any, AutomationAction>(`/api/v1/actions/${id}/approve`),

  reject: (id: number, reason: string) => request.post<any, AutomationAction>(`/api/v1/actions/${id}/reject`, { reason }),

  dryRun: (id: number) => request.post<any, DryRunResult>(`/api/v1/actions/${id}/dry-run`),

  execute: (id: number) => request.post<any, ExecutionResult>(`/api/v1/actions/${id}/execute`),

  cancel: (id: number) => request.post<any, AutomationAction>(`/api/v1/actions/${id}/cancel`),

  executions: (id: number) => request.get<any, ActionExecution[]>(`/api/v1/actions/${id}/executions`),

  audit: (params?: { action_id?: number; incident_id?: number; user_id?: number; page?: number; page_size?: number }) =>
    request.get<any, { items: any[]; total: number }>('/api/v1/automation/audit', { params }),

  // 旧版直接操作 API（Pod Detail / Deployment Detail 使用）
  podLogs: (pod: string, params?: { cluster?: string; namespace?: string; container?: string; tail_lines?: number; tail?: number }) =>
    request.get<any, string>(`/api/v1/automation/pods/${pod}/logs`, { params }),

  podEvents: (pod: string, params?: { cluster?: string; namespace?: string }) =>
    request.get<any, any[]>(`/api/v1/automation/pods/${pod}/events`, { params }),

  restartPod: (pod: string, data: { cluster: string; namespace: string; confirm: boolean }) =>
    request.post<any, any>(`/api/v1/automation/pods/${pod}/restart`, data),

  scaleDeployment: (name: string, data: { cluster: string; namespace: string; replicas: number; confirm: boolean }) =>
    request.post<any, any>(`/api/v1/automation/deployments/${name}/scale`, data),
}
