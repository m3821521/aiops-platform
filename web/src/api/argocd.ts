import request from './client'
import type { ArgoApp } from '@/types'

export const argocdApi = {
  apps: () => request.get<any, ArgoApp[]>('/api/v1/argocd/apps'),
  app: (name: string) => request.get<any, ArgoApp>(`/api/v1/argocd/apps/${name}`),
  sync: (name: string) => request.post(`/api/v1/argocd/apps/${name}/sync`),
  refresh: (name: string, hard?: boolean) =>
    request.post(`/api/v1/argocd/apps/${name}/refresh`, null, { params: { hard } }),
}
