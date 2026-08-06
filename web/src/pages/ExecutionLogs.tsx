import { useEffect, useState } from 'react'
import { Table, Tag, Button, Modal, Typography, Select, Space } from 'antd'
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
}

export default function ExecutionLogs() {
  const [executions, setExecutions] = useState<Execution[]>([])
  const [hooks, setHooks] = useState<Hook[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedHook, setSelectedHook] = useState<string>()
  const [detailVisible, setDetailVisible] = useState(false)
  const [currentExecution, setCurrentExecution] = useState<Execution>()

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
  }

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
        <Button type="link" onClick={() => showDetail(record)}>
          详情
        </Button>
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
            </Paragraph>
            <ExecutionLogView execution={currentExecution} />
          </div>
        )}
      </Modal>
    </div>
  )
}
