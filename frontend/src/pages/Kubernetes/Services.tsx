import { useEffect, useState } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import type { Namespace } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Service, Cluster } from '@/types'

export default function Services() {
  const [data, setData] = useState<Service[]>([])
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
      const res = await k8sApi.services({ cluster, namespace: namespace || undefined })
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

  const typeColor = (type: string) => {
    if (type === 'LoadBalancer') return 'orange'
    if (type === 'NodePort') return 'blue'
    if (type === 'ClusterIP') return 'green'
    return 'default'
  }

  const columns = [
    { title: 'Service', dataIndex: 'name', key: 'name', render: (t: string) => <span style={{ fontWeight: 500 }}>{t}</span> },
    { title: 'Namespace', dataIndex: 'namespace', key: 'namespace', render: (t: string) => <Tag>{t}</Tag> },
    { title: '类型', dataIndex: 'type', key: 'type', render: (t: string) => <Tag color={typeColor(t)}>{t}</Tag> },
    { title: 'Cluster IP', dataIndex: 'cluster_ip', key: 'cluster_ip', render: (v: string) => v || '-' },
    {
      title: '端口',
      dataIndex: 'ports',
      key: 'ports',
      render: (ports: Service['ports']) =>
        ports && ports.length > 0
          ? ports.map((p, i) => (
              <Tag key={i}>{p.port}/{p.protocol} → {p.target_port}</Tag>
            ))
          : '-',
    },
    { title: 'Age', dataIndex: 'age', key: 'age', render: (v: string) => v || '-' },
  ]

  return (
    <Card
      title="Service 管理"
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
