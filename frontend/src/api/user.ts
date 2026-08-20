import request from './client'
import type { User, Role, AuditLog, PageResult } from '@/types'

export const userApi = {
  list: (params: { page?: number; page_size?: number }) =>
    request.get<any, PageResult<User>>('/api/v1/users', { params }),
  create: (data: { username: string; password: string; email?: string; full_name?: string; role_ids?: number[] }) =>
    request.post<any, User>('/api/v1/users', data),
  update: (userId: number, data: { full_name?: string; email?: string; status?: string }) =>
    request.put<any, User>(`/api/v1/users/${userId}`, data),
  updateStatus: (userId: number, status: 'active' | 'disabled') =>
    request.put<any, { id: number; status: string; message: string }>(`/api/v1/users/${userId}/status`, { status }),
  resetPassword: (userId: number, password: string) =>
    request.put<any, { id: number; message: string }>(`/api/v1/users/${userId}/password`, { password }),
  assignRoles: (userId: number, roleIds: number[]) =>
    request.put<any, User>(`/api/v1/users/${userId}/roles`, { role_ids: roleIds }),
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
