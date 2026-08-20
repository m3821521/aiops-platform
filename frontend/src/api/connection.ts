import request from './client'

// Connection 类型定义
export interface Connection {
  id: number
  name: string
  type: string
  environment: string
  endpoint: string
  config?: Record<string, any>
  credential_id?: number
  credential_type?: string
  enabled: boolean
  status?: string
  last_check_at?: string
  last_error?: string
  description?: string
  is_system_default?: boolean
  created_at?: string
  updated_at?: string
}

export interface ConnectionView extends Connection {
  masked_config?: Record<string, string>
}

export interface Credential {
  id: number
  name: string
  type: string
  description?: string
  masked_data?: Record<string, string>
  created_at?: string
  updated_at?: string
}

export interface TestConnectionResult {
  status: string
  latency_ms: number
  error_code?: string
  error_message?: string
  checked_at: string
}

export interface PageResult<T> {
  items: T[]
  total: number
}

// Connection API
export const connectionApi = {
  // 列表
  list: (params: { page?: number; page_size?: number; type?: string; environment?: string; enabled?: boolean }) =>
    request.get<any, PageResult<ConnectionView>>('/api/v1/connections', { params }),

  // 详情
  get: (id: number) =>
    request.get<any, ConnectionView>(`/api/v1/connections/${id}`),

  // 创建
  create: (data: Partial<Connection>) =>
    request.post<any, ConnectionView>('/api/v1/connections', data),

  // 更新
  update: (id: number, data: Partial<Connection>) =>
    request.put<any, ConnectionView>(`/api/v1/connections/${id}`, data),

  // 删除
  delete: (id: number) =>
    request.delete<any, { success: boolean }>(`/api/v1/connections/${id}`),

  // 启用
  enable: (id: number) =>
    request.post<any, { success: boolean }>(`/api/v1/connections/${id}/enable`),

  // 禁用
  disable: (id: number) =>
    request.post<any, { success: boolean }>(`/api/v1/connections/${id}/disable`),

  // 测试连接
  test: (id: number) =>
    request.post<any, TestConnectionResult>(`/api/v1/connections/${id}/test`),

  // 支持的类型
  types: () =>
    request.get<any, { types: Array<{ type: string; registered: boolean }> }>('/api/v1/connections/types'),
}

// Credential API
export const credentialApi = {
  // 列表
  list: (params: { page?: number; page_size?: number }) =>
    request.get<any, PageResult<Credential>>('/api/v1/credentials', { params }),

  // 详情
  get: (id: number) =>
    request.get<any, Credential>(`/api/v1/credentials/${id}`),

  // 创建
  create: (data: { name: string; type: string; description?: string; data: Record<string, string> }) =>
    request.post<any, Credential>('/api/v1/credentials', data),

  // 更新
  update: (id: number, data: { name?: string; description?: string; data?: Record<string, string> }) =>
    request.put<any, Credential>(`/api/v1/credentials/${id}`, data),

  // 删除
  delete: (id: number) =>
    request.delete<any, { success: boolean }>(`/api/v1/credentials/${id}`),
}
