import { useEffect, useRef, useState } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Select, InputNumber, Input, List,
  message, Popconfirm, Empty, Spin, Pagination, Table, Switch,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  PlayCircleOutlined, StopOutlined, ReloadOutlined,
  FileTextOutlined, UndoOutlined, EditOutlined, PlusOutlined, DatabaseOutlined, LoadingOutlined, InfoCircleOutlined,
} from '@ant-design/icons';
import { SiMysql, SiPostgresql, SiRedis } from '@icons-pack/react-simple-icons';
import { dbServerApi } from '../../services/api';
import STYLES from './styles';
import type { VersionListProps, DBInstance } from './types';

// Engine brand logo + color (simple-icons). Falls back to a neutral accent for
// engines not in the map.
const ENGINE_BRAND: Record<string, { Icon: typeof SiMysql; color: string }> = {
  mysql: { Icon: SiMysql, color: '#4479A1' },
  postgresql: { Icon: SiPostgresql, color: '#4169E1' },
  redis: { Icon: SiRedis, color: '#FF4438' },
};

// Sentinel option value — picking "更多版本" opens the Docker Hub pager modal
// instead of selecting a version.
const MORE_VERSIONS = '__more_versions__';

export default function VersionList({
  server, versions, versionsLoading, operating,
  onSelectVersion, onRefreshVersions,
  onStartVersion, onStopVersion, onRestartVersion, onUninstallVersion,
  installVersionVisible, onInstallVersionVisibleChange,
  versionTemplates, installVersionForm, busy, onInstallVersion,
  portCheck, onCheckPort,
  logVisible, logVersion, logContent, logLoading, logFollow, logRef,
  onLogVisibleChange, onLogFollowChange, onShowLogs,
  activeInstalls, installLogInstance, installLogLines, installLogError, installLogDone, installLogFollow, installLogRef,
  onOpenInstallLog, onCloseInstallLog, onInstallLogFollowChange,
  statusTag,
}: VersionListProps) {

  // ===== Docker Hub "更多版本" pager modal =====
  const DOCKER_PAGE_SIZE = 10;
  const [dockerVisible, setDockerVisible] = useState(false);
  const [dockerTags, setDockerTags] = useState<string[]>([]);
  const [dockerTotal, setDockerTotal] = useState(0);
  const [dockerPage, setDockerPage] = useState(1);
  const [dockerLoading, setDockerLoading] = useState(false);

  const openDockerTags = async () => {
    setDockerVisible(true);
    setDockerLoading(true);
    try {
      const res = await dbServerApi.listDockerTags(server.db_type, 1, DOCKER_PAGE_SIZE);
      setDockerTags(res.data?.data?.items || []);
      setDockerTotal(res.data?.data?.total || 0);
      setDockerPage(1);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '查询 Docker Hub 失败');
      setDockerTags([]);
      setDockerTotal(0);
    } finally { setDockerLoading(false); }
  };

  const fetchDockerPage = async (page: number) => {
    setDockerLoading(true);
    try {
      const res = await dbServerApi.listDockerTags(server.db_type, page, DOCKER_PAGE_SIZE);
      setDockerTags(res.data?.data?.items || []);
      setDockerPage(res.data?.data?.page || page);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '查询 Docker Hub 失败');
    } finally { setDockerLoading(false); }
  };

  // Picking a published tag fills version + image into the install form; the
  // image is the fully-qualified `docker.io/<base_image>:<tag>` so the backend
  // never resolves (or has to resolve) short names.
  const pickDockerTag = (tag: string) => {
    installVersionForm.setFieldsValue({ version: tag, image: `docker.io/${server.base_image}:${tag}` });
    setDockerVisible(false);
  };

  // ===== Selected instance (drives the header actions) =====
  const [selectedVersion, setSelectedVersion] = useState<DBInstance | null>(null);

  // Instances load async; auto-select the first when the list arrives or the
  // previously selected one disappears. Always take the fresh object from the
  // incoming list (not `prev`) — status tags/buttons must reflect the latest
  // status (running/stopped) after a start/stop refresh.
  useEffect(() => {
    if (versions.length === 0) {
      setSelectedVersion(null);
      return;
    }
    setSelectedVersion(prev => {
      const fresh = prev ? versions.find(v => v.id === prev.id) : undefined;
      return fresh ?? versions[0] ?? null;
    });
  }, [versions]);

  // Sync the local selection to the parent so the instance detail (databases /
  // users / config, rendered below by DatabaseList) follows the selected
  // version — and refresh it whenever the instance's status changes.
  const lastNotifiedKey = useRef<string>('');
  useEffect(() => {
    if (!selectedVersion) return;
    const key = `${selectedVersion.id}:${selectedVersion.status}`;
    if (key !== lastNotifiedKey.current) {
      lastNotifiedKey.current = key;
      onSelectVersion(selectedVersion);
    }
  }, [selectedVersion, onSelectVersion]);

  // Installs are serialized per engine, so at most one is active for this tab;
  // the title-bar install button turns into a "正在安装" trigger for it. During
  // the image pull no instance row exists yet (active-installs endpoint), then
  // a "provisioning" row appears once the container is created — both lead here.
  const activeInstall = activeInstalls.find(a => a.engine === server.db_type) ?? null;
  const provisioningVersion = versions.find(v => v.status === 'provisioning') ?? null;
  const installing = activeInstall
    ? { id: activeInstall.install_id, version: activeInstall.version }
    : provisioningVersion
      ? { id: provisioningVersion.container_id, version: provisioningVersion.version }
      : null;

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

  // ===== Instance info modal (one row per field of the selected instance) =====
  const [infoVisible, setInfoVisible] = useState(false);
  const infoColumns: ColumnsType<{ key: string; label: string; value: React.ReactNode }> = [
    { title: '属性', dataIndex: 'label', width: 140 },
    { title: '值', dataIndex: 'value' },
  ];
  const infoRows: Array<{ key: string; label: string; value: React.ReactNode }> = selectedVersion ? [
    { key: 'version', label: '版本', value: <strong>{selectedVersion.version}</strong> },
    { key: 'status', label: '状态', value: statusTag(selectedVersion.status) },
    { key: 'port', label: '端口', value: selectedVersion.port },
    { key: 'container_engine', label: '容器引擎', value: selectedVersion.container_engine },
    { key: 'image', label: '镜像', value: <Tag>{selectedVersion.image}</Tag> },
    { key: 'container_id', label: '容器ID', value: <Tag>{selectedVersion.container_id}</Tag> },
    { key: 'volume_name', label: '数据卷', value: selectedVersion.volume_name },
    { key: 'bind_address', label: '监听地址', value: selectedVersion.bind_address },
    { key: 'created_at', label: '创建时间', value: selectedVersion.created_at },
  ] : [];

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12 }}>
          <Space align="center">
            {(() => {
              const brand = ENGINE_BRAND[server.db_type];
              const Icon = brand?.Icon;
              return Icon
                ? <Icon size={34} color={brand.color} style={{ display: 'flex' }} />
                : <DatabaseOutlined style={{ fontSize: 30, color: '#1677ff' }} />;
            })()}
            <span style={{ fontSize: 18, fontWeight: 'bold' }}>{server.display_name}</span>
          </Space>

          {/* Right side: version picker + the selected version's status /
              start-stop / logs / info, plus refresh & install */}
          <Space wrap size="middle" align="center">
            <Select
              style={{ minWidth: 130 }}
              placeholder="选择版本"
              value={selectedVersion?.version}
              onChange={(ver) => setSelectedVersion(versions.find(v => v.version === ver) || null)}
              options={versions.map(v => ({ value: v.version, label: v.version }))}
            />
            {selectedVersion && (
              <>
                <span style={{ fontWeight: 600 }}>版本 {selectedVersion.version}</span>
                {statusTag(selectedVersion.status)}
                {selectedVersion.status === 'running' ? (
                  <>
                    <Button icon={<StopOutlined />} loading={operating === `stop-${selectedVersion.id}`}
                      onClick={() => onStopVersion(selectedVersion)}>停止</Button>
                    <Button icon={<ReloadOutlined />} loading={operating === `restart-${selectedVersion.id}`}
                      onClick={() => onRestartVersion(selectedVersion)}>重启</Button>
                  </>
                ) : selectedVersion.status === 'provisioning' ? (
                  <Button type="primary" ghost icon={<FileTextOutlined />}
                    onClick={() => onOpenInstallLog({ id: selectedVersion.container_id, version: selectedVersion.version })}>查看安装进度</Button>
                ) : selectedVersion.status === 'failed' ? (
                  <Button type="primary" ghost icon={<FileTextOutlined />}
                    onClick={() => onOpenInstallLog({ id: selectedVersion.container_id, version: selectedVersion.version })}>查看安装日志</Button>
                ) : (
                  <Button type="primary" icon={<PlayCircleOutlined />} loading={operating === `start-${selectedVersion.id}`}
                    onClick={() => onStartVersion(selectedVersion)}>启动</Button>
                )}
                {selectedVersion.status !== 'provisioning' && (
                  <>
                    <Button icon={<FileTextOutlined />} onClick={() => onShowLogs(selectedVersion)}>日志</Button>
                    <Button icon={<EditOutlined />} onClick={() => handleUpdatePort(selectedVersion)}>修改端口</Button>
                    <Button icon={<InfoCircleOutlined />} onClick={() => setInfoVisible(true)}>实例信息</Button>
                    <Popconfirm title={`确定卸载 ${server.display_name} ${selectedVersion.version}？`}
                      onConfirm={() => onUninstallVersion(selectedVersion)}>
                      <Button danger icon={<UndoOutlined />} loading={operating === `uninstall-${selectedVersion.id}`}>卸载</Button>
                    </Popconfirm>
                  </>
                )}
              </>
            )}
            <Button icon={<ReloadOutlined />} loading={versionsLoading} onClick={onRefreshVersions}>刷新</Button>
            {installing ? (
              <Button style={{ background: '#fa8c16', borderColor: '#fa8c16', color: '#fff' }}
                icon={<LoadingOutlined />} onClick={() => onOpenInstallLog(installing)}>正在安装</Button>
            ) : (
              <Button type="primary" icon={<PlusOutlined />}
                onClick={() => { installVersionForm.resetFields(); onInstallVersionVisibleChange(true); }}>安装版本</Button>
            )}
          </Space>
        </div>
      </Card>

      {/* Instance Info Modal */}
      <Modal
        title={`${server.display_name} ${selectedVersion?.version || ''} - 实例信息`}
        open={infoVisible}
        onCancel={() => setInfoVisible(false)}
        footer={<Button size="small" onClick={() => setInfoVisible(false)}>关闭</Button>}
      >
        <Table
          rowKey="key"
          columns={infoColumns}
          dataSource={infoRows}
          pagination={false}
          showHeader={false}
          loading={versionsLoading}
          locale={{ emptyText: <Empty description="暂无实例信息" /> }}
        />
      </Modal>

      {/* Install Version Modal */}
      <Modal title={`安装${server.display_name}`} open={installVersionVisible} onCancel={() => onInstallVersionVisibleChange(false)}
        onOk={onInstallVersion} okText="安装" cancelText="取消" confirmLoading={busy === 'install-version'}>
        <Form form={installVersionForm} layout="vertical">
          {/* image is resolved from the preset catalogue or the Docker Hub
              picker; hidden because users never type image names. */}
          <Form.Item name="image" hidden><Input /></Form.Item>
          <Form.Item name="version" label="选择版本" rules={[{ required: true, message: '请选择版本' }]}>
            <Select
              placeholder="选择要安装的版本"
              onChange={(v) => {
                // "更多版本" is a sentinel: open the Docker Hub pager instead of
                // treating it as a version, and clear the selection (empty string
                // triggers the required rule if the user closes the pager without
                // picking anything).
                if (v === MORE_VERSIONS) {
                  installVersionForm.setFieldValue('version', '');
                  openDockerTags();
                  return;
                }
                const t = versionTemplates.find(t => t.version === v);
                installVersionForm.setFieldsValue({ image: t ? t.image : `docker.io/${server.base_image}:${v}` });
              }}
            >
              {versionTemplates.map(t => (
                <Select.Option key={t.version} value={t.version}>
                  <strong>{t.version}</strong><span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>{t.description}</span>
                </Select.Option>
              ))}
              <Select.Option value={MORE_VERSIONS}>
                <span style={{ color: '#1677ff' }}>＋ 更多版本（查询 Docker Hub）</span>
              </Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="container_engine" label="容器引擎" initialValue="podman">
            <Select options={[{ value: 'docker', label: 'Docker' }, { value: 'podman', label: 'Podman（rootful）' }]} />
          </Form.Item>
          <Form.Item name="bind_address" label="监听地址" initialValue="127.0.0.1">
            <Select options={[
              { value: '127.0.0.1', label: '仅本机（127.0.0.1）' },
              { value: '0.0.0.0', label: '所有网卡（0.0.0.0）' },
            ]} />
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

      {/* Docker Hub tag pager (opened from the "更多版本" Select option) */}
      <Modal
        title={`${server.display_name} - 更多版本（Docker Hub）`}
        open={dockerVisible}
        onCancel={() => setDockerVisible(false)}
        footer={null}
      >
        {dockerLoading ? (
          <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div>
        ) : dockerTags.length === 0 ? (
          <Empty description="未获取到版本列表" />
        ) : (
          <>
            <List
              size="small"
              bordered
              dataSource={dockerTags}
              renderItem={(tag) => (
                <List.Item onClick={() => pickDockerTag(tag)} style={{ cursor: 'pointer' }}>
                  <Space>
                    <Tag>docker.io/{server.base_image}:{tag}</Tag>
                    <span style={{ color: '#999', fontSize: 12 }}>点击选择此版本</span>
                  </Space>
                </List.Item>
              )}
            />
            <div style={{ textAlign: 'center', marginTop: 12 }}>
              <Pagination
                align="center"
                current={dockerPage}
                pageSize={DOCKER_PAGE_SIZE}
                total={dockerTotal}
                showSizeChanger={false}
                showQuickJumper={false}
                onChange={fetchDockerPage}
              />
            </div>
          </>
        )}
      </Modal>

      {/* Install Log Stream Modal — streams the async install's output via SSE.
          Footer is a follow Switch + close only: the install outcome is already
          the final log line (✅/❌), no extra status block below needed. */}
      <Modal
        title={
          <Space>
            <FileTextOutlined />
            <span>{server.display_name} {installLogInstance?.version} - 安装日志</span>
            {!installLogDone && <Spin size="small" />}
          </Space>
        }
        open={!!installLogInstance}
        onCancel={onCloseInstallLog}
        footer={
          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
            <Space size="small">
              <Switch checked={installLogFollow} onChange={onInstallLogFollowChange} />
              <span style={{ color: '#8c8c8c', fontSize: 12 }}>自动滚动</span>
            </Space>
            <Button size="small" onClick={onCloseInstallLog}>关闭</Button>
          </Space>
        }
        width="90vw" style={{ maxWidth: 960 }}>
        <div ref={installLogRef} style={{
          background: '#fafafa', border: '1px solid #e8e8e8',
          fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
          fontSize: 13, lineHeight: 1.8, padding: '8px 0', borderRadius: 6,
          maxHeight: '60vh', overflowY: 'auto', overflowX: 'auto',
        }}>
          {installLogLines.length === 0 && !installLogError && (
            <div style={{ textAlign: 'center', padding: 24, color: '#999' }}>
              {installLogDone ? '无日志输出' : '等待日志输出…'}
            </div>
          )}
          {installLogLines.map((line, i) => (
            <div key={i} style={STYLES.logLine}>
              <span style={STYLES.logLineNumber}>{i + 1}</span>
              <span style={STYLES.logLineText}>{line || ' '}</span>
            </div>
          ))}
          {installLogError && (
            <div style={STYLES.logLine}>
              <span style={STYLES.logLineNumber}>{installLogLines.length + 1}</span>
              <span style={{ ...STYLES.logLineText, color: '#ff4d4f' }}>❌ {installLogError}</span>
            </div>
          )}
        </div>
      </Modal>

      {/* Service Logs Modal */}
      <Modal
        title={<Space><FileTextOutlined /><span>{server.display_name} {logVersion?.version} - 服务日志</span>{logLoading && <Spin size="small" />}</Space>}
        open={logVisible} onCancel={() => onLogVisibleChange(false)}
        footer={
          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
            <Space size="small">
              <Switch checked={logFollow} onChange={onLogFollowChange} />
              <span style={{ color: '#8c8c8c', fontSize: 12 }}>自动滚动</span>
              <span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span>
            </Space>
            <Button size="small" onClick={() => onLogVisibleChange(false)}>关闭</Button>
          </Space>
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
