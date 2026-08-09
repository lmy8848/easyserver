import { useState, useEffect, useCallback, useRef } from 'react';
import { Tabs, Select, Badge, Button, Switch, Space, message, Result } from 'antd';
import {
  CodeOutlined, CloudDownloadOutlined,
  DatabaseOutlined, GlobalOutlined, FolderOutlined,
  ReloadOutlined, RocketOutlined, PlayCircleOutlined,
} from '@ant-design/icons';
import { SiDocker, SiPodman } from '@icons-pack/react-simple-icons';
import api from '../../services/api';
import { useAsyncRun } from '../../hooks/useAsyncRun';
import type { DockerStatus } from './types';
import { withEngine } from './types';
import ContainerTab from './ContainerTab';
import ImageTab from './ImageTab';
import ComposeTab from './ComposeTab';
import VolumeTab from './VolumeTab';
import NetworkTab from './NetworkTab';

const ENGINES = ['docker', 'podman'];

const engineName = (e: string) => (e === 'podman' ? 'Podman' : 'Docker');
const engineLogo = (e: string) =>
  e === 'podman'
    ? <SiPodman size={20} color="#892CA0" style={{ flexShrink: 0, display: 'block' }} />
    : <SiDocker size={20} color="#2496ED" style={{ flexShrink: 0, display: 'block' }} />;
const engineOptionLabel = (e: string) => (
  <span style={{ display: 'flex', alignItems: 'center', height: '100%', gap: 6 }}>
    <span style={{ display: 'inline-flex', alignItems: 'center', lineHeight: 0 }}>{engineLogo(e)}</span>
    {engineName(e)}
  </span>
);

// Health score for engine auto-selection: running > installed-not-running >
// not-installed > unknown. Podman counts as running once installed.
function engineScore(e: string, s: DockerStatus | null): number {
  if (!s) return 0;
  if (s.installed && (e === 'podman' || s.running)) return 3;
  if (s.installed) return 2;
  return 1;
}

