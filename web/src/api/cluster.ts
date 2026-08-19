import request from './client'
import type { Cluster } from '@/types'

export const clusterApi = {
  list: () => request.get<any, Cluster[]>('/api/v1/clusters'),
}
