import { useEffect, useState } from 'react'
import { Table, Tag, Card, Space, Button, Spin } from 'antd'
import { ReloadOutlined, CloudServerOutlined } from '@ant-design/icons'
import { clusterApi } from '@/api/cluster'
import type { Cluster } from '@/types'

export default function Clusters() {
  const [data, setData] = useState<Cluster[]>([])
  const [loading, setLoading] = useState(false)

  const fetchData = async () => {
    setLoading(true)
    try {
      const res = await clusterApi.list()
      setData(res || [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchData()
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
        <Button icon={<ReloadOutlined />} onClick={fetchData} loading={loading}>
          刷新
        </Button>
      }
    >
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
