import request from './client'

export interface WorkflowStep {
  id: number
  workflow_id: number
  order: number
  name: string
  action_type: string
  target_type?: string
  target_name?: string
  cluster?: string
  namespace?: string
  parameters?: string
  status: string
  depends_on?: number
  max_retry?: number
  retry_count?: number
  timeout_sec?: number
  result?: string
  error?: string
  started_at?: string
  finished_at?: string
}

export interface Workflow {
  id: number
  name: string
  description?: string
  incident_id?: number
  status: string
  risk: string
  created_by: number
  approved_by?: number
  approved_at?: string
  started_at?: string
  finished_at?: string
  duration_ms?: number
  steps?: WorkflowStep[]
  created_at: string
  updated_at: string
}

export const workflowApi = {
  create: (data: { name: string; description?: string; incident_id?: number; risk?: string; steps: Partial<WorkflowStep>[] }) =>
    request.post<any, Workflow>('/api/v1/workflows', data),

  list: (params?: { status?: string; incident_id?: number; page?: number; page_size?: number }) =>
    request.get<any, { items: Workflow[]; total: number; page: number; page_size: number }>('/api/v1/workflows', { params }),

  get: (id: number) => request.get<any, Workflow>(`/api/v1/workflows/${id}`),

  submit: (id: number) => request.post<any, Workflow>(`/api/v1/workflows/${id}/submit`),

  approve: (id: number) => request.post<any, Workflow>(`/api/v1/workflows/${id}/approve`),

  execute: (id: number) => request.post<any, Workflow>(`/api/v1/workflows/${id}/execute`),

  cancel: (id: number) => request.post<any, Workflow>(`/api/v1/workflows/${id}/cancel`),
}
