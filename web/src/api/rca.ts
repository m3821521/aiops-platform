import request from './client'
import type { RCAResult } from '@/types'

export const rcaApi = {
  analyze: (start?: string, end?: string) =>
    request.get<any, RCAResult>('/api/v1/rca/analyze', { params: { start, end } }),
}
