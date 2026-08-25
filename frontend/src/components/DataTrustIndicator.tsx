/**
 * P1-X.9 Data Trustworthiness Foundation
 * P1-X.10 Data Provenance & Timestamp Integrity 扩展
 * 统一 Data Trust Indicator UI 组件
 *
 * 显示状态：
 * - Fresh: ● Live + Last successful fetch + Fetch age + [Data age] + Source
 * - Fetching: ↻ Updating... + Last successful fetch + Fetch age
 * - Stale: ⚠ Stale + Showing last successful data + Last successful fetch + Fetch age + Source
 * - Error (无历史数据): ⚠ Update failed + No successful data available + Source + Error
 * - Error (有历史数据): 同 Stale，但额外显示 Error
 *
 * P1-X.10 关键修正：
 * - Fetch age (API 获取年龄) 与 Data age (数据本身年龄) 严格分离
 * - dataTimestamp 不可用时显示 "Data timestamp: Unavailable"，不伪造 Data age
 * - cacheHit=true 时显示 "Cached" 而非 "Live source"
 *
 * 严格禁止：
 * - Fetching 状态显示 Live
 * - Error/Stale 状态显示 Live
 * - 伪造 dataTimestamp
 * - 将 Fetch age 称为 Data age
 */

import { Tooltip, Tag, Space, Typography } from 'antd'
import {
  CheckCircleOutlined,
  LoadingOutlined,
  WarningOutlined,
  CloseCircleOutlined,
  DatabaseOutlined,
} from '@ant-design/icons'
import type { DataTrustStatus } from '../hooks/useDataTrust'
import type { DataProvenance } from '@/types'

const { Text } = Typography

export interface DataTrustIndicatorProps {
  status: DataTrustStatus
  lastSuccessfulAt?: Date
  /** P1-X.10: API 获取年龄（now - lastSuccessfulAt），原 ageSeconds */
  fetchAgeSeconds?: number
  /** P1-X.10: 数据本身年龄（now - dataTimestamp），仅当 dataTimestampAvailable=true 时存在 */
  dataAgeSeconds?: number
  /** P1-X.10: 后端是否返回 dataTimestamp */
  dataTimestampAvailable?: boolean
  /** P1-X.10: 后端返回的 provenance（cacheHit, sourceType 等） */
  provenance?: DataProvenance
  sourceLabel: string
  error?: string
  /** 自定义 formatFetchAge 函数（P1-X.10 重命名，原 formatAge） */
  formatFetchAge?: () => string
  /** 自定义 formatDataAge 函数 */
  formatDataAge?: () => string
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
  fetchAgeSeconds,
  dataAgeSeconds,
  dataTimestampAvailable = false,
  provenance,
  sourceLabel,
  error,
  formatFetchAge,
  formatDataAge,
  formatLastSuccessful,
  compact = false,
  showSource = true,
  style,
}: DataTrustIndicatorProps) {
  const fetchAgeText = formatFetchAge
    ? formatFetchAge()
    : (fetchAgeSeconds !== undefined ? `${fetchAgeSeconds}s` : 'N/A')
  const dataAgeText = formatDataAge
    ? formatDataAge()
    : (dataTimestampAvailable && dataAgeSeconds !== undefined ? `${dataAgeSeconds}s` : 'Unavailable')
  const lastText = formatLastSuccessful
    ? formatLastSuccessful()
    : lastSuccessfulAt
      ? lastSuccessfulAt.toLocaleTimeString('zh-CN', { hour12: false })
      : 'Never'

  const isCached = provenance?.cacheHit === true

  // 状态颜色和图标
  const statusConfig = {
    idle: { color: 'default', icon: null, label: 'Idle' },
    fetching: { color: 'processing', icon: <LoadingOutlined />, label: 'Updating' },
    fresh: { color: 'success', icon: isCached ? <DatabaseOutlined /> : <CheckCircleOutlined />, label: isCached ? 'Cached' : 'Live' },
    stale: { color: 'warning', icon: <WarningOutlined />, label: 'Stale' },
    error: { color: 'error', icon: <CloseCircleOutlined />, label: 'Error' },
  }[status]

  if (compact) {
    return (
      <Tag color={statusConfig.color} icon={statusConfig.icon} style={style}>
        {statusConfig.label}
        {status !== 'idle' && status !== 'error' && ` · ${fetchAgeText}`}
      </Tag>
    )
  }

  const hasData = lastSuccessfulAt !== undefined

  return (
    <div style={{ ...style, fontSize: 12 }}>
      <Space size={4} wrap>
        <Tag color={statusConfig.color} icon={statusConfig.icon}>
          {status === 'fresh' && (isCached ? '🗄 Cached' : '● Live')}
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

        {isCached && status === 'fresh' && (
          <Text type="secondary" style={{ fontSize: 12 }}>
            Source: Redis cache
          </Text>
        )}
      </Space>

      <div style={{ marginTop: 2, color: '#8c8c8c' }}>
        {hasData && (
          <span>
            Last successful fetch: <strong style={{ color: '#595959' }}>{lastText}</strong>
            {' · '}
            Fetch age: <strong style={{ color: '#595959' }}>{fetchAgeText}</strong>
          </span>
        )}
        {hasData && dataTimestampAvailable && (
          <span>
            {' · '}
            Data age: <strong style={{ color: '#595959' }}>{dataAgeText}</strong>
          </span>
        )}
        {hasData && !dataTimestampAvailable && (
          <span>
            {' · '}
            Data timestamp: <span style={{ color: '#8c8c8c' }}>Unavailable</span>
          </span>
        )}
        {showSource && !isCached && (
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
