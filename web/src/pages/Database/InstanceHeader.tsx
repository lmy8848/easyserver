import { useEffect, useRef, useState } from 'react';
import {
  Card, Button, Space, Select, Popconfirm, message, InputNumber, Modal,
} from 'antd';
import {
  PlayCircleOutlined, StopOutlined, ReloadOutlined,
  FileTextOutlined, DeleteOutlined, EditOutlined, DatabaseOutlined, LoadingOutlined, InfoCircleOutlined,
} from '@ant-design/icons';
import { SiMysql, SiPostgresql, SiRedis } from '@icons-pack/react-simple-icons';
import { dbServerApi } from '../../services/api';
import InstallModal from './InstallModal';
import InstallLogModal from './InstallLogModal';
import InstanceInfoModal from './InstanceInfoModal';
import ServiceLogModal from './ServiceLogModal';
import type { InstanceHeaderProps, DBInstance } from './types';

// Engine brand logo + color (simple-icons). Falls back to a neutral accent for
// engines not in the map.
const ENGINE_BRAND: Record<string, { Icon: typeof SiMysql; color: string }> = {
  mysql: { Icon: SiMysql, color: '#4479A1' },
  postgresql: { Icon: SiPostgresql, color: '#4169E1' },
  redis: { Icon: SiRedis, color: '#FF4438' },
};

// Sentinel option value in the version Select — picking it opens the install
// modal (the header no longer has a separate "安装版本" button).
const INSTALL_VERSION = '__install_version__';

// The engine header card: brand + version picker (with "＋ 安装版本" entry) +
// the selected instance's lifecycle/ops actions, plus its modals (install /
// install log / service log is page-level / instance info).
export default function InstanceHeader({
  server, versions, versionsLoading, operating,
  onSelectVersion, onRefreshVersions,
  onStartVersion, onStopVersion, onRestartVersion, onUninstallVersion,
  installVersionVisible, onInstallVersionVisibleChange,
  versionTemplates, installVersionForm, busy, onInstallVersion,
  portCheck, onCheckPort,
  activeInstalls, installLogInstance, installLogLines, installLogError, installLogDone, installLogFollow, installLogRef,
  onOpenInstallLog, onCloseInstallLog, onInstallLogFollowChange,
  statusTag,
}: InstanceHeaderProps) {
  const [selectedVersion, setSelectedVersion] = useState<DBInstance | null>(null);
  const [infoVisible, setInfoVisible] = useState(false);
  // Service log — self-contained (ServiceLogModal owns the poll/follow/scroll);
  // the header's 日志 button just sets the instance to show.
  const [logVersion, setLogVersion] = useState<DBInstance | null>(null);

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

  // Installs are serialized per engine, so at most one is active for this tab;
  // the header shows an orange "正在安装" trigger for it. During the image pull
  // no instance row exists yet (active-installs endpoint), then a "provisioning"
  // row appears once the container is created — both lead here.
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

  return (
    <>
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
                ) : selectedVersion.status === 'provisioning' ? (
                  <Button type="primary" ghost icon={<FileTextOutlined />}
                    onClick={() => onOpenInstallLog({ id: selectedVersion.container_id, version: selectedVersion.version })}>查看安装进度</Button>
                ) : selectedVersion.status === 'failed' ? (
                  <Button type="primary" ghost icon={<FileTextOutlined />}
                    onClick={() => onOpenInstallLog({ id: selectedVersion.container_id, version: selectedVersion.version })}>查看安装日志</Button>
                ) : (
                  <Button style={{ color: '#52c41a', borderColor: '#52c41a' }}
                    icon={<PlayCircleOutlined />} loading={operating === `start-${selectedVersion.id}`}
                    onClick={() => onStartVersion(selectedVersion)}>启动</Button>
                )}
                {selectedVersion.status !== 'provisioning' && (
                  <>
                    <Button icon={<FileTextOutlined />} onClick={() => setLogVersion(selectedVersion)}>日志</Button>
                    <Button icon={<EditOutlined />} onClick={() => handleUpdatePort(selectedVersion)}>修改端口</Button>
                    <Button icon={<InfoCircleOutlined />} onClick={() => setInfoVisible(true)}>实例信息</Button>
                    <Popconfirm title={`确定卸载 ${server.display_name} ${selectedVersion.version}？`}
                      onConfirm={() => onUninstallVersion(selectedVersion)}>
                      <Button danger icon={<DeleteOutlined />} loading={operating === `uninstall-${selectedVersion.id}`}>卸载</Button>
                    </Popconfirm>
                  </>
                )}
              </>
            )}
            {installing && (
              <Button style={{ background: '#fa8c16', borderColor: '#fa8c16', color: '#fff' }}
                icon={<LoadingOutlined />} onClick={() => onOpenInstallLog(installing)}>正在安装</Button>
            )}
          </Space>
        </div>
      </Card>

      {/* Modals owned by the header */}
      <InstallModal
        server={server}
        visible={installVersionVisible}
        onVisibleChange={onInstallVersionVisibleChange}
        versionTemplates={versionTemplates}
        form={installVersionForm}
        busy={busy}
        onInstall={onInstallVersion}
        portCheck={portCheck}
        onCheckPort={onCheckPort}
      />
      <InstallLogModal
        server={server}
        instance={installLogInstance}
        lines={installLogLines}
        error={installLogError}
        done={installLogDone}
        follow={installLogFollow}
        ref={installLogRef}
        onClose={onCloseInstallLog}
        onFollowChange={onInstallLogFollowChange}
      />
      <InstanceInfoModal
        server={server}
        version={selectedVersion}
        visible={infoVisible}
        loading={versionsLoading}
        onClose={() => setInfoVisible(false)}
        statusTag={statusTag}
      />
      <ServiceLogModal
        version={logVersion}
        engineName={server.display_name}
        onClose={() => setLogVersion(null)}
      />
    </>
  );
}
