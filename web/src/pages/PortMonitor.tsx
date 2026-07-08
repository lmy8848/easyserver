import { useState, useEffect } from 'react';
import { Card, Table, Tag, Input, Space, Typography, message } from 'antd';
import { systemApi } from '../services/api';

const { Search } = Input;
const { Text } = Typography;

interface PortInfo {
  protocol: string;
  port: number;
  local_addr: string;
  state: string;
  pid: number;
  process_name: string;
  user: string;
}

const PROTOCOL_COLORS: Record<string, string> = {
  tcp: 'blue',
  tcp6: 'cyan',
  udp: 'green',
  udp6: 'lime',
};

export default function PortMonitor() {
  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [filter, setFilter] = useState('');

  const fetchPorts = async () => {
    setLoading(true);
    try {
      const res = await systemApi.getListeningPorts();
      setPorts(res.data?.data?.ports || []);
    } catch (error) {
      message.error('获取端口信息失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchPorts();
    const timer = setInterval(fetchPorts, 10000); // auto-refresh every 10s
    return () => clearInterval(timer);
  }, []);

  const filtered = filter
    ? ports.filter(p =>
        String(p.port).includes(filter) ||
        p.process_name.toLowerCase().includes(filter.toLowerCase()) ||
        p.user.toLowerCase().includes(filter.toLowerCase()) ||
        p.protocol.toLowerCase().includes(filter.toLowerCase())
      )
    : ports;

  const columns = [
    {
      title: '协议',
      dataIndex: 'protocol',
      key: 'protocol',
      width: 80,
      render: (proto: string) => (
        <Tag color={PROTOCOL_COLORS[proto] || 'default'}>{proto.toUpperCase()}</Tag>
      ),
    },
    {
      title: '端口',
      dataIndex: 'port',
      key: 'port',
      width: 100,
      sorter: (a: PortInfo, b: PortInfo) => a.port - b.port,
    },
    {
      title: '本地地址',
      dataIndex: 'local_addr',
      key: 'local_addr',
    },
    {
      title: '状态',
      dataIndex: 'state',
      key: 'state',
      width: 100,
      render: (state: string) => <Tag color="success">{state}</Tag>,
    },
    {
      title: 'PID',
      dataIndex: 'pid',
      key: 'pid',
      width: 80,
      render: (pid: number) => pid > 0 ? pid : '-',
    },
    {
      title: '进程',
      dataIndex: 'process_name',
      key: 'process_name',
      render: (name: string) => name || <Text type="secondary">-</Text>,
    },
    {
      title: '用户',
      dataIndex: 'user',
      key: 'user',
      render: (user: string) => (
        <Text type={user === 'root' ? 'danger' : undefined}>{user || '-'}</Text>
      ),
    },
  ];

  return (
    <div>
      <Card
        title="端口占用列表"
        extra={
          <Space>
            <Search
              placeholder="搜索端口/进程/用户..."
              allowClear
              onSearch={setFilter}
              onChange={e => setFilter(e.target.value)}
              style={{ width: 220 }}
            />
          </Space>
        }
      >
        <Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
          共 {filtered.length} 个监听端口（每 10 秒自动刷新）
        </Text>
        <Table
          columns={columns}
          dataSource={filtered}
          rowKey={(record, index) => `${record.protocol}-${record.port}-${index}`}
          loading={loading}
          size="small"
          pagination={{ pageSize: 50, showTotal: (t) => `共 ${t} 条` }}
        />
      </Card>
    </div>
  );
}
