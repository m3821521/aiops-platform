import { useEffect, useState, useRef } from 'react'
import { Table, Tag, Card, Button, Spin, Select, Space, Alert } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { k8sApi } from '@/api/kubernetes'
import type { Namespace } from '@/api/kubernetes'
import { clusterApi } from '@/api/cluster'
import type { Service, Cluster } from '@/types'
import { useDataTrust } from '@/hooks/useDataTrust'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

const POLL_INTERVAL = 15000

export default function Services() {
  const [data, setData] = useState<Service[]>([])
  const [clusters, setClusters] = useState<Cluster[]>([])
  const [namespaces, setNamespaces] = useState<Namespace[]>([])
  const [cluster, setCluster] = useState<string>('')
  const [namespace, setNamespace] = useState<string>('')
  const [loading, setLoading] = useState(false)
  const pollingRef = useRef<number | null>(null)

  const trust = useDataTrust({ source: 'kubernetes' })

  const fetchClusters = async () => {
    try {
      const res = await clusterApi.list()
      setClusters(res || [])
      if (res && res.length > 0 && !cluster) setCluster(res[0].name)
    } catch { /* ignore */ }
  }

  const fetchNamespaces = async (c: string) => {
    try {
      const res = await k8sApi.namespaces(c)
      setNamespaces(res || [])
    } catch { /* ignore */ }
  }

  const fetchData = async (silent = false) => {
    if (!cluster) return
    const seq = trust.beginFetch()
    if (!silent) setLoading(true)
    try {
      const res = await k8sApi.services({ cluster, namespace: namespace || undefined })
      trust.markSuccess(seq)
      setData(res || [])
    } catch (err: any) {
      trust.markError(seq, err?.message || '加载 Service 列表失败')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  useEffect(() => { fetchClusters() }, [])

  useEffect(() => {
    if (cluster) {
      fetchNamespaces(cluster)
      fetchData()
    }
  }, [cluster, namespace])

  useEffect(() => {
    if (!cluster) return
    pollingRef.current = window.setInterval(() => { fetchData(true) }, POLL_INTERVAL)
    return () => { if (pollingRef.current) clearInterval(pollingRef.current) }
  }, [cluster, namespace])

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
      title: '端口', dataIndex: 'ports', key: 'ports',
      render: (ports: Service['ports']) =>
        ports && ports.length > 0
          ? ports.map((p, i) => <Tag key={i}>{p.port}/{p.protocol} → {p.target_port}</Tag>)
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
          <Button icon={<ReloadOutlined />} onClick={() => fetchData()} loading={loading}>刷新</Button>
        </Space>
      }
    >
      <div style={{ marginBottom: 8 }}>
        <DataTrustIndicator
          status={trust.status}
          lastSuccessfulAt={trust.lastSuccessfulAt}
          ageSeconds={trust.ageSeconds}
          sourceLabel={trust.sourceLabel}
          error={trust.error}
          formatAge={trust.formatAge}
          formatLastSuccessful={trust.formatLastSuccessful}
        />
      </div>
      {trust.error && trust.status === 'stale' && (
        <Alert message="数据刷新失败" description={trust.error + '（当前显示的是上次成功获取的数据）'} type="warning" showIcon style={{ marginBottom: 12 }} />
      )}
      <Spin spinning={loading}>
        <Table columns={columns} dataSource={data} rowKey="name" pagination={{ pageSize: 20 }} size="middle" />
      </Spin>
    </Card>
  )
}
