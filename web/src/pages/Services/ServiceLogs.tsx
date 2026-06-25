import { Modal, Button, Space } from 'antd';
import { SyncOutlined } from '@ant-design/icons';
import type { Service } from '../../types';
import type { LogEntry } from './types';

const getPriorityColor = (priority: string) => {
  const colors: Record<string, string> = {
    emerg: '#ff0000', alert: '#ff4444', crit: '#ff6666',
    err: '#ff4d4f', warn: '#faad14', notice: '#1890ff',
    info: '#d4d4d4', debug: '#888888',
  };
  return colors[priority] || '#d4d4d4';
};

interface ServiceLogsProps {
  visible: boolean;
  service: Service | null;
  logs: LogEntry[];
  loading: boolean;
  onClose: () => void;
  onStartStream: (serviceName: string) => void;
  logContainerRef: React.RefObject<HTMLDivElement | null>;
}

export default function ServiceLogs({
  visible,
  service,
  logs,
  loading,
  onClose,
  onStartStream,
  logContainerRef,
}: ServiceLogsProps) {
  return (
    <Modal
      title={
        <Space>
          <span>{service?.name} 日志</span>
          <Button
            size="small"
            type="primary"
            icon={<SyncOutlined />}
            onClick={() => {
              if (service) onStartStream(service.name);
            }}
          >
            实时流
          </Button>
        </Space>
      }
      open={visible}
      onCancel={onClose}
      footer={null}
      width={900}
    >
      <div
        ref={logContainerRef}
        style={{
          background: '#1e1e1e',
          color: '#d4d4d4',
          padding: 16,
          borderRadius: 4,
          maxHeight: 500,
          overflow: 'auto',
          fontFamily: 'Consolas, Monaco, monospace',
          fontSize: 12,
          lineHeight: 1.6,
        }}
      >
        {loading ? (
          <div style={{ color: '#666', textAlign: 'center', padding: 20 }}>加载中...</div>
        ) : logs.length > 0 ? (
          logs.map((log, index) => (
            <div key={index} style={{ marginBottom: 2 }}>
              {log.time && (
                <span style={{ color: '#6a9955', marginRight: 8 }}>{log.time}</span>
              )}
              {log.priority && log.priority !== 'info' && (
                <span style={{
                  color: getPriorityColor(log.priority),
                  marginRight: 8,
                  fontWeight: 'bold',
                  textTransform: 'uppercase',
                  fontSize: 10,
                }}>
                  [{log.priority}]
                </span>
              )}
              <span>{log.message}</span>
            </div>
          ))
        ) : (
          <div style={{ color: '#666', textAlign: 'center', padding: 20 }}>暂无日志</div>
        )}
      </div>
    </Modal>
  );
}
