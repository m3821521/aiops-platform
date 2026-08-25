/**
 * P1-X.9 Data Trustworthiness Foundation
 * P1-X.10 Data Provenance & Timestamp Integrity 扩展
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
 * - dataTimestamp: 数据本身对应时间（后端 meta.provenance.dataTimestamp，不存在时 = unavailable，禁止伪造）
 *
 * P1-X.10 扩展：
 * - fetchAgeSeconds: now - lastSuccessfulAt（API 获取年龄）
 * - dataAgeSeconds: now - dataTimestamp（数据本身年龄，仅当 dataTimestampAvailable=true 时存在）
 * - provenance: 后端返回的 meta.provenance（cacheHit, sourceType, fetchedAt 等）
 */

import { useState, useRef, useCallback, useEffect } from 'react'
import { DATA_FRESHNESS_THRESHOLDS, DATA_SOURCE_LABELS, type DataSourceType } from '../config/freshness'
import type { DataProvenance } from '@/types'

export type DataTrustStatus = 'idle' | 'fetching' | 'fresh' | 'stale' | 'error'

export interface DataTrustState {
  status: DataTrustStatus
  requestedAt?: Date
  receivedAt?: Date
  lastSuccessfulAt?: Date
  /** 数据本身对应时间。后端 meta.provenance.dataTimestamp，不存在时为 undefined */
  dataTimestamp?: Date
  source: string
  sequence: number
  fetchDurationMs?: number
  error?: string
  /** API 获取年龄：now - lastSuccessfulAt（P1-X.10 重命名，原 ageSeconds） */
  fetchAgeSeconds?: number
  /** 数据本身年龄：now - dataTimestamp。仅当 dataTimestampAvailable=true 时存在 */
  dataAgeSeconds?: number
  /** 后端是否返回 dataTimestamp。来自 meta.provenance.timestampAvailable */
  dataTimestampAvailable: boolean
  /** 后端返回的 provenance 元数据（cacheHit, sourceType, fetchedAt 等） */
  provenance?: DataProvenance
  isStale: boolean
}

export interface UseDataTrustOptions {
  source: DataSourceType
  /** 自定义 stale 阈值（ms），不传则使用 DATA_FRESHNESS_THRESHOLDS[source] */
  staleThresholdMs?: number
  /** age ticker 间隔（ms），默认 1000ms。设为 0 禁用自动 ticker */
  ageTickInterval?: number
}

export interface UseDataTrustReturn extends DataTrustState {
  /** 开始一次新请求，返回 sequence number。旧请求的 markSuccess/markError 将被忽略 */
  beginFetch: () => number
  /** 请求成功，更新 receivedAt/lastSuccessfulAt/status=fresh。seq 不匹配则忽略 */
  markSuccess: (seq: number, provenance?: DataProvenance) => void
  /** 请求失败，保留 lastSuccessfulAt，status=stale/error。seq 不匹配则忽略 */
  markError: (seq: number, error: string) => void
  /** 手动设置 stale 状态（如组件挂载时发现数据过旧） */
  markStale: () => void
  /** 格式化 fetch age 为人类可读字符串 */
  formatFetchAge: () => string
  /** 格式化 data age 为人类可读字符串（dataTimestamp 不可用时返回 'Unavailable'） */
  formatDataAge: () => string
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
  const dataTimestampRef = useRef<Date | null>(null)

  const [state, setState] = useState<DataTrustState>({
    status: 'idle',
    source,
    sequence: 0,
    dataTimestampAvailable: false,
    isStale: false,
  })

  // 计算 fetchAgeSeconds: now - lastSuccessfulAt
  const computeFetchAge = useCallback((lastAt: Date | null): number | undefined => {
    if (!lastAt) return undefined
    return Math.floor((Date.now() - lastAt.getTime()) / 1000)
  }, [])

  // 计算 dataAgeSeconds: now - dataTimestamp
  const computeDataAge = useCallback((dataTS: Date | null): number | undefined => {
    if (!dataTS) return undefined
    return Math.floor((Date.now() - dataTS.getTime()) / 1000)
  }, [])

