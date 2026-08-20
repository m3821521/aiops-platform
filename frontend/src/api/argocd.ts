import request from './client'
import type { ArgoApp } from '@/types'

export const argocdApi = {
  apps: (connectionId?: number) =>
    request.get<any, ArgoApp[]>('/api/v1/argocd/apps', { params: connectionId ? { connection_id: connectionId } : {} }),
  app: (name: string, connectionId?: number) =>
    request.get<any, ArgoApp>(`/api/v1/argocd/apps/${name}`, { params: connectionId ? { connection_id: connectionId } : {} }),
  sync: (name: string, connectionId?: number) =>
    request.post(`/api/v1/argocd/apps/${name}/sync`, null, { params: connectionId ? { connection_id: connectionId } : {} }),
  refresh: (name: string, hard?: boolean, connectionId?: number) =>
    request.post(`/api/v1/argocd/apps/${name}/refresh`, null, { params: { ...(hard ? { hard: 'true' } : {}), ...(connectionId ? { connection_id: connectionId } : {}) } }),
}
