import { create } from 'zustand'
import type { User } from '@/types'
import { authApi } from '@/api/auth'

interface AuthState {
  token: string | null
  user: User | null
  isAuthenticated: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  fetchMe: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set) => ({
  token: localStorage.getItem('aiops_token'),
  user: localStorage.getItem('aiops_user')
    ? JSON.parse(localStorage.getItem('aiops_user')!)
    : null,
  isAuthenticated: !!localStorage.getItem('aiops_token'),

  login: async (username, password) => {
    const res = await authApi.login(username, password)
    localStorage.setItem('aiops_token', res.access_token)
    localStorage.setItem('aiops_user', JSON.stringify(res.user))
    set({ token: res.access_token, user: res.user, isAuthenticated: true })
  },

  logout: () => {
    authApi.logout().catch(() => {})
    localStorage.removeItem('aiops_token')
    localStorage.removeItem('aiops_user')
    set({ token: null, user: null, isAuthenticated: false })
  },

  fetchMe: async () => {
    try {
      const user = await authApi.me()
      localStorage.setItem('aiops_user', JSON.stringify(user))
      set({ user })
    } catch {
      // token 失效，清除
      localStorage.removeItem('aiops_token')
      localStorage.removeItem('aiops_user')
      set({ token: null, user: null, isAuthenticated: false })
    }
  },
}))
