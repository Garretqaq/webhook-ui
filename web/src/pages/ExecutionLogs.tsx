import { useEffect, useState } from 'react'
import { Table, Tag, Button, Modal, Typography, Select, Space, Popconfirm, message, Alert } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { executionApi, hookApi } from '../api/client'
import type { Execution, Hook } from '../api/client'
import ExecutionLogView from '../components/ExecutionLogView'

const { Paragraph, Text } = Typography

const statusColors: Record<string, string> = {
  success: 'green',
  failed: 'red',
  running: 'blue',
  queued: 'gold',
  interrupted: 'orange',
  canceled: 'default',
}

// Only these can still be stopped; everything else has already finished.
const cancellable = (status: string) => status === 'running' || status === 'queued'

export default function ExecutionLogs() {
  const [executions, setExecutions] = useState<Execution[]>([])
  const [hooks, setHooks] = useState<Hook[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedHook, setSelectedHook] = useState<string>()
  const [detailVisible, setDetailVisible] = useState(false)
  const [currentExecution, setCurrentExecution] = useState<Execution>()
  const [detailError, setDetailError] = useState(false)

  const loadData = async () => {
    setLoading(true)
    try {
      const [execRes, hookRes] = await Promise.all([
        executionApi.list({ limit: 100, hook_id: selectedHook }),
        hookApi.list(),
      ])
      setExecutions(execRes.data)
      setHooks(hookRes.data)
    } catch (error) {
      console.error(error)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [selectedHook])

  const getHookName = (hookId: string) => {
    const hook = hooks.find(h => h.id === hookId)
    return hook ? hook.name : hookId
  }

  const showDetail = (record: Execution) => {
    setCurrentExecution(record)
    setDetailVisible(true)
    setDetailError(false)
    // The list row carries no output (the payload would be megabytes), so the
    // full record is fetched separately to fill the log view's fallback. A
    // stale response — the user closed the modal or opened another row before
    // it landed — must not overwrite whichever record is showing now.
    executionApi
      .get(record.id)
      .then(res => {
        if (currentExecution?.id === res.data.id) setCurrentExecution(res.data)
      })
      .catch(() => {
        if (currentExecution?.id === record.id) setDetailError(true)
      })
  }

  const cancelExecution = async (id: number) => {
    try {
      await executionApi.cancel(id)
      message.success('已请求中断')
      loadData()
      // The open modal holds its own copy of the record, so without this its
      // button would survive the cancel and 409 on the next click.
      if (currentExecution?.id === id) {
        const fresh = await executionApi.get(id)
        setCurrentExecution(fresh.data)
      }
    } catch (error: any) {
      message.error(error.response?.data?.error || '中断失败')
    }
  }

  const cancelButton = (record: Execution) =>
    cancellable(record.status) ? (
      <Popconfirm
        title="确定中断这次执行?"
        description="本地执行会连同子进程一起终止；已脱离 SSH 会话的远端进程无法中断"
        onConfirm={() => cancelExecution(record.id)}
      >
        <Button danger size="small">中断</Button>
      </Popconfirm>
    ) : null

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
    },
    {
      title: 'Hook',
      dataIndex: 'hook_id',
      key: 'hook_id',
      render: (hookId: string) => (
        <span>
          {getHookName(hookId)}
          <Text type="secondary" style={{ marginLeft: 8 }}>({hookId})</Text>
        </span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => <Tag color={statusColors[status] || 'default'}>{status}</Tag>,
    },
    {
      title: '执行位置',
      dataIndex: 'exec_target',
      key: 'exec_target',
      render: (target: string) =>
        target === 'local'
          ? <Tag>本地</Tag>
          : target
            ? <Tag color="purple">{target}</Tag>
            : '-',
    },
    {
      title: '来源',
      dataIndex: 'trigger_source',
      key: 'trigger_source',
    },
    {
      title: '开始时间',
      dataIndex: 'started_at',
      key: 'started_at',
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: '耗时',
      key: 'duration',
      render: (_: any, record: Execution) => {
        if (!record.finished_at) return '-'
        const start = new Date(record.started_at).getTime()
        const end = new Date(record.finished_at).getTime()
        return `${((end - start) / 1000).toFixed(2)}s`
      },
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Execution) => (
        <Space>
          <Button type="link" onClick={() => showDetail(record)}>
            详情
          </Button>
          {cancelButton(record)}
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
        <h2>执行日志</h2>
        <Space>
          <Select
            placeholder="筛选 Hook"
            allowClear
            style={{ width: 200 }}
            onChange={setSelectedHook}
            options={hooks.map(h => ({ value: h.id, label: h.name }))}
          />
          <Button icon={<ReloadOutlined />} onClick={loadData}>
            刷新
          </Button>
        </Space>
      </div>

      <Table
        columns={columns}
        dataSource={executions}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 20 }}
      />

      <Modal
        title="执行详情"
        width={760}
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        destroyOnClose
      >
        {currentExecution && (
          <div>
            <Paragraph>
              <Text strong>Hook: </Text>
              {getHookName(currentExecution.hook_id)}
            </Paragraph>
            <Paragraph>
              <Text strong>状态: </Text>
              <Tag color={statusColors[currentExecution.status] || 'default'}>
                {currentExecution.status}
              </Tag>
            </Paragraph>
            <Paragraph>
              <Text strong>执行位置: </Text>
              {currentExecution.exec_target || '-'}
            </Paragraph>
            <Paragraph>
              <Text strong>来源: </Text>
              {currentExecution.trigger_source}
            </Paragraph>
            <Paragraph>
              <Text strong>开始时间: </Text>
              {new Date(currentExecution.started_at).toLocaleString()}
            </Paragraph>
            {currentExecution.finished_at && (
              <Paragraph>
                <Text strong>结束时间: </Text>
                {new Date(currentExecution.finished_at).toLocaleString()}
              </Paragraph>
            )}
            <Paragraph>
              <Text strong>输出: </Text>
              <span style={{ float: 'right' }}>{cancelButton(currentExecution)}</span>
            </Paragraph>
            {detailError ? (
              <Alert type="error" showIcon message="输出加载失败，请重试" />
            ) : (
              <ExecutionLogView execution={currentExecution} />
            )}
          </div>
        )}
      </Modal>
    </div>
  )
}
