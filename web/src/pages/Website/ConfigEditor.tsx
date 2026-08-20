import { forwardRef, useImperativeHandle, useState, useEffect, useMemo } from 'react';
import {
  Button, Space, Tag, Modal, Input, message, Spin, Row, Col,
} from 'antd';
import {
  FileTextOutlined, ReloadOutlined,
  CheckCircleOutlined, CloseCircleOutlined,
} from '@ant-design/icons';
import { webServerApi } from '../../services/web';
import type { WebServer } from '../../types';
import type { ConfigTestResult } from './types';
import { LogViewer } from '../../components/LogViewer';

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

    const svcLogLines = useMemo(
      () => (svcLogContent ? svcLogContent.split('\n') : []),
      [svcLogContent]
    );

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
          const errMsg = error instanceof Error ? error.message : 'unknown';
          setSvcLogContent('Failed: ' + errMsg);
          message.error('获取服务日志失败: ' + errMsg);
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

    // Save config
    const handleSaveConfig = async () => {
      try {
        await webServerApi.saveConfig(selectedServer.id, configContent);
        message.success('配置已保存（已自动备份原文件）');
        setConfigVisible(false);
      } catch (error: unknown) {
        message.error('保存失败: ' + ((error instanceof Error ? error.message : 'unknown')));
      }
    };

    return (
      <>
        {/* Main configuration file modal */}
        <Modal
          title={
            <Row justify="space-between" align="middle" style={{ width: '100%', paddingRight: 24 }}>
              <Col>
                <Space>
                  <FileTextOutlined />
                  <span>{selectedServer.display_name} - 主配置文件</span>
                </Space>
              </Col>
              <Col>
                <Space>
                  {configTestResult && (
                    <Tag
                      color={configTestResult.valid ? 'success' : 'error'}
                      icon={configTestResult.valid ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
                    >
                      {configTestResult.valid ? '语法检查通过' : '语法错误'}
                    </Tag>
                  )}
                  <Button
                    size="small"
                    onClick={() => onTestConfig(selectedServer)}
                  >
                    测试配置
                  </Button>
                  <Button
                    type="primary"
                    size="small"
                    onClick={handleSaveConfig}
                  >
                    保存配置
                  </Button>
                </Space>
              </Col>
            </Row>
          }
          open={configVisible}
          onCancel={() => setConfigVisible(false)}
          footer={null}
          width="90vw"
          style={{ maxWidth: 960 }}
        >
          {configLoading ? (
            <div style={{ textAlign: 'center', padding: 40 }}><Spin /></div>
          ) : (
            <Input.TextArea
              value={configContent}
              onChange={(e) => setConfigContent(e.target.value)}
              rows={22}
              style={{
                fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
                fontSize: 13,
                lineHeight: 1.6,
                background: '#1e1e1e',
                color: '#d4d4d4',
              }}
            />
          )}
        </Modal>

        {/* Service logs modal */}
        <Modal
          title={
            <Space>
              <FileTextOutlined />
              <span>{selectedServer.display_name} - 服务日志</span>
            </Space>
          }
          open={svcLogVisible}
          onCancel={() => setSvcLogVisible(false)}
          footer={null}
          width={1000}
          destroyOnHidden
          styles={{ body: { padding: 0 } }}
        >
          <LogViewer
            lines={svcLogLines}
            downloadFileName={`webserver_${selectedServer.name}_log`}
            height={500}
            headerExtra={
              <Button
                icon={<ReloadOutlined />}
                loading={svcLogLoading}
                onClick={async () => {
                  setSvcLogLoading(true);
                  try {
                    const res = await webServerApi.getServiceLogs(selectedServer.id, 200);
                    setSvcLogContent(res.data.data?.logs || '(empty)');
                  } catch (e: unknown) {
                    const errMsg = e instanceof Error ? e.message : 'unknown';
                    setSvcLogContent('Failed: ' + errMsg);
                    message.error('获取服务日志失败: ' + errMsg);
                  } finally {
                    setSvcLogLoading(false);
                  }
                }}
              >
                刷新
              </Button>
            }
            style={{ border: 'none', borderRadius: 0 }}
          />
        </Modal>
      </>
    );
  }
);

export default ConfigEditor;
