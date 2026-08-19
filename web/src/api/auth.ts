import request from './client'
import type { User } from '@/types'

export const authApi = {
  login: (username: string, password: string) =>
    request.post<any, { access_token: string; token_type: string; expires_in: number; user: User }>(
      '/api/v1/auth/login',
      { username, password },
    ),
  logout: () => request.post('/api/v1/auth/logout'),
  me: () => request.get<any, User>('/api/v1/auth/me'),
}
