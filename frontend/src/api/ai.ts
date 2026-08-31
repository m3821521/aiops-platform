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

  // AI ask streaming：使用 fetch + ReadableStream 实时接收 SSE token
  // 不使用 Axios，因为 Axios 不支持流式响应
  // callback: 收到 token 时调用，返回 false 可中止流
  askStream: async (
    question: string,
    incidentId?: number,
    conversationId?: number,
    callback?: (event: { type: string; data: any }) => boolean | void,
    signal?: AbortSignal,
  ) => {
    const token = localStorage.getItem('token') || sessionStorage.getItem('token')
    const response = await fetch('/api/v1/ai/ask/stream', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ question, incident_id: incidentId, conversation_id: conversationId }),
      signal,
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`)
    }

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error('Response body is not readable')
    }

    const decoder = new TextDecoder()
    let buffer = ''

    // 解析单个完整 SSE 事件并调用 callback
    // 返回 false 表示调用方要求中止流
    const dispatchEvent = (eventType: string, dataStr: string): boolean => {
      let parsedData: any = dataStr
      try {
        parsedData = JSON.parse(dataStr)
      } catch {
        // 非 JSON 数据，保持原样
      }
      const shouldContinue = callback?.({ type: eventType, data: parsedData })
      return shouldContinue !== false
    }

    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })

        // 解析 SSE 事件：event: xxx\ndata: xxx\n\n
        // 统一处理 \r\n、\r、\n 三种换行符
        const lines = buffer.split(/\r\n|\r|\n/)
        // 最后一行可能不完整，保留到 buffer
        buffer = lines.pop() || ''

        let currentEvent = ''
        let currentDataLines: string[] = []

        for (const rawLine of lines) {
          const line = rawLine.trimEnd()
          if (line.startsWith('event:')) {
            currentEvent = line.slice(6).trim()
          } else if (line.startsWith('data:')) {
            currentDataLines.push(line.slice(5).trim())
          } else if (line === '' && currentEvent) {
            // 空行表示事件结束，多行 data 用 \n 连接（SSE 标准）
            const dataStr = currentDataLines.join('\n')
            if (!dispatchEvent(currentEvent, dataStr)) {
              reader.cancel()
              return
            }
            currentEvent = ''
            currentDataLines = []
          }
          // 其他行（注释、空行等）忽略
        }
      }

      // EOF flush：连接结束时，如果 buffer 中仍有完整事件，必须处理
      if (buffer.trim() !== '') {
        const lines = buffer.split(/\r\n|\r|\n/)
        let currentEvent = ''
        let currentDataLines: string[] = []
        for (const rawLine of lines) {
          const line = rawLine.trimEnd()
          if (line.startsWith('event:')) {
            currentEvent = line.slice(6).trim()
          } else if (line.startsWith('data:')) {
            currentDataLines.push(line.slice(5).trim())
          } else if (line === '' && currentEvent) {
            const dataStr = currentDataLines.join('\n')
            dispatchEvent(currentEvent, dataStr)
            currentEvent = ''
            currentDataLines = []
          }
        }
        // 处理最后一个没有尾随空行的事件
        if (currentEvent) {
          const dataStr = currentDataLines.join('\n')
          dispatchEvent(currentEvent, dataStr)
        }
      }
    } finally {
      reader.releaseLock()
    }
  },

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
