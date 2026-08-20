import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { Input, List, Avatar, Tag, Empty, Spin } from 'antd'
import {
  SearchOutlined, AlertOutlined, CloudOutlined, ApartmentOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import request from '@/api/client'

interface SearchResult {
  type: string
  id: string
  title: string
  subtitle: string
  severity?: string
  status?: string
  namespace?: string
  url: string
}

const typeIconMap: Record<string, any> = {
  incident: AlertOutlined,
  alert: WarningOutlined,
  pod: CloudOutlined,
  node: ApartmentOutlined,
  deployment: ApartmentOutlined,
}

const typeColorMap: Record<string, string> = {
  incident: 'red',
  alert: 'orange',
  pod: 'blue',
  node: 'green',
  deployment: 'purple',
}

const typeLabelMap: Record<string, string> = {
  incident: '事件',
  alert: '告警',
  pod: 'Pod',
  node: '节点',
  deployment: '部署',
}

export default function CommandPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SearchResult[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedIndex, setSelectedIndex] = useState(0)
  const inputRef = useRef<any>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>()

  useEffect(() => {
    if (open) {
      setQuery('')
      setResults([])
      setSelectedIndex(0)
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [open])

  useEffect(() => {
    if (!query.trim()) {
      setResults([])
      return
    }
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(async () => {
      setLoading(true)
      try {
        const res = await request.get<any, { results: SearchResult[]; total: number }>(
          '/api/v1/search',
          { params: { q: query } },
        )
        setResults(res.results || [])
        setSelectedIndex(0)
      } catch (e) {
        setResults([])
      } finally {
        setLoading(false)
      }
    }, 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelectedIndex((i) => Math.min(i + 1, results.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelectedIndex((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter' && results[selectedIndex]) {
      navigate(results[selectedIndex].url)
      onClose()
    }
  }

  if (!open) return null

  return (
    <div
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        background: 'rgba(0,0,0,0.5)',
        zIndex: 9999,
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'flex-start',
        paddingTop: '15vh',
      }}
      onClick={onClose}
    >
      <div
        style={{
          width: '100%',
          maxWidth: 600,
          background: 'var(--bg-surface)',
          borderRadius: 12,
          boxShadow: '0 20px 60px rgba(0,0,0,0.3)',
          overflow: 'hidden',
          border: '1px solid var(--border-color)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--border-color)' }}>
          <Input
            ref={inputRef}
            prefix={<SearchOutlined style={{ color: 'var(--text-muted)' }} />}
            placeholder="搜索 Incident、告警、Pod、服务..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            bordered={false}
            size="large"
            style={{ fontSize: 15 }}
            suffix={
              <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                ESC 关闭
              </span>
            }
          />
        </div>

        <div style={{ maxHeight: 400, overflow: 'auto' }}>
          {loading ? (
            <div style={{ padding: 40, textAlign: 'center' }}>
              <Spin />
            </div>
          ) : results.length === 0 ? (
            <Empty
              description={query ? '未找到相关结果' : '输入关键词开始搜索'}
              style={{ padding: 40 }}
            />
          ) : (
            <List
              dataSource={results}
              renderItem={(item, index) => {
                const Icon = typeIconMap[item.type] || SearchOutlined
                return (
                  <List.Item
                    onClick={() => {
                      navigate(item.url)
                      onClose()
                    }}
                    style={{
                      cursor: 'pointer',
                      padding: '10px 16px',
                      background: index === selectedIndex ? 'var(--color-primary-light)' : 'transparent',
                      borderBottom: '1px solid var(--border-light)',
                    }}
                    onMouseEnter={() => setSelectedIndex(index)}
                  >
                    <List.Item.Meta
                      avatar={
                        <Avatar
                          size="small"
                          icon={<Icon />}
                          style={{ background: `var(--color-${typeColorMap[item.type] || 'primary'})` }}
                        />
                      }
                      title={
                        <span style={{ fontSize: 14 }}>
                          {item.title}
                          <Tag color={typeColorMap[item.type]} style={{ marginLeft: 8, fontSize: 11 }}>
                            {typeLabelMap[item.type] || item.type}
                          </Tag>
                          {item.severity && (
                            <Tag color={item.severity === 'critical' ? 'red' : item.severity === 'warning' ? 'orange' : 'blue'} style={{ fontSize: 11 }}>
                              {item.severity}
                            </Tag>
                          )}
                        </span>
                      }
                      description={
                        <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                          {item.subtitle}
                          {item.status && ` · ${item.status}`}
                        </span>
                      }
                    />
                  </List.Item>
                )
              }}
            />
          )}
        </div>

        <div style={{ padding: '8px 16px', borderTop: '1px solid var(--border-color)', fontSize: 11, color: 'var(--text-muted)', display: 'flex', justifyContent: 'space-between' }}>
          <span>↑↓ 选择 · Enter 打开 · ESC 关闭</span>
          <span>⌘K / Ctrl+K</span>
        </div>
      </div>
    </div>
  )
}
