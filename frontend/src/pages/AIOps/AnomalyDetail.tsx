import { useEffect, useRef } from 'react'
import { Drawer, Descriptions, Tag, Space, Button, Card, Empty } from 'antd'
import * as echarts from 'echarts'
import type { AnomalyRecord } from '../../types'

interface Props {
  record: AnomalyRecord | null
  onClose: () => void
  onRefresh: () => void
}

const severityColor: Record<string, string> = {
  critical: 'red',
  warning: 'orange',
  info: 'blue',
}

export default function AnomalyDetail({ record, onClose, onRefresh }: Props) {
  const chartRef = useRef<HTMLDivElement>(null)
  const chartInstance = useRef<echarts.ECharts | null>(null)

  useEffect(() => {
    if (!record || !chartRef.current) return

    if (!chartInstance.current) {
      chartInstance.current = echarts.init(chartRef.current)
    }

    // 构造模拟时间序列数据（基于异常记录的时间点和基线）
    // 实际生产中应该从 Prometheus 查询该指标的历史数据
    const baseTime = new Date(record.timestamp).getTime()
    const points = []
    const anomalyPoints = []
    for (let i = -10; i <= 2; i++) {
      const t = baseTime + i * 30000
      const val = i === 0 ? record.value : record.baseline || record.value * 0.5
      points.push([new Date(t).toLocaleTimeString(), val.toFixed(2)])
      if (i === 0) {
        anomalyPoints.push([new Date(t).toLocaleTimeString(), val.toFixed(2)])
      }
    }

    chartInstance.current.setOption({
      tooltip: { trigger: 'axis' },
      legend: { data: ['实际值', '基线', '异常点'], textStyle: { fontSize: 11 } },
      grid: { left: 50, right: 20, top: 30, bottom: 30 },
      xAxis: { type: 'category', data: points.map((p) => p[0]) },
      yAxis: { type: 'value', name: record.metric.slice(0, 20) },
      series: [
        {
          name: '实际值',
          type: 'line',
          data: points.map((p) => p[1]),
          smooth: true,
          lineStyle: { width: 2 },
          itemStyle: { color: '#1890ff' },
        },
        {
          name: '基线',
          type: 'line',
          data: points.map(() => (record.baseline || 0).toFixed(2)),
          lineStyle: { type: 'dashed', color: '#52c41a' },
          itemStyle: { color: '#52c41a' },
        },
        {
          name: '异常点',
          type: 'scatter',
          data: anomalyPoints.map((p) => p[1]),
          symbolSize: 12,
          itemStyle: { color: '#ff4d4f' },
        },
      ],
    })

    return () => {
      chartInstance.current?.dispose()
      chartInstance.current = null
    }
  }, [record])

  if (!record) return null

  return (
    <Drawer
      title={
        <Space>
          <Tag color={severityColor[record.severity]}>{record.severity}</Tag>
          <span style={{ fontSize: 14 }}>{record.metric}</span>
        </Space>
      }
      width={640}
      open={!!record}
      onClose={onClose}
      extra={
        <Button size="small" onClick={onRefresh}>
          刷新
        </Button>
      }
    >
      <Descriptions column={2} size="small" bordered style={{ marginBottom: 16 }}>
          <Descriptions.Item label="异常 ID">{record.id}</Descriptions.Item>
          <Descriptions.Item label="状态">
            <Tag color={record.status === 'resolved' ? 'green' : 'red'}>{record.status}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label="指标">{record.metric}</Descriptions.Item>
          <Descriptions.Item label="算法">{record.algorithm}</Descriptions.Item>
          <Descriptions.Item label="资源类型">{record.resource_type || '-'}</Descriptions.Item>
          <Descriptions.Item label="资源名称">{record.resource_name || '-'}</Descriptions.Item>
          <Descriptions.Item label="Namespace">{record.namespace || '-'}</Descriptions.Item>
          <Descriptions.Item label="Cluster">{record.cluster || '-'}</Descriptions.Item>
          <Descriptions.Item label="当前值">{record.value.toFixed(4)}</Descriptions.Item>
          <Descriptions.Item label="基线">
            {record.baseline != null ? record.baseline.toFixed(4) : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="预期上限">
            {record.expected_max != null ? record.expected_max.toFixed(2) : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="异常分">{(record.anomaly_score * 100).toFixed(0)}%</Descriptions.Item>
          <Descriptions.Item label="检测时间" span={2}>
            {new Date(record.timestamp).toLocaleString()}
          </Descriptions.Item>
          {record.incident_id && (
            <Descriptions.Item label="关联事件" span={2}>
              <Tag color="purple">Incident #{record.incident_id}</Tag>
            </Descriptions.Item>
          )}
        </Descriptions>

        {record.reason && (
          <Card title="原因" size="small" style={{ marginBottom: 16 }}>
            <div style={{ color: '#666', fontSize: 13 }}>{record.reason}</div>
          </Card>
        )}

        <Card title="指标趋势" size="small" style={{ marginBottom: 16 }}>
          <div ref={chartRef} style={{ height: 240 }} />
          <div style={{ fontSize: 11, color: '#999', marginTop: 4 }}>
            注：趋势图基于异常记录时间点构造，完整历史数据需接入 Prometheus 查询。
          </div>
        </Card>

        <Card title="关联信息" size="small">
          {record.incident_id ? (
            <div>
              <p>此异常已关联到事件 #{record.incident_id}。</p>
              <p style={{ fontSize: 12, color: '#999' }}>
                可在「事件中心」查看完整事件时间线和关联信号。
              </p>
            </div>
          ) : (
            <Empty description="暂无关联事件" image={Empty.PRESENTED_IMAGE_SIMPLE} />
          )}
        </Card>
    </Drawer>
  )
}
