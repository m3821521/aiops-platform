import request from './client'

export const automationApi = {
  podLogs: (pod: string, params: { cluster?: string; namespace?: string; container?: string; tail?: number }) =>
    request.get(`/api/v1/automation/pods/${pod}/logs`, { params }),
  podEvents: (pod: string, params: { cluster?: string; namespace?: string }) =>
    request.get(`/api/v1/automation/pods/${pod}/events`, { params }),
  restartPod: (pod: string, data: { cluster?: string; namespace?: string; confirm: boolean }) =>
    request.post(`/api/v1/automation/pods/${pod}/restart`, data),
  scaleDeployment: (name: string, data: { cluster?: string; namespace?: string; replicas: number; confirm: boolean }) =>
    request.post(`/api/v1/automation/deployments/${name}/scale`, data),
}
