import {
  Card, Button, Space, Tag, Modal, Form, Select, InputNumber, Input,
  message, Popconfirm, Row, Col, Empty, Spin,
} from 'antd';
import {
  DatabaseOutlined, PlusOutlined, ReloadOutlined,
  PlayCircleOutlined, StopOutlined,
  FileTextOutlined, UndoOutlined, EditOutlined, ArrowLeftOutlined,
} from '@ant-design/icons';
import { dbServerApi } from '../../services/api';
import STYLES from './styles';
import type { VersionListProps, DBVersion } from './types';

export default function VersionList({
  server, versions, versionsLoading, operating,
  onBack, onEnterVersion, onRefreshVersions,
  onStartVersion, onStopVersion, onRestartVersion, onUninstallVersion,
  installVersionVisible, onInstallVersionVisibleChange,
  versionTemplates, installVersionForm, onInstallVersion,
  portCheck, onCheckPort,
  logVisible, logVersion, logContent, logLoading, logFollow, logRef,
  onLogVisibleChange, onLogFollowChange, onShowLogs,
  statusColor, statusTag,
}: VersionListProps) {

  const handleUpdatePort = (v: DBVersion) => {
    if (v.status === 'running') {
      message.warning('请先停止服务再修改端口');
      return;
    }
    let newPort = v.port;
    Modal.confirm({
      title: `修改端口 - ${server.display_name} ${v.version}`,
      content: (
        <div>
          <p>当前端口: {v.port}</p>
          <InputNumber min={1} max={65535} defaultValue={v.port}
            style={{ width: '100%' }}
            onChange={(val) => { newPort = val as number || v.port; }} />
        </div>
      ),
      onOk: async () => {
        if (newPort > 0 && newPort !== v.port) {
          try {
            await dbServerApi.updateVersionPort(v.id, newPort);
            message.success('端口已修改，启动服务后生效');
            onRefreshVersions();
          } catch (error: any) {
            message.error(error.message || '修改失败');
          }
        }
      },
    });
  };

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={onBack}>返回</Button>
          <DatabaseOutlined style={{ fontSize: 24, color: statusColor(server.status) }} />
          <span style={{ fontSize: 18, fontWeight: 'bold' }}>{server.display_name}</span>
          {statusTag(server.status)}
          {server.version && <Tag color="blue">已安装: {server.version}</Tag>}
        </Space>
      </Card>

      <Card title="已安装版本" extra={
        <Space>
          <Button icon={<ReloadOutlined />} loading={versionsLoading} onClick={onRefreshVersions}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => { installVersionForm.resetFields(); onInstallVersionVisibleChange(true); }}>安装版本</Button>
        </Space>
      }>
        <Row gutter={[16, 16]}>
          {versions.length === 0 && !versionsLoading && <Col span={24}><Empty description="暂未安装任何版本" /></Col>}
          {versions.map(v => (
            <Col xs={24} sm={12} lg={8} key={v.id}>
              <Card hoverable onClick={() => v.status === 'running' && onEnterVersion(v)}
                style={{ borderColor: statusColor(v.status), opacity: v.status !== 'running' ? 0.7 : 1 }}>
                <Card.Meta
                  title={<Space>{server.display_name} {v.version}{statusTag(v.status)}</Space>}
                  description={<div>
                    <p style={{ margin: '4px 0' }}>端口: <strong>{v.port}</strong></p>
                    <p style={{ margin: '4px 0' }}>服务: <Tag>{v.service_name}</Tag></p>
                  </div>} />
                <div style={STYLES.cardActions}>
                  {v.status === 'running' ? (
                    <Button size="small" danger icon={<StopOutlined />} loading={operating === `stop-${v.id}`}
                      onClick={(e) => { e.stopPropagation(); onStopVersion(v); }}>停止</Button>
                  ) : (
                    <Button size="small" type="primary" icon={<PlayCircleOutlined />} loading={operating === `start-${v.id}`}
                      onClick={(e) => { e.stopPropagation(); onStartVersion(v); }}>启动</Button>
                  )}
                  <Button size="small" icon={<FileTextOutlined />} onClick={(e) => { e.stopPropagation(); onShowLogs(v); }}>日志</Button>
                  <Button size="small" icon={<EditOutlined />} onClick={(e) => { e.stopPropagation(); handleUpdatePort(v); }}>修改端口</Button>
                  <Popconfirm title="确定卸载？" onConfirm={(e) => { e?.stopPropagation(); onUninstallVersion(v); }}>
                    <Button size="small" danger icon={<UndoOutlined />} loading={operating === `uninstall-${v.id}`} onClick={(e) => e.stopPropagation()}>卸载</Button>
                  </Popconfirm>
                </div>
              </Card>
            </Col>
          ))}
        </Row>
      </Card>

      {/* Install Version Modal */}
      <Modal title="安装数据库版本" open={installVersionVisible} onCancel={() => onInstallVersionVisibleChange(false)}
        onOk={onInstallVersion} okText="安装" cancelText="取消">
        <Form form={installVersionForm} layout="vertical">
          <Form.Item name="version" label="选择版本" rules={[{ required: true, message: '请选择版本' }]}>
            <Select placeholder="选择要安装的版本">
              {versionTemplates.map(t => (
                <Select.Option key={t.version} value={t.version}>
                  <strong>{t.version}</strong><span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>{t.description}</span>
                </Select.Option>
              ))}
            </Select>
          </Form.Item>
          <Form.Item name="port" label="端口（留空使用默认）"
            extra={portCheck && (
              portCheck.available
                ? <span style={{ color: '#52c41a' }}>{portCheck.message}</span>
                : <span style={{ color: '#ff4d4f' }}>{portCheck.message}{portCheck.process && ` (${portCheck.process})`}</span>
            )}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }}
              onChange={(val) => val && onCheckPort(val as number)} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Service Logs Modal */}
      <Modal
        title={<Space><FileTextOutlined /><span>{server.display_name} {logVersion?.version} - 服务日志</span>{logLoading && <Spin size="small" />}</Space>}
        open={logVisible} onCancel={() => onLogVisibleChange(false)}
        footer={
          <Row justify="space-between" align="middle">
            <Col><Space size="middle">
              <span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span>
              <span style={{ color: logFollow ? '#52c41a' : '#8c8c8c', fontSize: 12 }}>{logFollow ? '● 自动滚动' : '○ 已暂停'}</span>
            </Space></Col>
            <Col><Space size="small">
              <Button size="small" type={logFollow ? 'primary' : 'default'} onClick={() => onLogFollowChange(!logFollow)}>{logFollow ? 'Follow ON' : 'Follow OFF'}</Button>
              <Button size="small" onClick={() => onLogVisibleChange(false)}>关闭</Button>
            </Space></Col>
          </Row>
        }
        width="90vw" style={{ maxWidth: 960 }}>
        <div ref={logRef} style={{
          background: '#fafafa', border: '1px solid #e8e8e8',
          fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
          fontSize: 13, lineHeight: 1.8, padding: '8px 0', borderRadius: 6,
          maxHeight: '60vh', overflowY: 'auto', overflowX: 'auto',
        }}>
          {logContent.split('\n').map((line, i) => (
            <div key={i} style={STYLES.logLine}>
              <span style={STYLES.logLineNumber}>{i + 1}</span>
              <span style={STYLES.logLineText}>{line || ' '}</span>
            </div>
          ))}
        </div>
      </Modal>
    </div>
  );
}