  // 计算 isStale（基于 fetchAge）
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
    }))
    return seq
  }, [])

  const markSuccess = useCallback((seq: number, prov?: DataProvenance) => {
    // Race protection: 旧请求忽略
    if (seq !== sequenceRef.current) return
    const now = new Date()
    lastSuccessfulAtRef.current = now

    // 从 provenance 中提取真实 dataTimestamp（禁止伪造）
    let dataTS: Date | undefined
    let dataTSAvailable = false
    if (prov?.dataTimestamp && prov.timestampAvailable) {
      dataTS = new Date(prov.dataTimestamp)
      dataTimestampRef.current = dataTS
      dataTSAvailable = true
    } else {
      dataTimestampRef.current = null
    }

    setState(prev => ({
      ...prev,
      status: 'fresh',
      receivedAt: now,
      lastSuccessfulAt: now,
      fetchDurationMs: prev.requestedAt ? now.getTime() - prev.requestedAt.getTime() : undefined,
      sequence: seq,
      error: undefined,
      fetchAgeSeconds: 0,
      dataAgeSeconds: dataTSAvailable ? computeDataAge(dataTS!) : undefined,
      dataTimestamp: dataTS,
      dataTimestampAvailable: dataTSAvailable,
      provenance: prov,
      isStale: false,
    }))
  }, [computeDataAge])

  const markError = useCallback((seq: number, error: string) => {
    if (seq !== sequenceRef.current) return
    const hasLastGood = lastSuccessfulAtRef.current !== null
    setState(prev => ({
      ...prev,
      status: hasLastGood ? 'stale' : 'error',
      error,
      lastSuccessfulAt: lastSuccessfulAtRef.current || undefined,
      sequence: seq,
      fetchAgeSeconds: computeFetchAge(lastSuccessfulAtRef.current),
      dataAgeSeconds: computeDataAge(dataTimestampRef.current),
      isStale: true,
    }))
  }, [computeFetchAge, computeDataAge])

  const markStale = useCallback(() => {
    if (lastSuccessfulAtRef.current) {
      setState(prev => ({
        ...prev,
        status: 'stale',
        isStale: true,
        fetchAgeSeconds: computeFetchAge(lastSuccessfulAtRef.current),
        dataAgeSeconds: computeDataAge(dataTimestampRef.current),
      }))
    }
  }, [computeFetchAge, computeDataAge])

  // Age 自动递增 ticker（1s）
  // 只更新 fetchAgeSeconds/dataAgeSeconds 和 isStale，不修改 lastSuccessfulAt/receivedAt/dataTimestamp
  useEffect(() => {
    if (ageTickInterval <= 0) return
    if (state.status === 'idle' || state.status === 'error') return

    const timer = window.setInterval(() => {
      setState(prev => {
        if (!prev.lastSuccessfulAt) return prev
        const fetchAge = computeFetchAge(prev.lastSuccessfulAt)
        const dataAge = prev.dataTimestamp ? computeDataAge(prev.dataTimestamp) : undefined
        const stale = computeStale(fetchAge)
        const newStatus = stale && prev.status === 'fresh' ? 'stale' : prev.status
        return {
          ...prev,
          fetchAgeSeconds: fetchAge,
          dataAgeSeconds: dataAge,
          isStale: stale,
          status: newStatus,
        }
      })
    }, ageTickInterval)

    return () => clearInterval(timer)
  }, [state.status, ageTickInterval, computeFetchAge, computeDataAge, computeStale])

  const formatFetchAge = useCallback((): string => {
    const age = state.fetchAgeSeconds
    if (age === undefined) return 'N/A'
    if (age < 60) return `${age}s`
    const m = Math.floor(age / 60)
    const s = age % 60
    if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`
    const h = Math.floor(m / 60)
    return `${h}h ${m % 60}m`
  }, [state.fetchAgeSeconds])

  const formatDataAge = useCallback((): string => {
    if (!state.dataTimestampAvailable) return 'Unavailable'
    const age = state.dataAgeSeconds
    if (age === undefined) return 'N/A'
    if (age < 60) return `${age}s`
    const m = Math.floor(age / 60)
    const s = age % 60
    if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`
    const h = Math.floor(m / 60)
    return `${h}h ${m % 60}m`
  }, [state.dataTimestampAvailable, state.dataAgeSeconds])

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
    formatFetchAge,
    formatDataAge,
    formatLastSuccessful,
    sourceLabel,
  }
}
