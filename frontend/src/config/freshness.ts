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
  /** Topology：30s frontend polling + 60s Redis cache（后端缓存 K8s+Prometheus 聚合结果）。
   *  阈值 60s = cache TTL。注意：Data Trust 测量的是 API 获取成功时间，
   *  不保证 underlying source 数据生成时间（可能来自 Redis cache）。 */
  topology: 60000,
  /** External Connection：30s GET list + 5min backend HealthChecker，阈值 5min */
  connection: 300000,
  /** Dashboard 多数据源聚合：30s polling，阈值 30s */
  dashboard: 30000,
} as const

export type DataSourceType = keyof typeof DATA_FRESHNESS_THRESHOLDS

/** 数据源显示名称（集中定义，不散落硬编码）。
 *  注意：source label 表示前端调用的 API/Provider 标识，
 *  不保证数据直接来自底层数据源（如 topology 可能经过 Redis cache）。 */
export const DATA_SOURCE_LABELS: Record<DataSourceType, string> = {
  kubernetes: 'Kubernetes API',
  prometheus: 'Prometheus API',
  alertmanager: 'Alertmanager API',
  mysql: 'MySQL API',
  elasticsearch: 'Elasticsearch API',
  topology: 'Topology API (Redis cache)',
  connection: 'Connection API',
  dashboard: 'Multiple Sources',
}
