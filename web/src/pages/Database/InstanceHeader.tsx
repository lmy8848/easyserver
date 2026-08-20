import { useEffect, useRef, useState, useMemo } from 'react';
import {
  Card, Button, Space, Select, Popconfirm, message, InputNumber, Modal,
  Form, Input, List, Tag, Spin, Pagination, Empty,
} from 'antd';
import {
  PlayCircleOutlined, StopOutlined, ReloadOutlined,
  FileTextOutlined, DeleteOutlined, DatabaseOutlined, InfoCircleOutlined, CloseOutlined,
} from '@ant-design/icons';
import { SiMysql, SiPostgresql, SiRedis } from '@icons-pack/react-simple-icons';
import { dbServerApi } from '../../services/database';
import { LogViewer } from '../../components/LogViewer';
import type { InstanceHeaderProps, DBInstance } from './types';

// Type brand logo + color (simple-icons). Falls back to a neutral accent for
// types not in the map.
const DB_TYPE_BRAND: Record<string, { Icon: typeof SiMysql; color: string }> = {
  mysql: { Icon: SiMysql, color: '#4479A1' },
  postgresql: { Icon: SiPostgresql, color: '#4169E1' },
  redis: { Icon: SiRedis, color: '#FF4438' },
};

// Sentinel option value in the version Select — picking it opens the install
// modal (the header no longer has a separate "安装版本" button).
const INSTALL_VERSION = '__install_version__';

// Sentinel option value in the install version Select — "更多版本" opens the
// Docker Hub pager instead of selecting a version.
const MORE_VERSIONS = '__more_versions__';

