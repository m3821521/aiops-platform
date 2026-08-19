import request from './client'

export interface AIAskResponse {
  answer: string
  summary?: string
  root_cause?: string
  confidence?: number
  evidence?: AIAskEvidence[]
  recommendations?: AIAskRecommendation[]
  tool_calls?: AIToolCall[]
  duration_ms?: number
}

export interface AIAskEvidence {
  source: string
  description: string
  resource?: string
}

export interface AIAskRecommendation {
  priority: string
  title: string
  risk: string
}

export interface AIToolCall {
  tool_name: string
  input: Record<string, any>
  result: {
    success: boolean
    available: boolean
    source: string
    error?: string
  }
  duration_ms: number
  timestamp: string
}

export const aiApi = {
  ask: (question: string, incidentId?: number) =>
    request.post<any, AIAskResponse>(
      '/api/v1/ai/ask',
      { question, incident_id: incidentId },
    ),

  getAudit: (params: { incident_id?: number; tool_name?: string; page?: number; page_size?: number }) =>
    request.get<any, { items: any[]; total: number }>('/api/v1/ai/audit', { params }),
}
