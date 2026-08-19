import request from './client'
import type { User, Role, AuditLog, PageResult } from '@/types'

export const userApi = {
  list: (params: { page?: number; page_size?: number }) =>
    request.get<any, PageResult<User>>('/api/v1/users', { params }),
  create: (data: { username: string; password: string; email?: string; full_name?: string }) =>
    request.post('/api/v1/users', data),
  roles: () => request.get<any, Role[]>('/api/v1/roles'),
  auditLogs: (params: {
    page?: number
    page_size?: number
    username?: string
    action?: string
    resource?: string
    result?: string
  }) => request.get<any, PageResult<AuditLog>>('/api/v1/audit-logs', { params }),
}
