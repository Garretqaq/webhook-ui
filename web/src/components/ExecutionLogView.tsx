import { useEffect, useState } from 'react'
import { executionApi } from '../api/client'
import type { Execution } from '../api/client'
import LogStreamView from './LogStreamView'

interface Props {
  execution: Execution
}

/**
 * Streams one execution's output.
 *
 * Executions that predate streaming have no chunks at all, so the aggregated
 * output stored on the execution record is used as the fallback. That fallback
 * is what keeps this wrapper: it is specific to executions, which are the only
 * runs with a stored aggregate.
 */
export default function ExecutionLogView({ execution }: Props) {
  const [aggregate, setAggregate] = useState(execution)

  useEffect(() => {
    setAggregate(execution)
  }, [execution])

  return (
    <LogStreamView
      sourceKey={execution.id}
      initiallyFinished={!!execution.finished_at}
      fetchPage={async afterSeq => (await executionApi.logs(execution.id, afterSeq)).data}
      onFinished={async () => {
        // It finished while the modal was open, so the record we were handed
        // still has the empty output the list was showing. Refetch it, or the
        // fallback below would render that stale blank.
        const fresh = await executionApi.get(execution.id)
        setAggregate(fresh.data)
      }}
      renderEmpty={() => (
        <>
          {aggregate.output || '(无输出)'}
          {aggregate.error && <span style={{ color: '#ff7875' }}>{'\n' + aggregate.error}</span>}
        </>
      )}
    />
  )
}
