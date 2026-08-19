import { useEffect, useState } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import type { Namespace } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Cluster } from '@/types'

// 后端实际返回的 Deployment 结构
interface DeploymentItem {
  name: string
  namespace: string
  ready: string // 如 "1/1"
}

export default function Deployments() {
  const [data, setData] = useState<DeploymentItem[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [namespaces, setNamespaces] = useState<Namespace[]>([])
  const [cluster, setCluster] = useState<string>('')
  const [namespace, setNamespace] = useState<string>('')
  const [loading, setLoading] = useState(false)

  const fetchClusters = async () => {
    try {
      const res = await clusterApi.list()
      setClusters(res || [])
      if (res && res.length > 0 && !cluster) setCluster(res[0].name)
    } catch {
      // ignore
    }
  }

  const fetchNamespaces = async (c: string) => {
    try {
      const res = await k8sApi.namespaces(c)
      setNamespaces(res || [])
    } catch {
      // ignore
    }
  }

  const fetchData = async () => {
    if (!cluster) return
    setLoading(true)
    try {
      const res = await k8sApi.deployments({ cluster, namespace: namespace || undefined })
      setData((res as unknown as DeploymentItem[]) || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => {
    if (cluster) {
      fetchNamespaces(cluster)
      fetchData()
    }
  }, [cluster])

  const readyColor = (ready: string) => {
    if (!ready) return 'default'
    const [r, t] = ready.split('/').map(Number)
    if (r === t) return 'success'
    if (r === 0) return 'error'
    return 'warning'
  }

  const columns = [
    { title: 'Deployment', dataIndex: 'name', key: 'name', render: (t: string) => <span style={{ fontWeight: 500 }}>{t}</span> },
    { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', render: (t: string) => <Tag>{t}</Tag> },
    { title: '副本', dataIndex: 'ready', key: 'ready', render: (v: string) => <Tag color={readyColor(v)}>{v || '-'}</Tag> },
  ]

  return (
    <Card
      title="Deployment 管理"
      extra={
        <Space wrap>
          <Select value={cluster} onChange={setCluster} style={{ width: 140 }} placeholder="选择集群"
            options={clusters.map((c) => ({ label: c.name, value: c.name }))} />
          <Select value={namespace} onChange={setNamespace} style={{ width: 160 }} placeholder="所有命名空间" allowClear
            options={namespaces.map((n) => ({ label: n.name, value: n.name }))} />
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>刷新</Button>
        </Space>
      }
    >
      <Spin spinning={loading}>
        <Table columns={columns} dataSource={data} rowKey="name" pagination={{ pageSize: 20 }} size="middle" />
      </Spin>
    </Card>
  )
}
