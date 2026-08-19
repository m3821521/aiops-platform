import { useEffect, useState } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import type { Namespace } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Cluster } from '@/types'

export default function Namespaces() {
  const [data, setData] = useState<Namespace[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [cluster, setCluster] = useState<string>('')
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

  const fetchData = async () => {
    if (!cluster) return
    setLoading(true)
    try {
      const res = await k8sApi.namespaces(cluster)
      setData(res || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchClusters() }, [])
  useEffect(() => { if (cluster) fetchData() }, [cluster])

  const columns = [
    { title: 'Namespace', dataIndex: 'name', key: 'name', render: (t: string) => <span style={{ fontWeight: 500 }}>{t}</span> },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s: string) => <Tag color={s === 'Active' ? 'success' : 'default'}>{s}</Tag> },
    { title: 'Age', dataIndex: 'age', key: 'age', render: (v: string) => v || '-' },
  ]

  return (
    <Card
      title="Namespace 管理"
      extra={
        <Space>
          <Select value={cluster} onChange={setCluster} style={{ width: 160 }} placeholder="选择集群"
            options={clusters.map((c) => ({ label: c.name, value: c.name }))} />
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
