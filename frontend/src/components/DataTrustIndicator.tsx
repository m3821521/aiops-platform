/**
 * P1-X.9 Data Trustworthiness Foundation
 * 统一 Data Trust Indicator UI 组件
 *
 * 显示状态：
 * - Fresh: ● Live + Last successful fetch + Data age + Source
 * - Fetching: ↻ Updating... + Last successful fetch + Data age
 * - Stale: ⚠ Stale + Showing last successful data + Last successful fetch + Data age + Source
 * - Error (无历史数据): ⚠ Update failed + No successful data available + Source + Error
 * - Error (有历史数据): 同 Stale，但额外显示 Error
 *
 * 严格禁止：
 * - Fetching 状态显示 Live
 * - Error/Stale 状态显示 Live
 * - 伪造 dataTimestamp
 */

import { Tooltip, Tag, Space, Typography } from 'antd'
import {
  CheckCircleOutlined,
  LoadingOutlined,
  WarningOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons'
import type { DataTrustStatus } from '../hooks/useDataTrust'

const { Text } = Typography

export interface DataTrustIndicatorProps {
  status: DataTrustStatus
  lastSuccessfulAt?: Date
  ageSeconds?: number
  sourceLabel: string
  error?: string
  /** 自定义 formatAge 函数 */
  formatAge?: () => string
  /** 自定义 formatLastSuccessful 函数 */
  formatLastSuccessful?: () => string
  /** 紧凑模式（只显示状态点 + age） */
  compact?: boolean
  /** 显示数据源标签 */
  showSource?: boolean
  style?: React.CSSProperties
}

export function DataTrustIndicator({
  status,
  lastSuccessfulAt,
  ageSeconds,
  sourceLabel,
  error,
  formatAge,
  formatLastSuccessful,
  compact = false,
  showSource = true,
  style,
}: DataTrustIndicatorProps) {
  const ageText = formatAge ? formatAge() : (ageSeconds !== undefined ? `${ageSeconds}s` : 'N/A')
  const lastText = formatLastSuccessful
    ? formatLastSuccessful()
    : lastSuccessfulAt
      ? lastSuccessfulAt.toLocaleTimeString('zh-CN', { hour12: false })
      : 'Never'

  // 状态颜色和图标
  const statusConfig = {
    idle: { color: 'default', icon: null, label: 'Idle' },
    fetching: { color: 'processing', icon: <LoadingOutlined />, label: 'Updating' },
    fresh: { color: 'success', icon: <CheckCircleOutlined />, label: 'Live' },
    stale: { color: 'warning', icon: <WarningOutlined />, label: 'Stale' },
    error: { color: 'error', icon: <CloseCircleOutlined />, label: 'Error' },
  }[status]

  if (compact) {
    return (
      <Tag color={statusConfig.color} icon={statusConfig.icon} style={style}>
        {statusConfig.label}
        {status !== 'idle' && status !== 'error' && ` · ${ageText}`}
      </Tag>
    )
  }

  const hasData = lastSuccessfulAt !== undefined

  return (
    <div style={{ ...style, fontSize: 12 }}>
      <Space size={4} wrap>
        <Tag color={statusConfig.color} icon={statusConfig.icon}>
          {status === 'fresh' && '● Live'}
          {status === 'fetching' && '↻ Updating...'}
          {status === 'stale' && '⚠ Stale'}
          {status === 'error' && '⚠ Update failed'}
          {status === 'idle' && 'Idle'}
        </Tag>

        {status === 'stale' && hasData && (
          <Text type="secondary" style={{ fontSize: 12 }}>
            Showing last successful data
          </Text>
        )}

        {status === 'error' && !hasData && (
          <Text type="danger" style={{ fontSize: 12 }}>
            No successful data available
          </Text>
        )}
      </Space>

      <div style={{ marginTop: 2, color: '#8c8c8c' }}>
        {hasData && (
          <span>
            Last successful fetch: <strong style={{ color: '#595959' }}>{lastText}</strong>
            {' · '}
            Data age: <strong style={{ color: '#595959' }}>{ageText}</strong>
          </span>
        )}
        {showSource && (
          <span>
            {hasData && ' · '}
            Source: <strong style={{ color: '#595959' }}>{sourceLabel}</strong>
          </span>
        )}
      </div>

      {error && (
        <div style={{ marginTop: 2 }}>
          <Tooltip title={error}>
            <Text type="danger" style={{ fontSize: 12, cursor: 'help' }}>
              Error: {error.length > 60 ? error.slice(0, 60) + '...' : error}
            </Text>
          </Tooltip>
        </div>
      )}
    </div>
  )
}

export default DataTrustIndicator
