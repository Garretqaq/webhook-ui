import { useEffect, useState } from 'react'
import { Table, Button, Space, Tag, message, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { sshHostApi } from '../api/client'
import type { SSHHost } from '../api/client'

export default function SSHHostList() {
  const [hosts, setHosts] = useState<SSHHost[]>([])
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const loadHosts = async () => {
    setLoading(true)
    try {
      const res = await sshHostApi.list()
      setHosts(res.data)
    } catch (error) {
      message.error('加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadHosts()
  }, [])

  const handleDelete = async (id: string) => {
    try {
      await sshHostApi.delete(id)
      message.success('删除成功')
      loadHosts()
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
      title: '地址',
      key: 'addr',
      render: (_: any, r: SSHHost) => <code>{r.user}@{r.host}:{r.port}</code>,
    },
    {
      title: '系统',
      dataIndex: 'target_os',
      key: 'target_os',
      render: (os: string) => os === 'windows' ? <Tag color="geekblue">Windows</Tag> : <Tag>Linux</Tag>,
    },
    {
      title: '认证',
      dataIndex: 'auth_type',
      key: 'auth_type',
      render: (t: string) => t === 'key' ? <Tag color="blue">私钥</Tag> : <Tag>密码</Tag>,
    },
    {
      title: 'Host Key',
      dataIndex: 'host_key',
      key: 'host_key',
      render: (k: string) => k ? <Tag color="green">已固定</Tag> : <Tag color="orange">首次连接时记录</Tag>,
    },
    {
      title: '操作',
      key: 'action',
      render: (_: any, record: SSHHost) => (
        <Space>
          <Button
            type="text"
            icon={<EditOutlined />}
            onClick={() => navigate(`/ssh-hosts/${record.id}`)}
          />
          <Popconfirm
            title="确定删除此主机?"
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
        <h2>SSH 主机</h2>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => navigate('/ssh-hosts/new')}
        >
          新建主机
        </Button>
      </div>
      <Table
        columns={columns}
        dataSource={hosts}
        rowKey="id"
        loading={loading}
      />
    </div>
  )
}
