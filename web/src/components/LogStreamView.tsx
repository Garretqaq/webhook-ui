import { useEffect, useRef, useState } from 'react'
import { Alert, Spin, Typography } from 'antd'
import type { ExecutionLogChunk, ExecutionLogs } from '../api/client'

const { Text } = Typography

const POLL_INTERVAL_MS = 2000

export const logBoxStyle: React.CSSProperties = {
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
  /** Restarts the stream from the first chunk whenever it changes. */
  sourceKey: string | number
  /** Fetches one page of chunks past afterSeq. */
  fetchPage: (afterSeq: number) => Promise<ExecutionLogs>
  /** Whether the source had already finished before the first poll. */
  initiallyFinished: boolean
  /** Called once, when a stream that was still running is seen finishing. */
  onFinished?: () => void
  /** Rendered inside the log box when a finished source produced no chunks. */
  renderEmpty: () => React.ReactNode
}

/**
 * Streams one source's output. While it is unfinished the view polls for
 * chunks past its cursor; once finished it stops.
 *
 * The view deliberately does not know where the chunks come from. Executions
 * and script test runs serve the same page shape from different endpoints, and
 * only the caller knows what to show when a finished source has no chunks at
 * all — an execution falls back to its stored aggregate, a test run has none.
 */
export default function LogStreamView({
  sourceKey,
  fetchPage,
  initiallyFinished,
  onFinished,
  renderEmpty,
}: Props) {
  const [chunks, setChunks] = useState<ExecutionLogChunk[]>([])
  const [finished, setFinished] = useState(initiallyFinished)
  const [gap, setGap] = useState(false)
  const [failed, setFailed] = useState(false)
  const cursor = useRef(0)
  const boxRef = useRef<HTMLPreElement>(null)

  // Held in refs so a caller that rebuilds these on every render does not
  // restart the stream underneath itself.
  const fetchRef = useRef(fetchPage)
  fetchRef.current = fetchPage
  const finishedRef = useRef(onFinished)
  finishedRef.current = onFinished
  // Read through a ref as well: it only decides the starting state and whether
  // onFinished still owes a call, so a later flip must not restart the stream.
  const startedFinishedRef = useRef(initiallyFinished)

  useEffect(() => {
    let cancelled = false
    let timer: ReturnType<typeof setTimeout>

    cursor.current = 0
    startedFinishedRef.current = initiallyFinished
    setChunks([])
    setGap(false)
    setFailed(false)
    setFinished(initiallyFinished)

    const poll = async () => {
      try {
        const data = await fetchRef.current(cursor.current)
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

        // A finished source can still have a backlog: one response is capped,
        // so stopping on `finished` alone would strand the remainder.
        if (data.has_more) {
          timer = setTimeout(poll, 0)
        } else if (!data.finished) {
          timer = setTimeout(poll, POLL_INTERVAL_MS)
        } else if (!startedFinishedRef.current) {
          finishedRef.current?.()
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
    // Only sourceKey restarts the stream; every other prop is read through a
    // ref so a re-rendering caller cannot rewind the log it is watching.
  }, [sourceKey])

  useEffect(() => {
    const box = boxRef.current
    if (box) box.scrollTop = box.scrollHeight
  }, [chunks])

  if (failed) {
    return <Alert type="error" showIcon message="日志加载失败" />
  }

  if (chunks.length === 0) {
    if (!finished) {
      return (
        <pre style={logBoxStyle}>
          <Spin size="small" /> <Text style={{ color: '#8c8c8c' }}>等待输出...</Text>
        </pre>
      )
    }
    return <pre style={logBoxStyle}>{renderEmpty()}</pre>
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
      <pre ref={boxRef} style={logBoxStyle}>
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
