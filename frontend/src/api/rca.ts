import request from './client'
import type { RCAResult, RCAEvidence } from '@/types'

export const rcaApi = {
  // 触发 RCA 分析
  analyze(incidentId: number) {
    return request.post<any, RCAResult>(`/api/v1/incidents/${incidentId}/rca`)
  },

  // 获取最近 RCA 结果
  getLatest(incidentId: number) {
    return request.get<any, RCAResult>(`/api/v1/incidents/${incidentId}/rca`)
  },

  // 获取 RCA 历史
  getHistory(incidentId: number, limit = 20) {
    return request.get<any, any[]>(`/api/v1/incidents/${incidentId}/rca/history`, { params: { limit } })
  },

  // 重新分析
  reanalyze(incidentId: number) {
    return request.post<any, RCAResult>(`/api/v1/incidents/${incidentId}/rca/reanalyze`)
  },

  // 获取证据列表
  getEvidence(incidentId: number) {
    return request.get<any, RCAEvidence[]>(`/api/v1/incidents/${incidentId}/evidence`)
  },
}
