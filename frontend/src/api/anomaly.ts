import request from './client'
import type { AnomalyRecord, AnomalyListFilter, PageResult } from '@/types'

export const anomalyApi = {
  list(params: AnomalyListFilter & { page?: number; page_size?: number }) {
    return request.get<any, PageResult<AnomalyRecord>>('/api/v1/anomaly', { params })
  },

  get(id: number) {
    return request.get<any, AnomalyRecord>(`/api/v1/anomaly/${id}`)
  },

  activeCount() {
    return request.get<any, { count: number }>('/api/v1/anomaly/active/count')
  },

  detect(data: {
    query: string
    start: string
    end: string
    step?: string
    thresholds: {
      upper_warning?: number
      upper_critical?: number
      lower_warning?: number
      lower_critical?: number
    }
  }, persist = false) {
    return request.post<any, { metric: string; points_checked: number; anomalies?: any[]; saved_count?: number }>(
      `/api/v1/anomaly/detect${persist ? '?persist=true' : ''}`,
      data
    )
  },
}
