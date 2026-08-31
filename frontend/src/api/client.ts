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
    // 后端返回 { code, message, data, meta, request_id } 结构
    if (data && typeof data === 'object' && 'code' in data) {
      if (data.code === 0 || data.code === 200) {
        const result = data.data
        // 将 meta 和 request_id 作为不可枚举属性附加到返回对象上。
        // 现有业务代码 res.items 不受影响，useDataTrust 可通过 extractProvenance() 提取。
        if (result && typeof result === 'object') {
          if (data.meta) {
            Object.defineProperty(result, '__meta', {
              value: data.meta,
              enumerable: false,
              writable: false,
              configurable: true,
            })
          }
          if (data.request_id) {
            Object.defineProperty(result, '__requestId', {
              value: data.request_id,
              enumerable: false,
              writable: false,
              configurable: true,
            })
          }
        }
        return result
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
    // 提取后端返回的业务错误信息，确保 reject 的 Error.message 包含后端 message
    const respData = error.response?.data as ApiResponse | undefined
    const backendMessage = respData?.message
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
          const rid = respData?.request_id ? ` (${respData.request_id})` : ''
          message.error(`服务器错误${rid}`)
        }
      } else {
        if (!isGet) {
          message.error(backendMessage || `请求失败 (${status})`)
        }
      }
    } else if (error.code === 'ECONNABORTED') {
      if (!isGet) message.error('请求超时')
    } else {
      if (!isGet) message.error('网络错误')
    }
    // reject 时使用后端 message（如果有），确保页面能根据错误信息判断"未配置"等场景
    return Promise.reject(new Error(backendMessage || error.message || '请求失败'))
  },
)

export default request