export default function Container() {
  const [engine, setEngine] = useState('docker');
  const [statuses, setStatuses] = useState<Record<string, DockerStatus | null>>({ docker: null, podman: null });
  const [checking, setChecking] = useState(true);
  const [installing, runInstall] = useAsyncRun();
  const [starting, runStart] = useAsyncRun();
  const [socketLoading, runSocket] = useAsyncRun();
  const pickedRef = useRef(false); // auto-select the healthiest engine only on first load

  // Detect both engines on mount.
  const checkRuntimes = useCallback(async () => {
    setChecking(true);
    const next: Record<string, DockerStatus | null> = { docker: null, podman: null };
    await Promise.all(ENGINES.map(async (r) => {
      try {
        const res = await api.get(withEngine('/container/status', r));
        next[r] = res.data?.data;
      } catch {
        next[r] = null;
      }
    }));
    setStatuses(next);
    setChecking(false);
    if (!pickedRef.current) {
      pickedRef.current = true;
      let best: string = ENGINES[0]!;
      let bestScore = -1;
      for (const r of ENGINES) {
        const sc = engineScore(r, next[r] ?? null);
        if (sc > bestScore) { bestScore = sc; best = r; }
      }
      setEngine(best);
    }
  }, []);

  useEffect(() => { checkRuntimes(); }, [checkRuntimes]);

  const status = statuses[engine];
  // Podman has no daemon → running if installed; Docker needs docker.service running.
  const ready = !!status?.installed && (engine === 'podman' || !!status?.running);

  const handleInstall = async () => {
    try {
      await runInstall(() => api.post(withEngine('/container/install', engine)));
      message.success(`${engine} 安装成功`);
    } catch (err: unknown) {
      const axiosErr = err as { response?: { data?: { message?: string } }; message?: string };
      message.error(`安装失败：${axiosErr?.response?.data?.message || axiosErr?.message || '未知错误'}`);
    } finally {
      await checkRuntimes();
    }
  };

  const handleStart = async () => {
    try {
      await runStart(() => api.post(withEngine('/container/start', engine)));
      message.success(`${engine} 已启动`);
    } catch {
      message.error(`启动 ${engine} 失败`);
    } finally {
      await checkRuntimes();
    }
  };

  const handleSocket = async (action: 'enable' | 'disable') => {
    try {
      await runSocket(() => api.post(withEngine(`/container/socket/${action}`, engine)));
      message.success(`Socket 已${action === 'enable' ? '启用' : '禁用'}`);
    } catch {
      message.error('Socket 操作失败');
    } finally {
      await checkRuntimes();
    }
  };

  const resourceTabs = [
    { key: 'containers', label: <span><CodeOutlined /> 容器</span>, children: <ContainerTab engine={engine} /> },
    { key: 'images', label: <span><CloudDownloadOutlined /> 镜像</span>, children: <ImageTab engine={engine} /> },
    { key: 'compose', label: <span><FolderOutlined /> Compose</span>, children: <ComposeTab engine={engine} /> },
    { key: 'volumes', label: <span><DatabaseOutlined /> 存储卷</span>, children: <VolumeTab engine={engine} /> },
    { key: 'networks', label: <span><GlobalOutlined /> 网络</span>, children: <NetworkTab engine={engine} /> },
  ];

  return (
    <div>
      {/* 标题 + 右侧引擎切换/状态/操作 */}
      <div style={{ display: 'flex', alignItems: 'center', marginBottom: 16 }}>
        <span style={{ display: 'inline-flex', alignItems: 'center', marginRight: 10 }}>
          {engine === 'podman'
            ? <SiPodman size={32} color="#892CA0" style={{ flexShrink: 0, verticalAlign: 'middle' }} />
            : <SiDocker size={32} color="#2496ED" style={{ flexShrink: 0, verticalAlign: 'middle' }} />}
        </span>
        <h2 style={{ margin: 0 }}>容器管理</h2>
        <div style={{ flex: 1 }} />
        <Select
          value={engine}
          onChange={setEngine}
          style={{ width: 150 }}
          optionLabelProp="label"
          options={ENGINES.map(r => ({ value: r, label: engineOptionLabel(r) }))}
        />
        {status && (
          <span style={{ marginLeft: 12, display: 'inline-flex', alignItems: 'center', gap: 6, color: '#666' }}>
            <Badge status={ready ? 'success' : 'error'} />
            <span>
              {!status.installed ? '未安装' : (ready ? '运行中' : '已停止')}
              {status.installed && status.version ? ` · v${status.version}` : ''}
            </span>
          </span>
        )}
        <Space style={{ marginLeft: 12 }}>
          {!status?.installed ? (
            <Button icon={<RocketOutlined />} loading={installing} onClick={handleInstall}>安装 {engine}</Button>
          ) : engine === 'docker' && !status.running ? (
            <Button icon={<PlayCircleOutlined />} loading={starting} onClick={handleStart}>启动</Button>
          ) : null}
          {status?.installed && engine === 'podman' && (
            <Switch
              checked={!!status.socket_enabled}
              checkedChildren="Socket"
              unCheckedChildren="Socket"
              loading={socketLoading}
              onChange={(checked) => handleSocket(checked ? 'enable' : 'disable')}
            />
          )}
          <Button icon={<ReloadOutlined />} loading={checking} onClick={checkRuntimes}>刷新</Button>
        </Space>
      </div>

      {ready ? (
        <Tabs items={resourceTabs} />
      ) : !status?.installed ? (
        <Result
          status="info"
          title={`${engine === 'podman' ? 'Podman' : 'Docker'} 未安装`}
          subTitle={`点击下方按钮安装 ${engine}，安装完成后即可管理容器`}
          extra={<Button type="primary" icon={<RocketOutlined />} loading={installing} onClick={handleInstall}>安装 {engine}</Button>}
        />
      ) : (
        <Result
          status="warning"
          title={`${engine === 'podman' ? 'Podman' : 'Docker'} 已安装但未运行`}
          subTitle="请启动引擎服务后继续"
          extra={<Button type="primary" icon={<PlayCircleOutlined />} onClick={handleStart}>启动 {engine}</Button>}
        />
      )}
    </div>
  );
}