import { useEffect, useState, useRef } from 'react'
import { Table, Tag, Card, Space, Button, Spin, Alert } from 'antd'
import { ReloadOutlined, CloudServerOutlined } from '@ant-design/icons'
import { clusterApi } from '@/api/cluster'
import type { Cluster } from '@/types'
import { useDataTrust } from '@/hooks/useDataTrust'
import { extractProvenance } from '@/utils/provenance'
import { DataTrustIndicator } from '@/components/DataTrustIndicator'

const POLL_INTERVAL = 15000

export default function Clusters() {
  const [data, setData] = useState<Cluster[]>([])
  const [loading, setLoading] = useState(false)
  const pollingRef = useRef<number | null>(null)

  // P1-X.9: 统一 Data Trust（Clusters 来自 Kubernetes API / 集群配置）
  const trust = useDataTrust({ source: 'kubernetes' })

  const fetchData = async (silent = false) => {
    const seq = trust.beginFetch()
    if (!silent) setLoading(true)
    try {
      const res = await clusterApi.list()
      trust.markSuccess(seq, extractProvenance(res))
      setData(res || [])
    } catch (err: any) {
      // P1-X.10 Phase 3.2: API failure 不得转换为空数据，保留上次成功数据并标记 error/stale
      trust.markError(seq, err?.message || '加载集群列表失败')
    } finally {
      if (!silent) setLoading(false)
    }
  }

  // 首次加载
  useEffect(() => {
    fetchData()
  }, [])

  // 15s 自动轮询
  useEffect(() => {
    pollingRef.current = window.setInterval(() => {
      fetchData(true)
    }, POLL_INTERVAL)
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
    }
  }, [])

  const columns = [
    {
      title: '集群名称',
      dataIndex: 'name',
      key: 'name',
      render: (text: string) => (
        <Space>
          <CloudServerOutlined />
          <span style={{ fontWeight: 500 }}>{text}</span>
        </Space>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
    },
    {
      title: '认证方式',
      dataIndex: 'auth_type',
      key: 'auth_type',
      render: (type: string) => {
        const map: Record<string, { color: string; label: string }> = {
          kubeconfig: { color: 'blue', label: 'Kubeconfig' },
          serviceaccount: { color: 'green', label: 'ServiceAccount' },
          incluster: { color: 'purple', label: 'In-Cluster' },
        }
        const item = map[type] || { color: 'default', label: type }
        return <Tag color={item.color}>{item.label}</Tag>
      },
    },
    {
      title: '状态',
      dataIndex: 'enabled',
      key: 'enabled',
      render: (enabled: boolean) => (
        <Tag color={enabled ? 'success' : 'default'}>
          {enabled ? '已启用' : '已禁用'}
        </Tag>
      ),
    },
  ]

  return (
    <Card
      title="集群管理"
      extra={
        <Button icon={<ReloadOutlined />} onClick={() => fetchData()} loading={loading}>
          刷新
        </Button>
      }
    >
      {/* P1-X.10 Phase 3.2: Data Trust 状态指示器 */}
      <div style={{ marginBottom: 8 }}>
        <DataTrustIndicator
          status={trust.status}
          lastSuccessfulAt={trust.lastSuccessfulAt}
          fetchAgeSeconds={trust.fetchAgeSeconds}
          sourceLabel={trust.sourceLabel}
          error={trust.error}
          formatFetchAge={trust.formatFetchAge}
          formatLastSuccessful={trust.formatLastSuccessful}
          dataAgeSeconds={trust.dataAgeSeconds}
          dataTimestampAvailable={trust.dataTimestampAvailable}
          provenance={trust.provenance}
        />
      </div>

      {/* P1-X.10 Phase 3.2: API failure 时明确提示，不伪装成空数据 */}
      {trust.error && trust.status === 'stale' && (
        <Alert
          message="数据刷新失败"
          description={trust.error + '（当前显示的是上次成功获取的数据）'}
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
        />
      )}
      {trust.status === 'error' && (
        <Alert
          message="加载失败"
          description={trust.error || '无法获取集群列表数据'}
          type="error"
          showIcon
          style={{ marginBottom: 12 }}
        />
      )}

      <Spin spinning={loading}>
        <Table
          columns={columns}
          dataSource={data}
          rowKey="name"
          pagination={false}
          size="middle"
        />
      </Spin>
    </Card>
  )
}
