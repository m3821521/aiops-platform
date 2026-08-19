import { useEffect, useState } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space, Progress } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import type { Namespace } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Deployment, Cluster } from '@/types'

export default function Deployments() {
  const [data, setData] = useState<Deployment[]>([])
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
      setData(res || [])
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

  const columns = [
    { title: 'Deployment', dataIndex: 'name', key: 'name', render: (t: string) => <span style={{ fontWeight: 500 }}>{t}</span> },
    { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', render: (t: string) => <Tag>{t}</Tag> },
    {
      title: '副本',
      key: 'replicas',
      render: (_: any, r: Deployment) => {
        const total = r.replicas || 0
        const ready = r.ready_replicas || 0
        const pct = total > 0 ? Math.round((ready / total) * 100) : 0
        return (
          <Space>
            <span>{ready}/{total}</span>
            <Progress percent={pct} size="small" style={{ width: 80 }} status={pct === 100 ? 'success' : 'active'} />
          </Space>
        )
      },
    },
    { title: '可用', dataIndex: 'available_replicas', key: 'available', render: (v: number) => v ?? 0 },
    { title: '已更新', dataIndex: 'updated_replicas', key: 'updated', render: (v: number) => v ?? 0 },
    { title: 'Age', dataIndex: 'age', key: 'age', render: (v: string) => v || '-' },
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
