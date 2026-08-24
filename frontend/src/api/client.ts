import axios, { AxiosError, InternalAxiosRequestConfig } from 'axios'
import { message } from 'antd'
import type { ApiResponse } from '@/types'

const request = axios.create({
  baseURL: '/',
  timeout: 30000,
})

// 请求拦截器：附加 JWT Token
request.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const token = localStorage.getItem('aiops_token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error),
)

// 响应拦截器：统一错误处理
request.interceptors.response.use(
  (response) => {
    const data = response.data as ApiResponse
    // 后端返回 { code, message, data } 结构
    if (data && typeof data === 'object' && 'code' in data) {
      if (data.code === 0 || data.code === 200) {
        return data.data
      }
      // GET 请求不全局弹窗，由页面自行处理空状态/错误提示
      const method = response.config.method?.toLowerCase()
      if (method !== 'get') {
        message.error(data.message || '请求失败')
      }
      return Promise.reject(new Error(data.message || '请求失败'))
    }
    return response.data
  },
  (error: AxiosError) => {
    const method = error.config?.method?.toLowerCase()
    const isGet = method === 'get'
    if (error.response) {
      const status = error.response.status
      if (status === 401) {
        message.error('登录已过期，请重新登录')
        localStorage.removeItem('aiops_token')
        localStorage.removeItem('aiops_user')
        window.location.href = '/login'
      } else if (status === 403) {
        if (!isGet) message.error('权限不足')
      } else if (status === 404) {
        if (!isGet) message.error('资源不存在')
      } else if (status === 409) {
        if (!isGet) message.error('操作冲突，请刷新后重试')
      } else if (status === 429) {
        // GET 请求不全局弹窗，避免 Dashboard 批量请求被限流时弹出大量重复提示
        if (!isGet) message.error('请求过于频繁，请稍后再试')
      } else if (status >= 500) {
        // GET 请求的 500 不全局弹窗（如 k8s/Prometheus 不可用），由页面显示空状态
        if (!isGet) {
          const data = error.response.data as ApiResponse
          const rid = data?.request_id ? ` (${data.request_id})` : ''
          message.error(`服务器错误${rid}`)
        }
      } else {
        if (!isGet) {
          const data = error.response.data as ApiResponse
          message.error(data?.message || `请求失败 (${status})`)
        }
      }
    } else if (error.code === 'ECONNABORTED') {
      if (!isGet) message.error('请求超时')
    } else {
      if (!isGet) message.error('网络错误')
    }
    return Promise.reject(error)
  },
)

export default request
