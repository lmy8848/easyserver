import { useState, useCallback } from 'react';
import { Table, Button, Select, Space, Spin, Descriptions } from 'antd';
import { DashboardOutlined } from '@ant-design/icons';
import { cloudApi } from '../../services/api';
import type { MonitorPoint } from './types';

interface Props {
  selectedInstance: string;
}

export default function CloudMonitor({ selectedInstance }: Props) {
  const [data, setData] = useState<MonitorPoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [metric, setMetric] = useState('CPU_USAGE');

  const fetchData = useCallback(async () => {
    if (!selectedInstance) return;
    setLoading(true);
    try {
      const end = new Date().toISOString();
      const start = new Date(Date.now() - 3600 * 1000).toISOString(); // Last 1 hour
      const res = await cloudApi.getMonitor(selectedInstance, metric, start, end);
      setData(res.data?.data?.points || []);
    } catch (error: unknown) {
      console.error('Failed to fetch monitor data:', error);
      setData([]);
    } finally {
      setLoading(false);
    }
  }, [selectedInstance, metric]);

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Space>
          <Select
            value={metric}
            onChange={setMetric}
            style={{ width: 160 }}
            options={[
              { label: 'CPU 使用率', value: 'CPU_USAGE' },
              { label: '内存使用率', value: 'MEMORY_USAGE' },
              { label: '磁盘使用率', value: 'DISK_USAGE' },
              { label: '网络流量', value: 'NETWORK_IN_OUT' },
            ]}
          />
          <Button
            icon={<DashboardOutlined />}
            onClick={fetchData}
            loading={loading}
            disabled={!selectedInstance}
          >
            查询
          </Button>
        </Space>
        {!selectedInstance && (
          <span style={{ marginLeft: 8, color: '#999' }}>请先在实例管理中选择一个实例</span>
        )}
      </div>
      {loading ? (
        <Spin />
      ) : data.length > 0 ? (
        <div>
          <Descriptions size="small" column={3} style={{ marginBottom: 16 }}>
            <Descriptions.Item label="数据点">{data.length}</Descriptions.Item>
            <Descriptions.Item label="时间范围">
              {data[0]?.timestamp} ~ {data[data.length - 1]?.timestamp}
            </Descriptions.Item>
            <Descriptions.Item label="最新值">
              {data[data.length - 1]?.value?.toFixed(2)}%
            </Descriptions.Item>
          </Descriptions>
          <Table
            dataSource={data}
            rowKey="timestamp"
            size="small"
            pagination={{ pageSize: 20 }}
            columns={[
              { title: '时间', dataIndex: 'timestamp', key: 'timestamp', width: 200 },
              { title: '值', dataIndex: 'value', key: 'value', render: (v: number) => `${v?.toFixed(2)}%` },
            ]}
          />
        </div>
      ) : (
        <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>
          {selectedInstance ? '暂无数据' : '请先选择实例'}
        </div>
      )}
    </div>
  );
}
