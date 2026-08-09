import { useState, useEffect, useCallback } from 'react';
import { Tabs, Select, Tag, Button, Spin, Space, message } from 'antd';
import {
  CodeOutlined, CloudDownloadOutlined,
  DatabaseOutlined, GlobalOutlined, FolderOutlined,
  ReloadOutlined, RocketOutlined, PlayCircleOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { DockerStatus } from './types';
import { withEngine } from './types';
import ContainerTab from './ContainerTab';
import ImageTab from './ImageTab';
import ComposeTab from './ComposeTab';
import VolumeTab from './VolumeTab';
import NetworkTab from './NetworkTab';

const ENGINES = ['docker', 'podman'];

export default function Container() {
  const [engine, setEngine] = useState('docker');
  const [statuses, setStatuses] = useState<Record<string, DockerStatus | null>>({ docker: null, podman: null });
  const [checking, setChecking] = useState(true);
  const [installing, setInstalling] = useState(false);

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
  }, []);

  useEffect(() => { checkRuntimes(); }, [checkRuntimes]);

  const status = statuses[engine];
  // Podman has no daemon → running if installed; Docker needs docker.service running.
  const ready = !!status?.installed && (engine === 'podman' || !!status?.running);

  const handleInstall = async () => {
    setInstalling(true);
    try {
      await api.post(withEngine('/container/install', engine));
      message.success(`${engine} 安装成功`);
    } catch (err: unknown) {
      const axiosErr = err as { response?: { data?: { message?: string } }; message?: string };
      message.error(`安装失败：${axiosErr?.response?.data?.message || axiosErr?.message || '未知错误'}`);
    } finally {
      setInstalling(false);
      await checkRuntimes();
    }
  };

  const handleStart = async () => {
    try {
      await api.post(withEngine('/container/start', engine));
      message.success(`${engine} 已启动`);
    } catch {
      message.error(`启动 ${engine} 失败`);
    } finally {
      await checkRuntimes();
    }
  };

  const handleSocket = async (action: 'enable' | 'disable') => {
    try {
      await api.post(withEngine(`/container/socket/${action}`, engine));
      message.success(`Socket 已${action === 'enable' ? '启用' : '禁用'}`);
    } catch {
      message.error('Socket 操作失败');
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
        <h2 style={{ margin: 0 }}>容器管理</h2>
        <div style={{ flex: 1 }} />
        {checking ? (
          <Spin size="small" />
        ) : (
          <>
            <Select
              value={engine}
              onChange={setEngine}
              style={{ width: 140 }}
              options={ENGINES.map(r => ({ value: r, label: r === 'podman' ? 'Podman' : 'Docker' }))}
            />
            {status && (
              <Tag color={ready ? 'green' : 'red'} style={{ marginLeft: 8 }}>
                {!status.installed ? '未安装' : (ready ? '运行中' : '已停止')}
              </Tag>
            )}
            <Space style={{ marginLeft: 8 }}>
              {!status?.installed ? (
                <Button icon={<RocketOutlined />} loading={installing} onClick={handleInstall}>安装 {engine}</Button>
              ) : engine === 'docker' && !status.running ? (
                <Button icon={<PlayCircleOutlined />} onClick={handleStart}>启动</Button>
              ) : null}
              {status?.installed && engine === 'podman' && (
                <>
                  <Button size="small" onClick={() => handleSocket('enable')}>启用Socket</Button>
                  <Button size="small" onClick={() => handleSocket('disable')}>禁用Socket</Button>
                </>
              )}
              <Button icon={<ReloadOutlined />} onClick={checkRuntimes}>刷新</Button>
            </Space>
          </>
        )}
      </div>

      {ready ? (
        <Tabs items={resourceTabs} />
      ) : (
        <div style={{ textAlign: 'center', padding: '60px 0', color: '#888' }}>
          {status?.installed ? `请先启动 ${engine}` : `${engine} 未安装，请点击右上角「安装 ${engine}」`}
        </div>
      )}
    </div>
  );
}