import request from './client'
import type { Cluster } from '@/types'

export interface NodeMetric {
  name: string
  cpu_cores: string
  cpu_percent: number
  memory_bytes: string
  memory_percent: number
  window: string
}

export interface PodMetric {
  namespace: string
  name: string
  cpu_cores: string
  memory_bytes: string
  window: string
}

export const clusterApi = {
  list: () => request.get<any, Cluster[]>('/api/v1/clusters'),
  nodeMetrics: () => request.get<any, NodeMetric[]>('/api/v1/nodes/metrics'),
  podMetrics: (namespace?: string) => request.get<any, PodMetric[]>('/api/v1/pods/metrics', { params: { namespace } }),
}
