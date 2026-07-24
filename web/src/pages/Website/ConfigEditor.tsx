import { forwardRef, useImperativeHandle, useState, useEffect, useRef } from 'react';
import {
  Button, Space, Tag, Modal, Input, message, Spin, Row, Col,
} from 'antd';
import {
  FileTextOutlined, ReloadOutlined, CopyOutlined,
  CheckCircleOutlined, CloseCircleOutlined,
} from '@ant-design/icons';
import { webServerApi } from '../../services/api';
import type { WebServer } from '../../types';
import type { ConfigTestResult } from './types';
import { copyToClipboard } from '../../utils/clipboard';

export interface ConfigEditorRef {
  showConfig: () => void;
  showServiceLogs: () => void;
}

interface ConfigEditorProps {
  selectedServer: WebServer;
  configTestResult: ConfigTestResult | null;
  onTestConfig: (server: WebServer) => void;
  onConfigTestResultChange: (result: ConfigTestResult | null) => void;
}

// Log level styles
const LOG_STYLES = {
  error:  { color: '#cf1322', bg: '#fff1f0', border: '#ffa39e' },
  warn:   { color: '#ad6800', bg: '#fffbe6', border: '#ffe58f' },
  info:   { color: '#006d75', bg: '#e6fffb', border: '#b5f5ec' },
  debug:  { color: '#8c8c8c', bg: 'transparent', border: 'transparent' },
  default:{ color: '#262626', bg: 'transparent', border: 'transparent' },
} as const;

function highlightLogLine(line: string) {
  const lower = line.toLowerCase();
  if (lower.includes('error') || lower.includes('fatal') || lower.includes('panic')) return LOG_STYLES.error;
  if (lower.includes('warn')) return LOG_STYLES.warn;
  if (lower.includes('info') || lower.includes('started') || lower.includes('listening')) return LOG_STYLES.info;
  if (lower.includes('debug')) return LOG_STYLES.debug;
  return LOG_STYLES.default;
}

