/**
 * P1-X.9 Data Trustworthiness Foundation
 * 统一 Data Trust Hook
 *
 * 核心原则：
 * - Every realtime data view must be traceable to the latest successful acquisition.
 * - 只有最近一次成功 API 响应才能更新 lastSuccessfulAt。
 * - API 失败时保留 Last Good Data，标记 stale/error。
 * - 旧请求（sequence 不匹配）不得覆盖新请求数据。
 *
 * 三个时间严格区分：
 * - requestedAt: 请求发出时间
 * - receivedAt: 前端收到成功响应时间
 * - dataTimestamp: 数据本身对应时间（后端不返回时 = unavailable，禁止伪造）
 */

import { useState, useRef, useCallback, useEffect } from 'react'
import { DATA_FRESHNESS_THRESHOLDS, DATA_SOURCE_LABELS, type DataSourceType } from '../config/freshness'

export type DataTrustStatus = 'idle' | 'fetching' | 'fresh' | 'stale' | 'error'

export interface DataTrustState {
  status: DataTrustStatus
  requestedAt?: Date
  receivedAt?: Date
  lastSuccessfulAt?: Date
  /** 数据本身对应时间。后端不返回时为 undefined，dataTimestampAvailable=false */
  dataTimestamp?: Date
  source: string
  sequence: number
  fetchDurationMs?: number
  error?: string
  ageSeconds?: number
  /** 后端是否返回 dataTimestamp。当前项目后端不返回，始终为 false */
  dataTimestampAvailable: boolean
  isStale: boolean
}

export interface UseDataTrustOptions {
  source: DataSourceType
  /** 自定义 stale 阈值（ms），不传则使用 DATA_FRESHNESS_THRESHOLDS[source] */
  staleThresholdMs?: number
  /** dataAge ticker 间隔（ms），默认 1000ms。设为 0 禁用自动 ticker */
  ageTickInterval?: number
}

export interface UseDataTrustReturn extends DataTrustState {
  /** 开始一次新请求，返回 sequence number。旧请求的 markSuccess/markError 将被忽略 */
  beginFetch: () => number
  /** 请求成功，更新 receivedAt/lastSuccessfulAt/status=fresh。seq 不匹配则忽略 */
  markSuccess: (seq: number) => void
  /** 请求失败，保留 lastSuccessfulAt，status=stale/error。seq 不匹配则忽略 */
  markError: (seq: number, error: string) => void
  /** 手动设置 stale 状态（如组件挂载时发现数据过旧） */
  markStale: () => void
  /** 格式化 data age 为人类可读字符串 */
  formatAge: () => string
  /** 格式化 lastSuccessfulAt 为 HH:MM:SS */
  formatLastSuccessful: () => string
  /** source 显示名称 */
  sourceLabel: string
}

export function useDataTrust(options: UseDataTrustOptions): UseDataTrustReturn {
  const { source, staleThresholdMs, ageTickInterval = 1000 } = options
  const threshold = staleThresholdMs ?? DATA_FRESHNESS_THRESHOLDS[source] ?? 30000
  const sourceLabel = DATA_SOURCE_LABELS[source] ?? source

  const sequenceRef = useRef(0)
  const lastSuccessfulAtRef = useRef<Date | null>(null)

  const [state, setState] = useState<DataTrustState>({
    status: 'idle',
    source,
    sequence: 0,
    dataTimestampAvailable: false,
    isStale: false,
  })

  // 计算 ageSeconds
  const computeAge = useCallback((lastAt: Date | null): number | undefined => {
    if (!lastAt) return undefined
    return Math.floor((Date.now() - lastAt.getTime()) / 1000)
  }, [])

  // 计算 isStale
  const computeStale = useCallback((age: number | undefined): boolean => {
    if (age === undefined) return false
    return age * 1000 > threshold
  }, [threshold])

  const beginFetch = useCallback((): number => {
    const seq = ++sequenceRef.current
    setState(prev => ({
      ...prev,
      status: 'fetching',
      requestedAt: new Date(),
      sequence: seq,
      error: undefined,
      // fetching 时保留 lastSuccessfulAt 和 age
    }))
    return seq
  }, [])

  const markSuccess = useCallback((seq: number) => {
    // Race protection: 旧请求忽略
    if (seq !== sequenceRef.current) return
    const now = new Date()
    lastSuccessfulAtRef.current = now
    setState(prev => ({
      ...prev,
      status: 'fresh',
      receivedAt: now,
      lastSuccessfulAt: now,
      fetchDurationMs: prev.requestedAt ? now.getTime() - prev.requestedAt.getTime() : undefined,
      sequence: seq,
      error: undefined,
      ageSeconds: 0,
      isStale: false,
      // dataTimestamp 始终不设置（后端不返回），dataTimestampAvailable=false
    }))
  }, [])

  const markError = useCallback((seq: number, error: string) => {
    if (seq !== sequenceRef.current) return
    const hasLastGood = lastSuccessfulAtRef.current !== null
    setState(prev => ({
      ...prev,
      // 有历史成功数据 → stale（保留旧数据），无 → error
      status: hasLastGood ? 'stale' : 'error',
      error,
      lastSuccessfulAt: lastSuccessfulAtRef.current || undefined,
      sequence: seq,
      // receivedAt 不更新（失败不是成功接收）
      ageSeconds: computeAge(lastSuccessfulAtRef.current),
      isStale: true,
    }))
  }, [computeAge])

  const markStale = useCallback(() => {
    if (lastSuccessfulAtRef.current) {
      setState(prev => ({
        ...prev,
        status: 'stale',
        isStale: true,
        ageSeconds: computeAge(lastSuccessfulAtRef.current),
      }))
    }
  }, [computeAge])

  // Data Age 自动递增 ticker（1s）
  // 只更新 ageSeconds 和 isStale，不修改 lastSuccessfulAt/receivedAt
  useEffect(() => {
    if (ageTickInterval <= 0) return
    // 只有在有成功数据时才需要 ticker
    if (state.status === 'idle' || state.status === 'error') return

    const timer = window.setInterval(() => {
      setState(prev => {
        if (!prev.lastSuccessfulAt) return prev
        const age = computeAge(prev.lastSuccessfulAt)
        const stale = computeStale(age)
        // 如果当前是 fresh 但 age 超过阈值，自动转为 stale
        const newStatus = stale && prev.status === 'fresh' ? 'stale' : prev.status
        return {
          ...prev,
          ageSeconds: age,
          isStale: stale,
          status: newStatus,
        }
      })
    }, ageTickInterval)

    return () => clearInterval(timer)
  }, [state.status, ageTickInterval, computeAge, computeStale])

  const formatAge = useCallback((): string => {
    const age = state.ageSeconds
    if (age === undefined) return 'N/A'
    if (age < 60) return `${age}s`
    const m = Math.floor(age / 60)
    const s = age % 60
    if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`
    const h = Math.floor(m / 60)
    return `${h}h ${m % 60}m`
  }, [state.ageSeconds])

  const formatLastSuccessful = useCallback((): string => {
    if (!state.lastSuccessfulAt) return 'Never'
    return state.lastSuccessfulAt.toLocaleTimeString('zh-CN', { hour12: false })
  }, [state.lastSuccessfulAt])

  return {
    ...state,
    beginFetch,
    markSuccess,
    markError,
    markStale,
    formatAge,
    formatLastSuccessful,
    sourceLabel,
  }
}
