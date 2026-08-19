import request from './client'
import type { Node, Pod, Deployment, Service } from '@/types'

export interface Namespace {
  name: string
  status: string
  age?: string
}

export const k8sApi = {
  nodes: (cluster?: string) =>
    request.get<any, Node[]>('/api/v1/nodes', { params: { cluster } }),
  namespaces: (cluster?: string) =>
    request.get<any, Namespace[]>('/api/v1/namespaces', { params: { cluster } }),
  pods: (params: { cluster?: string; namespace?: string }) =>
    request.get<any, Pod[]>('/api/v1/pods', { params }),
  deployments: (params: { cluster?: string; namespace?: string }) =>
    request.get<any, Deployment[]>('/api/v1/deployments', { params }),
  statefulsets: (params: { cluster?: string; namespace?: string }) =>
    request.get<any, any[]>('/api/v1/statefulsets', { params }),
  daemonsets: (params: { cluster?: string; namespace?: string }) =>
    request.get<any, any[]>('/api/v1/daemonsets', { params }),
  services: (params: { cluster?: string; namespace?: string }) =>
    request.get<any, Service[]>('/api/v1/services', { params }),
  configmaps: (params: { cluster?: string; namespace?: string }) =>
    request.get<any, any[]>('/api/v1/configmaps', { params }),
  secrets: (params: { cluster?: string; namespace?: string }) =>
    request.get<any, any[]>('/api/v1/secrets', { params }),
}
