import request from './client'
import type { JenkinsJob, JenkinsBuild } from '@/types'

export const jenkinsApi = {
  jobs: () => request.get<any, JenkinsJob[]>('/api/v1/jenkins/jobs'),
  builds: (job: string) =>
    request.get<any, JenkinsBuild[]>(`/api/v1/jenkins/jobs/${job}/builds`),
  build: (job: string) => request.post(`/api/v1/jenkins/jobs/${job}/build`),
  buildLog: (job: string, number: number) =>
    request.get<any, string>(`/api/v1/jenkins/jobs/${job}/builds/${number}/log`),
}
