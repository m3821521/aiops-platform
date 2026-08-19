import request from './client'
import type { LogEntry } from '@/types'

export const logsApi = {
  search: (params: {
    keyword?: string
    namespace?: string
    pod?: string
    container?: string
    level?: string
    trace_id?: string
    start?: string
    end?: string
  }) => request.get<any, { list: LogEntry[]; total: number }>('/api/v1/logs/search', { params }),
  analyze: (params: {
    namespace?: string
    pod?: string
    level?: string
    start?: string
    end?: string
  }) => request.get('/api/v1/logs/analyze', { params }),
}
