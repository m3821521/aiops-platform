import request from './client'
import type { AIAnalysisResult } from '@/types'

export const aiAnalysisApi = {
  // 触发 AI 分析
  analyze(incidentId: number) {
    return request.post<any, AIAnalysisResult>(`/api/v1/incidents/${incidentId}/ai-analyze`)
  },

  // 获取最近 AI 分析结果
  getLatest(incidentId: number) {
    return request.get<any, AIAnalysisResult>(`/api/v1/incidents/${incidentId}/ai-analysis`)
  },

  // 获取 AI 分析历史
  getHistory(incidentId: number, limit = 20) {
    return request.get<any, any[]>(`/api/v1/incidents/${incidentId}/ai-analysis/history`, { params: { limit } })
  },
}
