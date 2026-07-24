import { useEffect, useState } from 'react'
import { Table, Button, Space, Tag, message, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, CopyOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { hookApi } from '../api/client'
import type { Hook } from '../api/client'

export default function HookList() {
  const [hooks, setHooks] = useState<Hook[]>([])
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const loadHooks = async () => {
    setLoading(true)
    try {
      const res = await hookApi.list()
      setHooks(res.data)
    } catch (error) {
      message.error('加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadHooks()
  }, [])

  const handleDelete = async (id: string) => {
    try {
      await hookApi.delete(id)
      message.success('删除成功')
      loadHooks()
    } catch (error) {
      message.error('删除失败')
    }
  }

  const copyWebhookUrl = (id: string) => {
    const url = `${window.location.origin}/hooks/${id}`
    navigator.clipboard.writeText(url)
    message.success('Webhook URL 已复制')
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      render: (id: string) => <code>{id}</code>,
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '命令',
      dataIndex: 'command',
      key: 'command',
      ellipsis: true,
    },
    {
      title: 'HMAC',
      key: 'hmac',
      render: (_: any, record: Hook) => (
        record.hmac_secret ? <Tag color="green">启用</Tag> : <Tag>未启用</Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Hook) => (
        <Space>
          <Button
            type="text"
            icon={<CopyOutlined />}
            onClick={() => copyWebhookUrl(record.id)}
            title="复制 Webhook URL"
          />
          <Button
            type="text"
            icon={<EditOutlined />}
            onClick={() => navigate(`/hooks/${record.id}`)}
          />
          <Popconfirm
            title="确定删除此 Hook?"
            onConfirm={() => handleDelete(record.id)}
          >
            <Button type="text" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
        <h2>Webhook 管理</h2>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => navigate('/hooks/new')}
        >
          新建 Hook
        </Button>
      </div>
      <Table
        columns={columns}
        dataSource={hooks}
        rowKey="id"
        loading={loading}
      />
    </div>
  )
}