// The type header card: brand + version picker + the selected instance's
// lifecycle/ops actions, plus the install (with embedded Docker Hub pager),
// instance-info and service-log modals. Per-tab modals (create db/user/grant/
// table/record) live in their own tab files; the install log is inline
// (InstallLogPanel) rendered by the page, not a modal here.
export default function InstanceHeader({
  server, versions, operating,
  onSelectVersion,
  onStartVersion, onStopVersion, onRestartVersion, onUninstallVersion,
  onCancelInstall, onReinstallVersion,
  installVersionVisible, onInstallVersionVisibleChange,
  versionTemplates, installVersionForm, busy, onInstallVersion,
  portCheck, onCheckPort,
  statusTag, pendingSelectVersion, onPendingSelectConsumed,
}: InstanceHeaderProps) {
  const [selectedVersion, setSelectedVersion] = useState<DBInstance | null>(null);
  const [infoVisible, setInfoVisible] = useState(false);
  // Service log — self-contained (poll/follow/scroll live here); the 日志 button
  // just sets the instance to show.
  const [logVersion, setLogVersion] = useState<DBInstance | null>(null);
  const [logContent, setLogContent] = useState('');
  const [logLoading, setLogLoading] = useState(false);

  // Docker Hub "更多版本" pager state (install modal).
  const DOCKER_PAGE_SIZE = 10;
  const [dockerVisible, setDockerVisible] = useState(false);
  const [dockerTags, setDockerTags] = useState<string[]>([]);
  const [dockerTotal, setDockerTotal] = useState(0);
  const [dockerPage, setDockerPage] = useState(1);
  const [dockerLoading, setDockerLoading] = useState(false);

  // Instances load async; auto-select the first when the list arrives or the
  // previously selected one disappears. Always take the fresh object from the
  // incoming list (not `prev`) — status tags/buttons must reflect the latest
  // status (running/stopped) after a start/stop refresh. 安装新版本时
  // pendingSelectVersion 优先：列表刷新后跳到新装的那一行。
  useEffect(() => {
    if (versions.length === 0) {
      setSelectedVersion(null);
      return;
    }
    setSelectedVersion(prev => {
      if (pendingSelectVersion) {
        const target = versions.find(v => v.version === pendingSelectVersion);
        if (target) return target;
      }
      const fresh = prev ? versions.find(v => v.id === prev.id) : undefined;
      return fresh ?? versions[0] ?? null;
    });
  }, [versions, pendingSelectVersion]);

  // 消费一次性跳转指令：目标已被选中后通知父组件清空，否则后续每次列表刷新
  // （如 installing 轮询、手动刷新）都会跳回该版本。
  useEffect(() => {
    if (pendingSelectVersion && selectedVersion?.version === pendingSelectVersion) {
      onPendingSelectConsumed?.();
    }
  }, [selectedVersion, pendingSelectVersion, onPendingSelectConsumed]);

  // Sync the local selection to the parent so the instance detail (databases /
  // users / config, rendered below) follows the selected version — and refresh
  // it whenever the instance's status changes.
  const lastNotifiedKey = useRef<string>('');
  useEffect(() => {
    if (!selectedVersion) return;
    const key = `${selectedVersion.id}:${selectedVersion.status}`;
    if (key !== lastNotifiedKey.current) {
      lastNotifiedKey.current = key;
      onSelectVersion(selectedVersion);
    }
  }, [selectedVersion, onSelectVersion]);

  // ===== Service log poll (self-contained) =====
  useEffect(() => {
    if (!logVersion) return;
    setLogLoading(true);
    let active = true;
    const refresh = async () => {
      try {
        const res = await dbServerApi.getInstanceLogs(logVersion.id, 200);
        if (active) setLogContent(res.data?.data?.logs || '(empty)');
      } catch (error) {
        if (active) {
          const errMsg = error instanceof Error ? error.message : String(error);
          setLogContent('Failed: ' + errMsg);
          message.error('获取服务日志失败: ' + errMsg);
        }
      } finally {
        if (active) setLogLoading(false);
      }
    };
    refresh();
    const timer = setInterval(refresh, 5000);
    return () => { active = false; clearInterval(timer); };
  }, [logVersion]);

  const logLines = useMemo(() => (logContent ? logContent.split('\n') : []), [logContent]);

  // ===== Docker Hub pager helpers (install modal) =====
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
      setDockerPage(page);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '查询 Docker Hub 失败');
    } finally { setDockerLoading(false); }
  };

  // Picking a published tag fills version + image (and regenerates the container
  // name) into the install form; the image is the fully-qualified
  // `docker.io/<base_image>:<tag>` so the backend never resolves (or has to
  // resolve) short names.
  const pickDockerTag = (tag: string) => {
    const generated = `easyserver-db-${server.db_type}-${String(tag).toLowerCase().replace(/[^a-z0-9-]/g, '-')}`;
    installVersionForm.setFieldsValue({ version: tag, image: `docker.io/${server.base_image}:${tag}`, container_name: generated });
    setDockerVisible(false);
  };
  // ===== Instance info rows =====
  const infoRows: Array<{ key: string; label: string; value: React.ReactNode }> = selectedVersion ? [
    { key: 'version', label: '版本', value: <strong>{selectedVersion.version}</strong> },
    { key: 'status', label: '状态', value: statusTag(selectedVersion.status) },
    { key: 'port', label: '端口', value: selectedVersion.port },
    { key: 'container_engine', label: '容器引擎', value: selectedVersion.container_engine },
    { key: 'image', label: '镜像', value: <Tag>{selectedVersion.image}</Tag> },
    { key: 'container_name', label: '容器名', value: <Tag>{selectedVersion.container_name}</Tag> },
    { key: 'volume_name', label: '数据目录', value: selectedVersion.volume_name },
    { key: 'bind_address', label: '监听地址', value: selectedVersion.bind_address },
    { key: 'created_at', label: '创建时间', value: selectedVersion.created_at },
  ] : [];

  return (
    <>
      <Card style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12 }}>
          <Space align="center">
            {(() => {
              const brand = DB_TYPE_BRAND[server.db_type];
              const Icon = brand?.Icon;
              return Icon
                ? <Icon size={34} color={brand.color} style={{ display: 'flex' }} />
                : <DatabaseOutlined style={{ fontSize: 30, color: '#1677ff' }} />;
            })()}
            <span style={{ fontSize: 18, fontWeight: 'bold' }}>{server.display_name}</span>
          </Space>

          {/* Right side: version picker + the selected version's status /
              start-stop / logs / info */}
          <Space wrap size="middle" align="center">
            <Select
              style={{ minWidth: 150 }}
              placeholder="选择版本"
              value={selectedVersion?.version}
              onChange={(ver) => {
                // "安装版本" is a sentinel: open the install modal and leave the
                // selection unchanged (controlled value snaps back).
                if (ver === INSTALL_VERSION) {
                  installVersionForm.resetFields();
                  onInstallVersionVisibleChange(true);
                  return;
                }
                setSelectedVersion(versions.find(v => v.version === ver) || null);
              }}
            >
              {versions.map(v => (
                <Select.Option key={v.version} value={v.version}>{v.version}</Select.Option>
              ))}
              <Select.Option value={INSTALL_VERSION}>
                <span style={{ color: '#1677ff' }}>＋ 安装版本</span>
              </Select.Option>
            </Select>
            {selectedVersion && (
              <>
                <span style={{ fontWeight: 600 }}>版本 {selectedVersion.version}</span>
                {statusTag(selectedVersion.status)}
                {selectedVersion.status === 'running' ? (
                  <>
                    <Button style={{ color: '#fa8c16', borderColor: '#fa8c16' }}
                      icon={<StopOutlined />} loading={operating === `stop-${selectedVersion.id}`}
                      onClick={() => onStopVersion(selectedVersion)}>停止</Button>
                    <Button type="primary" ghost icon={<ReloadOutlined />} loading={operating === `restart-${selectedVersion.id}`}
                      onClick={() => onRestartVersion(selectedVersion)}>重启</Button>
                  </>
                ) : selectedVersion.status === 'installing' ? (
                  <Button danger icon={<CloseOutlined />} loading={operating === `cancel-install-${selectedVersion.id}`}
                    onClick={() => onCancelInstall(selectedVersion)}>取消安装</Button>
                ) : selectedVersion.status === 'failed' ? (
                  <Button type="primary" icon={<ReloadOutlined />} loading={operating === `reinstall-${selectedVersion.id}`}
                    onClick={() => onReinstallVersion(selectedVersion)}>重新安装</Button>
                ) : (
                  <Button style={{ color: '#52c41a', borderColor: '#52c41a' }}
                    icon={<PlayCircleOutlined />} loading={operating === `start-${selectedVersion.id}`}
                    onClick={() => onStartVersion(selectedVersion)}>启动</Button>
                )}
                {selectedVersion.status !== 'installing' && selectedVersion.status !== 'failed' && (
                  <>
                    <Button icon={<FileTextOutlined />} onClick={() => setLogVersion(selectedVersion)}>日志</Button>
                    <Button icon={<InfoCircleOutlined />} onClick={() => setInfoVisible(true)}>实例信息</Button>
                    <Popconfirm title={`确定卸载 ${server.display_name} ${selectedVersion.version}？`}
                      onConfirm={() => onUninstallVersion(selectedVersion)}>
                      <Button danger icon={<DeleteOutlined />} loading={operating === `uninstall-${selectedVersion.id}`}>卸载</Button>
                    </Popconfirm>
                  </>
                )}
                {/* failed: the log/port/info buttons are hidden (nothing meaningful
                    to inspect), and the row is removed via reinstall's purge; 卸载
                    stays as a plain delete of the failed attempt. */}
                {selectedVersion.status === 'failed' && (
                  <Popconfirm title={`删除失败的 ${server.display_name} ${selectedVersion.version} 记录？`}
                    onConfirm={() => onUninstallVersion(selectedVersion)}>
                    <Button danger icon={<DeleteOutlined />} loading={operating === `uninstall-${selectedVersion.id}`}>卸载</Button>
                  </Popconfirm>
                )}
              </>
            )}
          </Space>
        </div>
      </Card>

      {/* ===== Install modal + embedded Docker Hub pager ===== */}
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
                // 选版本即按规则预填容器名：easyserver-db-<类型>-<版本>。用户可改，
                // 后端仍会校验；留空回退默认名。
                const generated = `easyserver-db-${server.db_type}-${String(v).toLowerCase().replace(/[^a-z0-9-]/g, '-')}`;
                installVersionForm.setFieldsValue({ image: t ? t.image : `docker.io/${server.base_image}:${v}`, container_name: generated });
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
          <Form.Item name="port" label={`端口（默认 ${server.default_port}）`}
            rules={[{ required: true, message: '请输入端口' }]}
            initialValue={server.default_port}
            extra={portCheck && (
              portCheck.available
                ? <span style={{ color: '#52c41a' }}>{portCheck.message}</span>
                : <span style={{ color: '#ff4d4f' }}>{portCheck.message}{portCheck.process && ` (${portCheck.process})`}</span>
            )}>
            <InputNumber min={1} max={65535} placeholder="请输入端口" style={{ width: '100%' }}
              onChange={(val) => val && onCheckPort(val as number)} />
          </Form.Item>
          <Form.Item name="container_name" label="容器名"
            rules={[
              { required: true, message: '请输入容器名' },
              { pattern: /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/, max: 128, message: '只能包含字母、数字以及 _ . -，且必须以字母或数字开头' },
            ]}>
            <Input placeholder="请输入容器名" />
          </Form.Item>
        </Form>
      </Modal>

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

      {/* ===== Instance info modal ===== */}
      <Modal
        title={`${server.display_name} ${selectedVersion?.version || ''} - 实例信息`}
        open={infoVisible}
        onCancel={() => setInfoVisible(false)}
        footer={<Button size="small" onClick={() => setInfoVisible(false)}>关闭</Button>}
      >
        {infoRows.map(row => (
          <div key={row.key} style={{ display: 'flex', padding: '8px 0', borderBottom: '1px solid #f0f0f0' }}>
            <span style={{ width: 140, color: '#8c8c8c' }}>{row.label}</span>
            <span>{row.value}</span>
          </div>
        ))}
      </Modal>

      {/* ===== Service log modal (self-contained) ===== */}
      <Modal
        title={<Space><FileTextOutlined /><span>{logVersion ? `${server.display_name} ${logVersion.version}` : ''} - 服务日志</span></Space>}
        open={!!logVersion}
        onCancel={() => setLogVersion(null)}
        footer={null}
        width={1000}
        destroyOnHidden
        styles={{ body: { padding: 0 } }}
      >
        <LogViewer
          lines={logLines}
          downloadFileName={`db_${server.db_type}_${logVersion?.version || 'instance'}_log`}
          height={500}
          headerExtra={
            <Button
              icon={<ReloadOutlined />}
              loading={logLoading}
              onClick={async () => {
                if (!logVersion) return;
                setLogLoading(true);
                try {
                  const res = await dbServerApi.getInstanceLogs(logVersion.id, 200);
                  setLogContent(res.data?.data?.logs || '(empty)');
                } catch (error) {
                  const errMsg = error instanceof Error ? error.message : String(error);
                  setLogContent('Failed: ' + errMsg);
                  message.error('获取服务日志失败: ' + errMsg);
                } finally {
                  setLogLoading(false);
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
