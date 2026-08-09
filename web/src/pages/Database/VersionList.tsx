import { useState } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Select, InputNumber, Input, Divider,
  message, Popconfirm, Row, Col, Empty, Spin,
} from 'antd';
import {
  DatabaseOutlined, PlusOutlined, ReloadOutlined,
  PlayCircleOutlined, StopOutlined,
  FileTextOutlined, UndoOutlined, EditOutlined,
} from '@ant-design/icons';
import { dbServerApi } from '../../services/api';
import STYLES from './styles';
import type { VersionListProps, DBInstance } from './types';

export default function VersionList({
  server, versions, versionsLoading, operating,
  onEnterVersion, onRefreshVersions,
  onStartVersion, onStopVersion, onUninstallVersion,
  installVersionVisible, onInstallVersionVisibleChange,
  versionTemplates, installVersionForm, onInstallVersion,
  portCheck, onCheckPort,
  logVisible, logVersion, logContent, logLoading, logFollow, logRef,
  onLogVisibleChange, onLogFollowChange, onShowLogs,
  statusColor, statusTag,
}: VersionListProps) {

  // ===== Docker Hub "更多版本" (paged, merged into the version Select) =====
  const DOCKER_PAGE_SIZE = 10;
  const [dockerTags, setDockerTags] = useState<string[]>([]);
  const [dockerTotal, setDockerTotal] = useState(0);
  const [dockerPage, setDockerPage] = useState(1);
  const [dockerLoading, setDockerLoading] = useState(false);
  const dockerTotalPages = Math.ceil(dockerTotal / DOCKER_PAGE_SIZE);

  const fetchDockerTags = async (page: number) => {
    setDockerLoading(true);
    try {
      const res = await dbServerApi.listDockerTags(server.db_type, page, DOCKER_PAGE_SIZE);
      setDockerTags(res.data?.data?.items || []);
      setDockerTotal(res.data?.data?.total || 0);
      setDockerPage(res.data?.data?.page || page);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '查询 Docker Hub 失败');
      setDockerTags([]);
      setDockerTotal(0);
    } finally { setDockerLoading(false); }
  };

  const handleUpdatePort = (v: DBInstance) => {
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
            await dbServerApi.updateInstancePort(v.id, newPort);
            message.success('端口已修改，启动服务后生效');
            onRefreshVersions();
          } catch (error: unknown) {
            message.error((error instanceof Error ? error.message : '修改失败'));
          }
        }
      },
    });
  };

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Space>
          <DatabaseOutlined style={{ fontSize: 24, color: '#1677ff' }} />
          <span style={{ fontSize: 18, fontWeight: 'bold' }}>{server.display_name}</span>
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
            <Col xs={24} sm={12} lg={8} key={v.id} style={{ display: 'flex' }}>
              <Card
                hoverable
                onClick={() => v.status === 'running' && onEnterVersion(v)}
                style={{
                  borderColor: statusColor(v.status),
                  opacity: v.status !== 'running' ? 0.7 : 1,
                  width: '100%',
                  display: 'flex',
                  flexDirection: 'column',
                }}
                styles={{
                  body: {
                    flex: 1,
                    display: 'flex',
                    flexDirection: 'column',
                    justifyContent: 'space-between',
                  },
                }}
              >
                <div>
                  <Card.Meta
                    title={<Space wrap>{server.display_name} {v.version}{statusTag(v.status)}</Space>}
                    description={
                      <div>
                        <p style={{ margin: '4px 0' }}>端口: <strong>{v.port}</strong></p>
                        <p style={{ margin: '4px 0' }}>服务: <Tag>{v.container_id}</Tag></p>
                      </div>
                    }
                  />
                </div>
                <div style={{ ...STYLES.cardActions, marginTop: 'auto' }}>
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
          {/* image is resolved from the preset catalogue or the Docker Hub
              picker; hidden because users never type image names. */}
          <Form.Item name="image" hidden><Input /></Form.Item>
          <Form.Item name="version" label="选择版本" rules={[{ required: true, message: '请选择版本' }]}>
            <Select
              placeholder="选择要安装的版本"
              onChange={(v) => {
                const t = versionTemplates.find(t => t.version === v);
                installVersionForm.setFieldsValue({ image: t ? t.image : `${server.base_image}:${v}` });
              }}
              dropdownRender={(menu) => (
                <>
                  {menu}
                  <Divider style={{ margin: '4px 0' }} />
                  <Space style={{ padding: '0 8px 4px', justifyContent: 'space-between', width: '100%' }}>
                    <Button type="link" size="small" icon={<ReloadOutlined />} loading={dockerLoading}
                      style={{ paddingLeft: 0 }} onClick={() => fetchDockerTags(1)}>
                      更多版本
                    </Button>
                    {dockerTotal > 0 && (
                      <Space size="small">
                        <Button size="small" disabled={dockerPage <= 1 || dockerLoading}
                          onClick={() => fetchDockerTags(dockerPage - 1)}>上一页</Button>
                        <span style={{ fontSize: 12, color: '#999' }}>
                          {dockerLoading ? '加载中…' : `第 ${dockerPage} / ${dockerTotalPages} 页`}
                        </span>
                        <Button size="small" disabled={dockerPage >= dockerTotalPages || dockerLoading}
                          onClick={() => fetchDockerTags(dockerPage + 1)}>下一页</Button>
                      </Space>
                    )}
                  </Space>
                </>
              )}
            >
              <Select.OptGroup label="预设版本">
                {versionTemplates.map(t => (
                  <Select.Option key={t.version} value={t.version}>
                    <strong>{t.version}</strong><span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>{t.description}</span>
                  </Select.Option>
                ))}
              </Select.OptGroup>
              {dockerTags.length > 0 && (
                <Select.OptGroup label={`更多版本（Docker Hub 第 ${dockerPage} 页）`}>
                  {dockerTags.map(tag => (
                    <Select.Option key={tag} value={tag}>
                      <strong>{server.base_image}:{tag}</strong>
                    </Select.Option>
                  ))}
                </Select.OptGroup>
              )}
            </Select>
          </Form.Item>
          <Form.Item name="container_engine" label="容器引擎" initialValue="docker">
            <Select options={[{ value: 'docker', label: 'Docker' }, { value: 'podman', label: 'Podman（rootful）' }]} />
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
