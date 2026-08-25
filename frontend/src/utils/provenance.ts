import type { DataProvenance } from '@/types'

/**
 * 从 API 响应对象中提取 meta.provenance。
 * axios interceptor 将 meta 作为不可枚举属性 __meta 附加到返回对象上。
 * 对于数组返回（如 K8s list），无法附加属性，返回 undefined。
 */
export function extractProvenance(data: unknown): DataProvenance | undefined {
  if (!data || typeof data !== 'object') {
    return undefined
  }
  const meta = (data as any).__meta
  if (meta && meta.provenance) {
    return meta.provenance as DataProvenance
  }
  return undefined
}

/**
 * 从 API 响应对象中提取 request_id。
 */
export function extractRequestId(data: unknown): string | undefined {
  if (!data || typeof data !== 'object') {
    return undefined
  }
  return (data as any).__requestId
}
