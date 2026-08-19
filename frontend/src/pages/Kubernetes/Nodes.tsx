import { useEffect, useState } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Node, Cluster } from '@/types'

export default function Nodes() {
  const [data, setData] = useState<Node[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [cluster, setCluster] = useState<string>('')
  const [loading, setLoading] = useState(false)

  const fetchClusters = async () => {
    try {
      const res = await clusterApi.list()
      setClusters(res || [])
      if (res && res.length > 0 && !cluster) {
        setCluster(res[0].name)
      }
    } catch {
      // ignore
    }
  }

  const fetchData = async () => {
    if (!cluster) return
    setLoading(true)
    try {
      const res = await k8sApi.nodes(cluster)
      setData(res || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchClusters()
  }, [])

  useEffect(() => {
    if (cluster) fetchData()
  }, [cluster])

  const statusColor = (status: string) => {
    if (status === 'Ready') return 'success'
    if (status === 'NotReady') return 'error'
    return 'default'
  }

  const columns = [
    { title: 'Node', dataIndex: 'name', key: 'name', render: (t: string) => <span style={{ fontWeight: 500 }}>{t}</span> },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s: string) => <Tag color={statusColor(s)}>{s}</Tag> },
    { title: 'K8s 版本', dataIndex: 'version', key: 'version', render: (v: string) => v || '-' },
    { title: '内部 IP', dataIndex: 'internal_ip', key: 'ip', render: (v: string) => v || '-' },
    { title: 'Pod 数量', dataIndex: 'pod_count', key: 'pods', render: (v: number) => v ?? '-' },
    { title: 'CPU 使用率', dataIndex: 'cpu_usage', key: 'cpu', render: (v: number) => v != null ? `${v}%` : '-' },
    { title: '内存使用率', dataIndex: 'memory_usage', key: 'mem', render: (v: number) => v != null ? `${v}%` : '-' },
    { title: 'Age', dataIndex: 'age', key: 'age', render: (v: string) => v || '-' },
  ]

  return (
    <Card
      title="Node 管理"
      extra={
        <Space>
          <Select
            value={cluster}
            onChange={setCluster}
            style={{ width: 160 }}
            placeholder="选择集群"
            options={clusters.map((c) => ({ label: c.name, value: c.name }))}
          />
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
