import { Modal, Space, Switch, Spin, Button } from 'antd';
import { FileTextOutlined } from '@ant-design/icons';
import STYLES from './styles';
import type { EngineInfo } from './types';

interface InstallLogModalProps {
  server: EngineInfo;
  instance: { id: string; version: string } | null;
  lines: string[];
  error: string;
  done: boolean;
  follow: boolean;
  ref: React.RefObject<HTMLDivElement | null>;
  onClose: () => void;
  onFollowChange: (follow: boolean) => void;
}

// Install log stream modal — the async install's output, replayed via SSE.
// Footer is a follow Switch + close only: the install outcome is already the
// final log line (✅/❌), no extra status block below needed.
export default function InstallLogModal({
  server, instance, lines, error, done, follow, ref, onClose, onFollowChange,
}: InstallLogModalProps) {
  return (
    <Modal
      title={
        <Space>
          <FileTextOutlined />
          <span>{server.display_name} {instance?.version} - 安装日志</span>
          {!done && <Spin size="small" />}
        </Space>
      }
      open={!!instance}
      onCancel={onClose}
      footer={
        <Space style={{ width: '100%', justifyContent: 'space-between' }}>
          <Space size="small">
            <Switch checked={follow} onChange={onFollowChange} />
            <span style={{ color: '#8c8c8c', fontSize: 12 }}>自动滚动</span>
          </Space>
          <Button size="small" onClick={onClose}>关闭</Button>
        </Space>
      }
      width="90vw" style={{ maxWidth: 960 }}>
      <div ref={ref} style={{
        background: '#fafafa', border: '1px solid #e8e8e8',
        fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
        fontSize: 13, lineHeight: 1.8, padding: '8px 0', borderRadius: 6,
        maxHeight: '60vh', overflowY: 'auto', overflowX: 'auto',
      }}>
        {lines.length === 0 && !error && (
          <div style={{ textAlign: 'center', padding: 24, color: '#999' }}>
            {done ? '无日志输出' : '等待日志输出…'}
          </div>
        )}
        {lines.map((line, i) => (
          <div key={i} style={STYLES.logLine}>
            <span style={STYLES.logLineNumber}>{i + 1}</span>
            <span style={STYLES.logLineText}>{line || ' '}</span>
          </div>
        ))}
        {error && (
          <div style={STYLES.logLine}>
            <span style={STYLES.logLineNumber}>{lines.length + 1}</span>
            <span style={{ ...STYLES.logLineText, color: '#ff4d4f' }}>❌ {error}</span>
          </div>
        )}
      </div>
    </Modal>
  );
}
