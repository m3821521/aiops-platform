import request from './client'
import type { Cluster } from '@/types'

export interface NodeMetric {
  name: string
  cpu_cores: string
  cpu_percent: number
  memory_bytes: string
  memory_percent: number
  window: string
  timestamp: string
}

export interface PodMetric {
  namespace: string
  name: string
  cpu_cores: string
  memory_bytes: string
  window: string
  timestamp: string
}

export interface NamespaceItem {
  name: string
  status: string
  age: string
}

export interface PodItem {
  namespace: string
  name: string
  status: string
  node?: string
  age?: string
  ip?: string
}

export const clusterApi = {
  list: () => request.get<any, Cluster[]>('/api/v1/clusters'),
  listNamespaces: () => request.get<any, NamespaceItem[]>('/api/v1/namespaces'),
  listPods: (namespace?: string) => request.get<any, PodItem[]>('/api/v1/pods', { params: namespace ? { namespace } : {} }),
  nodeMetrics: () => request.get<any, NodeMetric[]>('/api/v1/nodes/metrics'),
  podMetrics: (namespace?: string) => request.get<any, PodMetric[]>('/api/v1/pods/metrics', { params: { namespace } }),
}
