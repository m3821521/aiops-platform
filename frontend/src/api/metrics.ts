import request from './client'

export const metricsApi = {
  query: (query: string, time?: string) =>
    request.get('/api/v1/metrics/query', { params: { query, time } }),
  range: (params: { query: string; start?: string; end?: string; step?: string }) =>
    request.get('/api/v1/metrics/range', { params }),
  nodes: () => request.get('/api/v1/metrics/nodes'),
  pods: (namespace?: string) =>
    request.get('/api/v1/metrics/pods', { params: { namespace } }),
}
