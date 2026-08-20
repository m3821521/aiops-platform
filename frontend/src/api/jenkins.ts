import request from './client'
import type { JenkinsJob, JenkinsBuild } from '@/types'

export const jenkinsApi = {
  jobs: (connectionId?: number) =>
    request.get<any, JenkinsJob[]>('/api/v1/jenkins/jobs', { params: connectionId ? { connection_id: connectionId } : {} }),
  builds: (job: string, connectionId?: number) =>
    request.get<any, JenkinsBuild[]>(`/api/v1/jenkins/jobs/${job}/builds`, { params: connectionId ? { connection_id: connectionId } : {} }),
  build: (job: string, connectionId?: number) =>
    request.post(`/api/v1/jenkins/jobs/${job}/build`, null, { params: connectionId ? { connection_id: connectionId } : {} }),
  buildLog: (job: string, number: number, connectionId?: number) =>
    request.get<any, string>(`/api/v1/jenkins/jobs/${job}/builds/${number}/log`, { params: connectionId ? { connection_id: connectionId } : {} }),
}
