import request from './client'

export const aiApi = {
  ask: (question: string, service?: string, duration?: string) =>
    request.post<any, { answer: string; evidence?: any[]; suggestions?: string[] }>(
      '/api/v1/ai/ask',
      { question, service, duration },
    ),
}