const ConfigEditor = forwardRef<ConfigEditorRef, ConfigEditorProps>(
  ({ selectedServer, configTestResult, onTestConfig, onConfigTestResultChange }, ref) => {
    // Config modal state
    const [configVisible, setConfigVisible] = useState(false);
    const [configContent, setConfigContent] = useState('');
    const [configLoading, setConfigLoading] = useState(false);

    // Service log modal state
    const [svcLogVisible, setSvcLogVisible] = useState(false);
    const [svcLogContent, setSvcLogContent] = useState('');
    const [svcLogLoading, setSvcLogLoading] = useState(false);
    const [svcLogFollow, setSvcLogFollow] = useState(true);
    const svcLogRef = useRef<HTMLDivElement>(null);

    // Expose methods via ref
    useImperativeHandle(ref, () => ({
      showConfig: async () => {
        setConfigVisible(true);
        setConfigLoading(true);
        onConfigTestResultChange(null);
        try {
          const res = await webServerApi.getConfig(selectedServer.id);
          setConfigContent(res.data.data?.content || '');
        } catch (error: unknown) {
          setConfigContent('# Failed to load: ' + ((error instanceof Error ? error.message : 'unknown')));
        } finally {
          setConfigLoading(false);
        }
      },
      showServiceLogs: async () => {
        setSvcLogVisible(true);
        setSvcLogLoading(true);
        try {
          const res = await webServerApi.getServiceLogs(selectedServer.id, 200);
          setSvcLogContent(res.data.data?.logs || '(empty)');
        } catch (error: unknown) {
          setSvcLogContent('Failed: ' + ((error instanceof Error ? error.message : 'unknown')));
        } finally {
          setSvcLogLoading(false);
        }
      },
    }), [selectedServer, onConfigTestResultChange]);

    // Auto-refresh service logs when modal is open (every 5s)
    useEffect(() => {
      if (!svcLogVisible) return;

      const refresh = async () => {
        try {
          const res = await webServerApi.getServiceLogs(selectedServer.id, 200);
          setSvcLogContent(res.data.data?.logs || '(empty)');
        } catch (e) {
          console.debug('Service log refresh failed:', e);
        }
      };

      const timer = setInterval(refresh, 5000);
      return () => clearInterval(timer);
    }, [svcLogVisible, selectedServer.id]);

    // Auto-scroll to bottom when follow mode is on and content changes
    useEffect(() => {
      if (svcLogFollow && svcLogRef.current) {
        svcLogRef.current.scrollTop = svcLogRef.current.scrollHeight;
      }
    }, [svcLogContent, svcLogFollow]);

    // Save config
    const handleSaveConfig = async () => {
      try {
        await webServerApi.saveConfig(selectedServer.id, configContent);
        message.success('配置已保存（已自动备份原文件）');
      } catch (error: unknown) {
        message.error((error instanceof Error ? error.message : '保存失败'));
      }
    };

    return (
      <>
        {/* Config Editor Modal */}
        <Modal
          title={`${selectedServer.display_name} - 配置文件`}
          open={configVisible}
          onCancel={() => setConfigVisible(false)}
          width={900}
          footer={
            <Space>
              <Button onClick={() => onTestConfig(selectedServer)}>测试配置</Button>
              <Button onClick={() => setConfigVisible(false)}>关闭</Button>
              <Button type="primary" onClick={handleSaveConfig}>保存</Button>
            </Space>
          }
        >
          {configTestResult && (
            <div style={{ marginBottom: 12 }}>
              {configTestResult.valid
                ? <Tag icon={<CheckCircleOutlined />} color="success">{configTestResult.message}</Tag>
                : <Tag icon={<CloseCircleOutlined />} color="error">{configTestResult.message}</Tag>
              }
            </div>
          )}
          <div style={{ marginBottom: 8, color: '#999', fontSize: 12 }}>
            文件路径: {selectedServer.config_file}（保存时自动备份原文件）
          </div>
          <Input.TextArea
            value={configLoading ? 'Loading...' : configContent}
            onChange={(e) => setConfigContent(e.target.value)}
            rows={25}
            style={{ fontFamily: 'monospace', fontSize: 12 }}
          />
        </Modal>

        {/* Service Logs Modal */}
        <Modal
          title={
            <Space>
              <FileTextOutlined />
              <span>{selectedServer.display_name} - 服务日志</span>
              {svcLogLoading && <Spin size="small" />}
            </Space>
          }
          open={svcLogVisible}
          onCancel={() => setSvcLogVisible(false)}
          footer={
            <Row justify="space-between" align="middle" style={{ gap: 12 }}>
              <Col>
                <Space size="middle">
                  <span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span>
                  <span style={{ color: svcLogFollow ? '#52c41a' : '#8c8c8c', fontSize: 12 }}>
                    {svcLogFollow ? '● 自动滚动' : '○ 已暂停'}
                  </span>
                </Space>
              </Col>
              <Col>
                <Space size="small">
                  <Button
                    size="small"
                    type={svcLogFollow ? 'primary' : 'default'}
                    onClick={() => setSvcLogFollow(!svcLogFollow)}
                  >
                    {svcLogFollow ? 'Follow ON' : 'Follow OFF'}
                  </Button>
                  <Button
                    size="small"
                    icon={<CopyOutlined />}
                    onClick={() => {
                      copyToClipboard(svcLogContent, '日志已复制到剪贴板');
                    }}
                  >
                    复制
                  </Button>
                  <Button size="small" icon={<ReloadOutlined />} onClick={() => {
                    setSvcLogLoading(true);
                    webServerApi.getServiceLogs(selectedServer.id, 200).then(res => {
                      setSvcLogContent(res.data.data?.logs || '(empty)');
                    }).catch(() => {}).finally(() => setSvcLogLoading(false));
                  }}>
                    刷新
                  </Button>
                  <Button size="small" onClick={() => setSvcLogVisible(false)}>关闭</Button>
                </Space>
              </Col>
            </Row>
          }
          width="90vw"
          style={{ maxWidth: 960 }}
        >
          {svcLogLoading && !svcLogContent ? (
            <div style={{ padding: 16 }}>
              {Array.from({ length: 12 }).map((_, i) => (
                <div key={i} style={{ display: 'flex', gap: 8, marginBottom: 6 }}>
                  <div style={{ width: 40, height: 14, background: '#f5f5f5', borderRadius: 2 }} />
                  <div style={{ width: 160, height: 14, background: '#f5f5f5', borderRadius: 2 }} />
                  <div style={{ flex: 1, height: 14, background: '#f5f5f5', borderRadius: 2, opacity: 0.5 }} />
                </div>
              ))}
            </div>
          ) : (
            <div
              ref={svcLogRef}
              style={{
                background: '#fafafa',
                border: '1px solid #e8e8e8',
                fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
                fontSize: 13,
                lineHeight: 1.8,
                padding: '8px 0',
                borderRadius: 6,
                maxHeight: '60vh',
                overflowY: 'auto',
                overflowX: 'auto',
              }}
            >
              {svcLogContent.split('\n').map((line, i) => {
                const style = highlightLogLine(line);
                return (
                  <div
                    key={i}
                    style={{
                      display: 'flex',
                      alignItems: 'baseline',
                      color: style.color,
                      background: style.bg,
                      borderLeft: `3px solid ${style.border}`,
                      padding: '0 12px',
                      minHeight: 22,
                    }}
                  >
                    <span style={{ color: '#bfbfbf', minWidth: 36, width: 36, flexShrink: 0, textAlign: 'right', marginRight: 16, userSelect: 'none', fontSize: 11 }}>
                      {i + 1}
                    </span>
                    <span style={{ whiteSpace: 'nowrap' }}>
                      {line || ' '}
                    </span>
                  </div>
                );
              })}
            </div>
          )}
        </Modal>
      </>
    );
  }
);

ConfigEditor.displayName = 'ConfigEditor';

export default ConfigEditor;
