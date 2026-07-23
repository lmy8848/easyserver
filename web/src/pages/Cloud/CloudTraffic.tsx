import { useState, useCallback } from 'react';
import { Button, Spin, Card, Statistic, Row, Col, Alert } from 'antd';
import { CloudDownloadOutlined } from '@ant-design/icons';
import { cloudApi } from '../../services/api';
import type { TrafficInfo } from './types';

interface Props {
  selectedInstance: string;
}

export default function CloudTraffic({ selectedInstance }: Props) {
  const [trafficInfo, setTrafficInfo] = useState<TrafficInfo | null>(null);
  const [loading, setLoading] = useState(false);

  const fetchTraffic = useCallback(async () => {
    if (!selectedInstance) return;
    setLoading(true);
    try {
      const res = await cloudApi.getTraffic();
      setTrafficInfo(res.data?.data || null);
    } catch (error: unknown) {
      console.error('Failed to fetch traffic:', error);
      setTrafficInfo(null);
    } finally {
      setLoading(false);
    }
  }, [selectedInstance]);

  return (
    <div>
      <div style={{ marginBottom: 16 }}>
        <Button
          icon={<CloudDownloadOutlined />}
          onClick={fetchTraffic}
          loading={loading}
          disabled={!selectedInstance}
        >
          查询流量
        </Button>
        {!selectedInstance && (
          <span style={{ marginLeft: 8, color: '#999' }}>请先在实例管理中选择一个实例</span>
        )}
      </div>
      {loading ? (
        <Spin />
      ) : trafficInfo ? (
        <Row gutter={16}>
          <Col span={6}>
            <Card>
              <Statistic title="总流量" value={trafficInfo.package_total_gb} suffix="GB" />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic
                title="已使用"
                value={trafficInfo.package_used_gb}
                suffix="GB"
                valueStyle={{ color: trafficInfo.package_used_gb > trafficInfo.package_total_gb * 0.8 ? '#ff4d4f' : undefined }}
              />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic title="剩余" value={trafficInfo.package_remaining_gb} suffix="GB" />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic
                title="使用率"
                value={trafficInfo.package_total_gb > 0 ? ((trafficInfo.package_used_gb / trafficInfo.package_total_gb) * 100) : 0}
                precision={1}
                suffix="%"
                valueStyle={{ color: trafficInfo.package_used_gb > trafficInfo.package_total_gb * 0.8 ? '#ff4d4f' : undefined }}
              />
            </Card>
          </Col>
          {trafficInfo.package_expired_at && (
            <Col span={24} style={{ marginTop: 16 }}>
              <Alert title={`流量包过期时间: ${trafficInfo.package_expired_at}`} type="info" />
            </Col>
          )}
        </Row>
      ) : (
        <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>
          {selectedInstance ? '暂无流量数据' : '请先选择实例'}
        </div>
      )}
    </div>
  );
}
