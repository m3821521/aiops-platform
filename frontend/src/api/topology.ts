import request from './client'
import type { TopologyGraph, TopologyNode, TopologyDependency, TopologyImpact, TopologyNodeType } from '@/types'

export const topologyApi = {
  getGraph(params: { cluster?: string; namespace?: string; refresh?: boolean }) {
    return request.get<any, TopologyGraph>('/api/v1/topology', { params })
  },

  getNode(cluster: string, type: TopologyNodeType, namespace: string, name: string) {
    return request.get<any, TopologyNode>(`/api/v1/topology/nodes/${type}/${name}`, {
      params: { cluster, namespace },
    })
  },

  getDependencies(cluster: string, type: TopologyNodeType, namespace: string, name: string) {
    return request.get<any, TopologyDependency>(`/api/v1/topology/dependencies/${type}/${name}`, {
      params: { cluster, namespace },
    })
  },

  getImpact(cluster: string, type: TopologyNodeType, namespace: string, name: string) {
    return request.get<any, TopologyImpact>(`/api/v1/topology/impact/${type}/${name}`, {
      params: { cluster, namespace },
    })
  },

  invalidateCache(cluster: string, namespace?: string) {
    return request.post<any, { status: string }>('/api/v1/topology/cache/invalidate', null, {
      params: { cluster, namespace },
    })
  },
}
