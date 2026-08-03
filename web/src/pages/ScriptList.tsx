import { useEffect, useState } from 'react'
import { Table, Button, Space, Tag, message, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { scriptApi } from '../api/client'
import type { Script } from '../api/client'

export default function ScriptList() {
  const [scripts, setScripts] = useState<Script[]>([])
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const loadScripts = async () => {
    setLoading(true)
    try {
      const res = await scriptApi.list()
      setScripts(res.data)
    } catch (error) {
      message.error('加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadScripts()
  }, [])

  const handleDelete = async (id: string) => {
    try {
      await scriptApi.delete(id)
      message.success('删除成功')
      loadScripts()
    } catch (error: any) {
      message.error(error.response?.data?.error || '删除失败')
    }
  }

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '解释器',
      dataIndex: 'interpreter',
      key: 'interpreter',
      render: (interpreter: string) => <Tag color="blue">{interpreter}</Tag>,
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      key: 'updated_at',
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: Script) => (
        <Space>
          <Button
            type="text"
            icon={<EditOutlined />}
            onClick={() => navigate(`/scripts/${record.id}`)}
          />
          <Popconfirm
            title="确定删除此脚本?"
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
        <h2>脚本管理</h2>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => navigate('/scripts/new')}
        >
          新建脚本
        </Button>
      </div>
      <Table
        columns={columns}
        dataSource={scripts}
        rowKey="id"
        loading={loading}
      />
    </div>
  )
}
