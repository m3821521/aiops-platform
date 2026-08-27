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
  conversation_id?: number
}

export interface AIAskEvidence {
  source: string
  description: string
  resource?: string
}

export interface AIAskRecommendation {
  priority: string
  title: string
  description?: string
  reason?: string
  risk: string
  action_type?: string
  target?: string
  namespace?: string
  parameters?: Record<string, any>
  incident_id?: number  // 关联的 Incident ID，用于 investigate 类型跳转到 Incident Detail
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

export interface AIConversation {
  id: number
  user_id: number
  title: string
  incident_id?: number
  message_count: number
  created_at: string
  updated_at: string
}

export interface AIConversationMessage {
  id: number
  conversation_id: number
  role: 'user' | 'assistant'
  content: string
  summary?: string
  root_cause?: string
  confidence?: number
  duration_ms?: number
  created_at: string
}

export const aiApi = {
  // AI ask 使用专用 timeout 28s（略大于 Backend 25s overall timeout，留 3s 网络延迟余量）
  // 不修改全局 Axios 30s timeout，避免影响其他 API
  ask: (question: string, incidentId?: number, conversationId?: number) =>
    request.post<any, AIAskResponse>(
      '/api/v1/ai/ask',
      { question, incident_id: incidentId, conversation_id: conversationId },
      { timeout: 28000 },
    ),

  getAudit: (params: { incident_id?: number; tool_name?: string; page?: number; page_size?: number }) =>
    request.get<any, { items: any[]; total: number }>('/api/v1/ai/audit', { params }),

  listConversations: (page = 1, pageSize = 20) =>
    request.get<any, { items: AIConversation[]; total: number }>('/api/v1/ai/conversations', {
      params: { page, page_size: pageSize },
    }),

  getConversation: (id: number) =>
    request.get<any, { conversation: AIConversation; messages: AIConversationMessage[] }>(
      `/api/v1/ai/conversations/${id}`,
    ),

  deleteConversation: (id: number) =>
    request.delete<any, { code: string; message: string }>(`/api/v1/ai/conversations/${id}`),

  getConfig: () =>
    request.get<any, AIConfigResponse>('/api/v1/ai/config'),

  updateConfig: (data: AIConfigUpdateRequest) =>
    request.post<any, AIConfigResponse>('/api/v1/ai/config', data),
}

export interface AIConfigResponse {
  provider: string
  base_url: string
  model: string
  enabled: boolean
  api_key_set: boolean
  api_key_masked?: string
}

export interface AIConfigUpdateRequest {
  provider?: string
  base_url?: string
  api_key?: string
  model?: string
  enabled?: boolean
}
