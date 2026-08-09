import { useState, useEffect, useCallback } from 'react';
import { Tabs, Spin } from 'antd';
import {
  CodeOutlined, CloudDownloadOutlined,
  DatabaseOutlined, GlobalOutlined, FolderOutlined, DockerOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { DockerStatus } from './types';
import { withEngine } from './types';
import DockerInstallWizard from './DockerInstallWizard';
import ContainerTab from './ContainerTab';
import ImageTab from './ImageTab';
import ComposeTab from './ComposeTab';
import VolumeTab from './VolumeTab';
import NetworkTab from './NetworkTab';

const ENGINES = ['docker', 'podman'];

export default function Container() {
  const [statuses, setStatuses] = useState<Record<string, DockerStatus | null>>({ docker: null, podman: null });
  const [checking, setChecking] = useState(true);

  // Detect both runtimes on mount.
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

  if (checking) {
    return (
      <div style={{ textAlign: 'center', padding: '100px 0' }}>
        <Spin size="large" description="检测容器运行环境..." />
      </div>
    );
  }

  const resourceTabs = (engine: string) => [
    { key: 'containers', label: <span><CodeOutlined /> 容器</span>, children: <ContainerTab engine={engine} /> },
    { key: 'images', label: <span><CloudDownloadOutlined /> 镜像</span>, children: <ImageTab engine={engine} /> },
    { key: 'compose', label: <span><FolderOutlined /> Compose</span>, children: <ComposeTab engine={engine} /> },
    { key: 'volumes', label: <span><DatabaseOutlined /> 存储卷</span>, children: <VolumeTab engine={engine} /> },
    { key: 'networks', label: <span><GlobalOutlined /> 网络</span>, children: <NetworkTab engine={engine} /> },
  ];

  return (
    <div>
      <h2>容器管理</h2>
      <Tabs
        items={ENGINES.map((r) => {
          const ready = statuses[r]?.installed && statuses[r]?.running;
          return {
            key: r,
            label: <span><DockerOutlined /> {r === 'podman' ? 'Podman' : 'Docker'}</span>,
            children: ready
              ? <Tabs items={resourceTabs(r)} />
              : <DockerInstallWizard engine={r} onInstalled={checkRuntimes} />,
          };
        })}
      />
    </div>
  );
}