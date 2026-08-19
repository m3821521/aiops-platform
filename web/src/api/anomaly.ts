import request from './client'
import type { Anomaly } from '@/types'

export const anomalyApi = {
  detect: (params: {
    query: string
    start?: string
    end?: string
    step?: string
    thresholds?: Record<string, number>
  }) => request.post<any, Anomaly[]>('/api/v1/anomaly/detect', params),
}
