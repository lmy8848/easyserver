import { Modal, Tag, Descriptions, Typography } from 'antd';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
} from '@ant-design/icons';
import type { Service } from '../../types';

const { Text } = Typography;

interface ServiceDetailProps {
  visible: boolean;
  service: Service | null;
  onClose: () => void;
}

export default function ServiceDetail({ visible, service, onClose }: ServiceDetailProps) {
  return (
    <Modal
      title={`服务详情 - ${service?.name}`}
      open={visible}
      onCancel={onClose}
      footer={null}
      width={600}
    >
      {service && (
        <Descriptions column={2} bordered size="small">
          <Descriptions.Item label="服务名称" span={2}>
            <strong>{service.name}</strong>
          </Descriptions.Item>
          <Descriptions.Item label="描述" span={2}>
            {service.description || '-'}
          </Descriptions.Item>
          <Descriptions.Item label="状态">
            <Tag color={service.state === 'active' ? 'success' : service.state === 'failed' ? 'error' : 'default'}>
              {service.state}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="子状态">{service.sub_state}</Descriptions.Item>
          <Descriptions.Item label="PID">{service.pid || '-'}</Descriptions.Item>
          <Descriptions.Item label="内存">
            {service.memory_bytes
              ? service.memory_bytes < 1048576
                ? `${(service.memory_bytes / 1024).toFixed(1)} KB`
                : `${(service.memory_bytes / 1048576).toFixed(1)} MB`
              : '-'}
          </Descriptions.Item>
          <Descriptions.Item label="开机自启">
            {service.unit_file_state === 'static' || service.unit_file_state === 'masked'
              ? <Tag color="warning">{service.unit_file_state}</Tag>
              : service.enabled
                ? <Tag icon={<CheckCircleOutlined />} color="success">已启用</Tag>
                : <Tag icon={<CloseCircleOutlined />} color="default">未启用</Tag>}
          </Descriptions.Item>
          <Descriptions.Item label="配置文件路径">
            <Text copyable>/etc/systemd/system/{service.name}.service</Text>
          </Descriptions.Item>
        </Descriptions>
      )}
    </Modal>
  );
}
