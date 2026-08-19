import { useState, useEffect, useCallback } from 'react'
import {
  Table, Card, Tag, Button, Space, Modal, message, Drawer, Descriptions, Timeline, Badge, Empty,
} from 'antd'
import {
  PlayCircleOutlined, CheckCircleOutlined, CloseCircleOutlined,
  ThunderboltOutlined, ApartmentOutlined,
} from '@ant-design/icons'
import { workflowApi, type Workflow } from '@/api/workflow'

const statusColor: Record<string, string> = {
  draft: 'default',
  pending_approval: 'orange',
  approved: 'blue',
  running: 'cyan',
  success: 'green',
  failed: 'red',
  cancelled: 'default',
}

const stepStatusColor: Record<string, string> = {
  pending: 'default',
  running: 'cyan',
  success: 'green',
  failed: 'red',
  skipped: 'default',
}

const riskColor: Record<string, string> = {
  low: 'green',
  medium: 'orange',
  high: 'red',
  critical: 'volcano',
}

export default function Workflows() {
  const [items, setItems] = useState<Workflow[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [detail, setDetail] = useState<Workflow | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await workflowApi.list({ page, page_size: pageSize })
      setItems(res.items)
      setTotal(res.total)
    } catch (e: any) {
      message.error('加载工作流失败: ' + (e?.message || ''))
    } finally {
      setLoading(false)
    }
  }, [page, pageSize])

  useEffect(() => { load() }, [load])

  const openDetail = async (wf: Workflow) => {
    try {
      const full = await workflowApi.get(wf.id)
      setDetail(full)
      setDetailOpen(true)
    } catch (e: any) {
      message.error('加载工作流详情失败: ' + (e?.message || ''))
    }
  }

  const handleSubmit = async (id: number) => {
    Modal.confirm({
      title: '提交审批',
      content: '提交后工作流将进入待审批状态。确认提交？',
      onOk: async () => {
        try {
          await workflowApi.submit(id)
          message.success('已提交审批')
          load()
        } catch (e: any) {
          message.error('提交失败: ' + (e?.message || ''))
        }
      },
    })
  }

  const handleApprove = async (id: number) => {
    Modal.confirm({
      title: '审批工作流',
      content: '审批通过后工作流可以被执行。确认审批？',
      okText: '确认审批',
      onOk: async () => {
        try {
          await workflowApi.approve(id)
          message.success('审批通过')
          load()
          if (detail?.id === id) openDetail(detail)
        } catch (e: any) {
          message.error('审批失败: ' + (e?.message || ''))
        }
      },
    })
  }

  const handleExecute = async (id: number) => {
    Modal.confirm({
      title: '执行工作流',
      content: '此操作将按顺序执行所有步骤，可能修改 Kubernetes/Jenkins/ArgoCD 资源。确认执行？',
      okText: '确认执行',
      okType: 'danger',
      onOk: async () => {
        try {
          const result = await workflowApi.execute(id)
          message.success(`执行完成: ${result.status}`)
          load()
          openDetail(result)
        } catch (e: any) {
          message.error('执行失败: ' + (e?.message || ''))
        }
      },
    })
  }

  const columns = [
    { title: 'ID', dataIndex: 'id', width: 70, render: (id: number) => <span style={{ fontFamily: 'monospace' }}>#{id}</span> },
    { title: '名称', dataIndex: 'name', render: (name: string, record: Workflow) => <a onClick={() => openDetail(record)}>{name}</a> },
    { title: '步骤数', dataIndex: 'steps', width: 80, render: (steps: any[]) => steps?.length || 0 },
    { title: '风险', dataIndex: 'risk', width: 80, render: (r: string) => <Tag color={riskColor[r]}>{r}</Tag> },
    { title: '状态', dataIndex: 'status', width: 130, render: (s: string) => <Badge color={statusColor[s]} text={s} /> },
    { title: '关联事件', dataIndex: 'incident_id', width: 90, render: (id: number) => id ? `#${id}` : '-' },
    { title: '创建时间', dataIndex: 'created_at', width: 170, render: (t: string) => new Date(t).toLocaleString() },
    {
      title: '操作', width: 220,
      render: (_: any, record: Workflow) => (
        <Space size={4}>
          <Button size="small" type="link" onClick={() => openDetail(record)}>详情</Button>
          {record.status === 'draft' && <Button size="small" type="link" onClick={() => handleSubmit(record.id)}>提交</Button>}
          {record.status === 'pending_approval' && <Button size="small" type="link" onClick={() => handleApprove(record.id)}>审批</Button>}
          {record.status === 'approved' && <Button size="small" type="link" danger onClick={() => handleExecute(record.id)}>执行</Button>}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <Card
        title={<Space><ApartmentOutlined /> 自动化工作流</Space>}
        extra={<Button onClick={load}>刷新</Button>}
      >
        <Table
          rowKey="id"
          columns={columns}
          dataSource={items}
          loading={loading}
          pagination={{ current: page, pageSize, total, showSizeChanger: true, onChange: (p, ps) => { setPage(p); setPageSize(ps) } }}
          size="small"
        />
      </Card>

      <Drawer
        title={detail ? `工作流 #${detail.id}: ${detail.name}` : ''}
        width={600}
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        destroyOnClose
      >
        {detail && (
          <div>
            <Descriptions column={2} size="small" bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="状态"><Badge color={statusColor[detail.status]} text={detail.status} /></Descriptions.Item>
              <Descriptions.Item label="风险"><Tag color={riskColor[detail.risk]}>{detail.risk}</Tag></Descriptions.Item>
              <Descriptions.Item label="步骤数">{detail.steps?.length || 0}</Descriptions.Item>
              <Descriptions.Item label="关联事件">{detail.incident_id ? `#${detail.incident_id}` : '-'}</Descriptions.Item>
              <Descriptions.Item label="创建时间" span={2}>{new Date(detail.created_at).toLocaleString()}</Descriptions.Item>
              {detail.approved_at && <Descriptions.Item label="审批时间" span={2}>{new Date(detail.approved_at).toLocaleString()}</Descriptions.Item>}
              {detail.started_at && <Descriptions.Item label="执行时间" span={2}>{new Date(detail.started_at).toLocaleString()}</Descriptions.Item>}
              {detail.duration_ms ? <Descriptions.Item label="耗时" span={2}>{detail.duration_ms}ms</Descriptions.Item> : null}
            </Descriptions>

            <Space style={{ marginBottom: 16 }}>
              {detail.status === 'draft' && <Button onClick={() => handleSubmit(detail.id)}>提交审批</Button>}
              {detail.status === 'pending_approval' && <Button type="primary" onClick={() => handleApprove(detail.id)}>审批通过</Button>}
              {detail.status === 'approved' && <Button type="primary" danger icon={<PlayCircleOutlined />} onClick={() => handleExecute(detail.id)}>执行</Button>}
              {(detail.status === 'draft' || detail.status === 'pending_approval' || detail.status === 'approved') && (
                <Button onClick={() => {
                  Modal.confirm({
                    title: '取消工作流',
                    onOk: async () => { await workflowApi.cancel(detail.id); message.success('已取消'); load(); setDetailOpen(false) },
                  })
                }}>取消</Button>
              )}
            </Space>

            <Card size="small" title={<Space><ThunderboltOutlined /> 执行步骤</Space>}>
              {!detail.steps || detail.steps.length === 0 ? (
                <Empty description="暂无步骤" />
              ) : (
                <Timeline
                  items={detail.steps.map((step) => ({
                    color: step.status === 'success' ? 'green' : step.status === 'failed' ? 'red' : step.status === 'running' ? 'cyan' : step.status === 'skipped' ? 'gray' : 'blue',
                    children: (
                      <div>
                        <Space>
                          <strong>Step {step.order}: {step.name || step.action_type}</strong>
                          <Tag color={stepStatusColor[step.status]}>{step.status}</Tag>
                          {step.target_name && <Tag style={{ fontSize: 10 }}>{step.target_name}</Tag>}
                        </Space>
                        {step.result && <div style={{ fontSize: 12, color: '#666', marginTop: 4 }}>{step.result}</div>}
                        {step.error && <div style={{ fontSize: 12, color: '#ff4d4f', marginTop: 4 }}>错误: {step.error}</div>}
                        {step.finished_at && <div style={{ fontSize: 11, color: '#999', marginTop: 2 }}>{new Date(step.finished_at).toLocaleString()}</div>}
                      </div>
                    ),
                  }))}
                />
              )}
            </Card>
          </div>
        )}
      </Drawer>
    </div>
  )
}
