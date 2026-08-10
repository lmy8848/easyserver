import { useEffect, useEffectEvent, useRef, useState } from 'react';
import { Alert, Card, Space, Switch, Tag } from 'antd';
import { FileTextOutlined, LoadingOutlined } from '@ant-design/icons';
import STYLES from './styles';

// Inline install log for an installing/failed instance — replaces the old modal.
// Self-contained: owns the SSE stream and follow-scroll. Cancel / reinstall
// live on the header card (InstanceHeader); this panel only shows the log and
// the outcome (installing / done / failed). The log area is capped so the whole
// panel stays within one viewport below the header card.
export default function InstallLogPanel({
  containerId, version, onDone,
}: {
  containerId: string;
  version: string;
  onDone?: () => void; // install finished (success / failure / cancel)
}) {
  const [lines, setLines] = useState<string[]>([]);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  const [follow, setFollow] = useState(true);
  const ref = useRef<HTMLDivElement>(null);

  // The parent re-renders every install-poll with fresh inline-arrow callbacks;
  // useEffectEvent keeps the SSE effect stable so it doesn't reconnect (and
  // replay the whole log) on every poll.
  const onDoneEvent = useEffectEvent(() => onDone?.());

  useEffect(() => {
    // SSE replay: the server sends every buffered line first (cursor starts at
    // 0), then follows live until the {type:'done'} frame.
    const es = new EventSource(`/api/db/installs/${containerId}/log`);
    es.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        if (msg.type === 'line') setLines(prev => [...prev, msg.text]);
        else if (msg.type === 'done') {
          setError(msg.error || '');
          setDone(true);
          es.close();
          onDoneEvent();
        }
      } catch { /* ignore malformed frames */ }
    };
    // Server closed the stream (or a transient blip) — stop so the "done" state
    // governs the UI. EventSource auto-reconnects otherwise.
    es.onerror = () => { setDone(true); es.close(); };
    return () => es.close();
  }, [containerId]);

  useEffect(() => {
    if (follow && ref.current) {
      ref.current.scrollTop = ref.current.scrollHeight;
    }
  }, [lines, follow]);

  const failed = done && !!error;

  return (
    <Card style={failed ? { borderColor: '#ff4d4f' } : undefined}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <Space>
          <FileTextOutlined />
          <span style={{ fontWeight: 600 }}>{version} 安装日志</span>
          {failed ? (
            <Tag color="error">安装失败</Tag>
          ) : done ? (
            <Tag color="success">安装完成</Tag>
          ) : (
            <LoadingOutlined style={{ color: '#fa8c16' }} />
          )}
        </Space>
        <Space size="small">
          <Switch checked={follow} onChange={setFollow} />
          <span style={{ color: '#8c8c8c', fontSize: 12 }}>自动滚动</span>
        </Space>
      </div>
      {failed && (
        <Alert type="error" showIcon style={{ marginBottom: 8 }}
          message="安装失败" description={error} />
      )}
      {/* Cap the log height to the viewport minus the page chrome above (type
          tabs + header card + card padding) so the panel never exceeds the page. */}
      <div ref={ref} style={{ ...STYLES.logContainer, maxHeight: 'calc(100vh - 220px)' }}>
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
    </Card>
  );
}
