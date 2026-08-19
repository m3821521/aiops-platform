import request from './client'
import type { Alert, AlertGroup, PageResult } from '@/types'

export const alertsApi = {
  list: (params: {
    page?: number
    page_size?: number
    status?: string
    severity?: string
    service?: string
  }) => request.get<any, PageResult<Alert>>('/api/v1/alerts', { params }),
  get: (id: number) => request.get<any, Alert>(`/api/v1/alerts/${id}`),
  acknowledge: (id: number) => request.post(`/api/v1/alerts/${id}/acknowledge`),
  resolve: (id: number) => request.post(`/api/v1/alerts/${id}/resolve`),
  aggregate: (dimension?: string) =>
    request.get<any, AlertGroup[]>('/api/v1/alerts/aggregate', { params: { dimension } }),
  noise: (window?: string) =>
    request.get('/api/v1/alerts/noise', { params: { window } }),
}
