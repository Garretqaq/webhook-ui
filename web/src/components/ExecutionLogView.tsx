import { useEffect, useRef, useState } from 'react'
import { Alert, Spin, Typography } from 'antd'
import { executionApi } from '../api/client'
import type { Execution, ExecutionLogChunk } from '../api/client'

const { Text } = Typography

const POLL_INTERVAL_MS = 2000

const boxStyle: React.CSSProperties = {
  background: '#1e1e1e',
  color: '#e6e6e6',
  padding: 12,
  borderRadius: 4,
  maxHeight: 420,
  overflow: 'auto',
  fontFamily: 'monospace',
  fontSize: 12,
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-all',
  margin: 0,
}

interface Props {
  execution: Execution
}

/**
 * Streams one execution's output. While the execution is unfinished it polls
 * for chunks past its cursor; once finished it stops.
 *
 * Executions that predate streaming — and remote ones, until the SSH path
 * streams too — have no chunks at all, so the aggregated output on the
 * execution record is used as the fallback.
 */
export default function ExecutionLogView({ execution }: Props) {
  const [chunks, setChunks] = useState<ExecutionLogChunk[]>([])
  const [aggregate, setAggregate] = useState(execution)
  const [finished, setFinished] = useState(!!execution.finished_at)
  const [gap, setGap] = useState(false)
  const [failed, setFailed] = useState(false)
  const cursor = useRef(0)
  const boxRef = useRef<HTMLPreElement>(null)

  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout>

    cursor.current = 0
    setChunks([])
    setAggregate(execution)
    setGap(false)
    setFailed(false)
    setFinished(!!execution.finished_at)

    const poll = async () => {
      try {
        const { data } = await executionApi.logs(execution.id, cursor.current)
        if (cancelled) return

        // oldest_seq above our cursor means the head rolled off before we read
        // it. Say so rather than silently splicing a hole into the output.
        if (cursor.current > 0 && data.oldest_seq > cursor.current + 1) {
          setGap(true)
        }
        if (data.chunks.length > 0) {
          cursor.current = data.next_seq
          setChunks(prev => [...prev, ...data.chunks])
        }
        setFinished(data.finished)

        // A finished execution can still have a backlog: one response is
        // capped, so stopping on `finished` alone would strand the remainder.
        if (data.has_more) {
          timer = setTimeout(poll, 0)
        } else if (!data.finished) {
          timer = setTimeout(poll, POLL_INTERVAL_MS)
        } else if (!execution.finished_at) {
          // It finished while the modal was open, so the record we were handed
          // still has the empty output the list was showing. Refetch it, or the
          // fallback below would render that stale blank.
          const fresh = await executionApi.get(execution.id)
          if (!cancelled) setAggregate(fresh.data)
        }
      } catch {
        if (!cancelled) setFailed(true)
      }
    }

    poll()
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [execution.id])

  useEffect(() => {
    const box = boxRef.current
    if (box) box.scrollTop = box.scrollHeight
  }, [chunks])

  if (failed) {
    return <Alert type="error" showIcon message="日志加载失败" />
  }

  // No streamed chunks: fall back to the aggregate stored on the execution.
  if (chunks.length === 0) {
    if (!finished) {
      return (
        <pre style={boxStyle}>
          <Spin size="small" /> <Text style={{ color: '#8c8c8c' }}>等待输出...</Text>
        </pre>
      )
    }
    return (
      <pre style={boxStyle}>
        {aggregate.output || '(无输出)'}
        {aggregate.error && <span style={{ color: '#ff7875' }}>{'\n' + aggregate.error}</span>}
      </pre>
    )
  }

  return (
    <>
      {gap && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 8 }}
          message="部分早期日志已超出保留上限被丢弃，下方不是完整输出"
        />
      )}
      <pre ref={boxRef} style={boxStyle}>
        {chunks.map(chunk => (
          <span
            key={chunk.seq}
            style={chunk.stream === 'stderr' ? { color: '#ff7875' } : undefined}
          >
            {chunk.text}
          </span>
        ))}
      </pre>
      {!finished && (
        <div style={{ marginTop: 8 }}>
          <Spin size="small" /> <Text type="secondary">执行中，日志持续刷新</Text>
        </div>
      )}
    </>
  )
}
