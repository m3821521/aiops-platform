/**
 * P1-X.9 Data Trustworthiness Foundation
 * 统一数据新鲜度阈值配置
 *
 * 原则：
 * - Fresh: age <= threshold
 * - Stale: age > threshold
 * - 不同数据源根据真实 polling interval 和数据特性调整
 */

export const DATA_FRESHNESS_THRESHOLDS = {
  /** Kubernetes API：15s polling，阈值 15s */
  kubernetes: 15000,
  /** Prometheus API：30s polling，阈值 30s */
  prometheus: 30000,
  /** Alertmanager API：30s polling，阈值 30s */
  alertmanager: 30000,
  /** MySQL 业务状态（Incident/Action/Workflow）：30s polling，阈值 30s */
  mysql: 30000,
  /** Elasticsearch：30s polling，阈值 30s */
  elasticsearch: 30000,
  /** Topology：30s polling + 60s Redis cache，阈值 60s */
  topology: 60000,
  /** External Connection：30s GET list + 5min backend HealthChecker，阈值 5min */
  connection: 300000,
  /** Dashboard 多数据源聚合：30s polling，阈值 30s */
  dashboard: 30000,
} as const

export type DataSourceType = keyof typeof DATA_FRESHNESS_THRESHOLDS

/** 数据源显示名称（集中定义，不散落硬编码） */
export const DATA_SOURCE_LABELS: Record<DataSourceType, string> = {
  kubernetes: 'Kubernetes API',
  prometheus: 'Prometheus API',
  alertmanager: 'Alertmanager API',
  mysql: 'MySQL API',
  elasticsearch: 'Elasticsearch API',
  topology: 'Kubernetes + Prometheus',
  connection: 'Connection API',
  dashboard: 'Multiple Sources',
}
