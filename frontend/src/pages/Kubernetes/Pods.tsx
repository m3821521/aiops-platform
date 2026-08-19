import { useEffect, useState } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space, Input } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import type { Namespace } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Pod, Cluster } from '@/types'

export default function Pods() {
  const [data, setData] = useState<Pod[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [namespaces, setNamespaces] = useState<Namespace[]>([])
  const [cluster, setCluster] = useState<string>('')
  const [namespace, setNamespace] = useState<string>('')
  const [keyword, setKeyword] = useState<string>('')
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
      const res = await k8sApi.pods({ cluster, namespace: namespace || undefined })
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

  const statusColor = (status: string) => {
    if (status === 'Running') return 'success'
    if (status === 'Pending') return 'warning'
    if (status === 'Failed' || status === 'Unknown') return 'error'
    return 'default'
  }

  const filtered = data.filter((p) =>
    !keyword || p.name.toLowerCase().includes(keyword.toLowerCase())
  )

  const columns = [
    { title: 'Pod', dataIndex: 'name', key: 'name', render: (t: string) => <span style={{ fontWeight: 500 }}>{t}</span> },
    { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', render: (t: string) => <Tag>{t}</Tag> },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s: string) => <Tag color={statusColor(s)}>{s}</Tag> },
    { title: 'Node', dataIndex: 'node', key: 'node', render: (v: string) => v || '-' },
    { title: 'IP', dataIndex: 'ip', key: 'ip', render: (v: string) => v || '-' },
    { title: '重启次数', dataIndex: 'restart_count', key: 'restart', render: (v: number) => v ?? 0 },
    { title: 'Age', dataIndex: 'age', key: 'age', render: (v: string) => v || '-' },
  ]

  return (
    <Card
      title="Pod 管理"
      extra={
        <Space wrap>
          <Select value={cluster} onChange={setCluster} style={{ width: 140 }} placeholder="选择集群"
            options={clusters.map((c) => ({ label: c.name, value: c.name }))} />
          <Select value={namespace} onChange={setNamespace} style={{ width: 160 }} placeholder="所有命名空间" allowClear
            options={namespaces.map((n) => ({ label: n.name, value: n.name }))} />
          <Input.Search placeholder="搜索 Pod" value={keyword} onChange={(e) => setKeyword(e.target.value)} style={{ width: 180 }} allowClear />
          <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>刷新</Button>
        </Space>
      }
    >
      <Spin spinning={loading}>
        <Table columns={columns} dataSource={filtered} rowKey="name" pagination={{ pageSize: 20 }} size="middle" />
      </Spin>
    </Card>
  )
}
