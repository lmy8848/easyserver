import { useEffect, useRef, useState } from 'react';
import { Modal, Space, Switch, Spin, Button } from 'antd';
import { FileTextOutlined } from '@ant-design/icons';
import { dbServerApi } from '../../services/api';
import STYLES from './styles';
import type { DBInstance } from './types';

interface ServiceLogModalProps {
  /** Which instance's logs to show; null closes the modal. */
  version: DBInstance | null;
  /** Engine display name for the title (e.g. "MySQL"). */
  engineName: string;
  onClose: () => void;
}

// Service logs, self-contained: owns the 5s poll, follow switch and scroll —
// the one place the "日志" button (header / table explorer) opens. Shared via
// a single modal rendered at the page root.
export default function ServiceLogModal({ version, engineName, onClose }: ServiceLogModalProps) {
  const [logContent, setLogContent] = useState('');
  const [logLoading, setLogLoading] = useState(false);
  const [logFollow, setLogFollow] = useState(true);
  const logRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!version) return;
    setLogLoading(true);
    let active = true;
    const refresh = async () => {
      try {
        const res = await dbServerApi.getInstanceLogs(version.id, 200);
        if (active) setLogContent(res.data?.data?.logs || '(empty)');
      } catch (error) {
        if (active) setLogContent('Failed: ' + (error instanceof Error ? error.message : String(error)));
      } finally {
        if (active) setLogLoading(false);
      }
    };
    refresh();
    const timer = setInterval(refresh, 5000);
    return () => { active = false; clearInterval(timer); };
  }, [version]);

  useEffect(() => {
    if (logFollow && logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [logContent, logFollow]);

  return (
    <Modal
      title={<Space><FileTextOutlined /><span>{version ? `${engineName} ${version.version}` : ''} - 服务日志</span>{logLoading && <Spin size="small" />}</Space>}
      open={!!version}
      onCancel={onClose}
      footer={
        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
          <Space size="small">
            <Switch checked={logFollow} onChange={setLogFollow} />
            <span style={{ color: '#8c8c8c', fontSize: 12 }}>自动滚动</span>
            <span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span>
          </Space>
          <Button size="small" onClick={onClose}>关闭</Button>
        </Space>
      }
      width="90vw" style={{ maxWidth: 960 }}>
      <div ref={logRef} style={{ ...STYLES.logContainer }}>
        {logContent.split('\n').map((line, i) => (
          <div key={i} style={STYLES.logLine}>
            <span style={STYLES.logLineNumber}>{i + 1}</span>
            <span style={STYLES.logLineText}>{line || ' '}</span>
          </div>
        ))}
      </div>
    </Modal>
  );
}
