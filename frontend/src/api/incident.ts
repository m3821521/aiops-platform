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
}
