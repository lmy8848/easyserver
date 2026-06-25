import { useState, useEffect, useCallback } from 'react';
import { Tabs, Spin } from 'antd';
import {
  CodeOutlined, CloudDownloadOutlined,
  DatabaseOutlined, GlobalOutlined, FolderOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { DockerStatus } from './types';
import DockerInstallWizard from './DockerInstallWizard';
import ContainerTab from './ContainerTab';
import ImageTab from './ImageTab';
import ComposeTab from './ComposeTab';
import VolumeTab from './VolumeTab';
import NetworkTab from './NetworkTab';

export default function Container() {
  const [dockerReady, setDockerReady] = useState(false);
  const [checking, setChecking] = useState(true);

  // Check Docker status on mount
  const checkDocker = useCallback(async () => {
    setChecking(true);
    try {
      const res = await api.get('/docker/status');
      const status: DockerStatus = res.data?.data;
      setDockerReady(status?.installed && status?.running);
    } catch {
      setDockerReady(false);
    } finally {
      setChecking(false);
    }
  }, []);

  useEffect(() => { checkDocker(); }, [checkDocker]);

  if (checking) {
    return (
      <div style={{ textAlign: 'center', padding: '100px 0' }}>
        <Spin size="large" tip="检测 Docker 环境..." />
      </div>
    );
  }

  if (!dockerReady) {
    return (
      <div>
        <h2>容器管理</h2>
        <DockerInstallWizard onInstalled={checkDocker} />
      </div>
    );
  }

  return (
    <div>
      <h2>容器管理</h2>
      <Tabs
        items={[
          { key: 'containers', label: <span><CodeOutlined /> 容器</span>, children: <ContainerTab /> },
          { key: 'images', label: <span><CloudDownloadOutlined /> 镜像</span>, children: <ImageTab /> },
          { key: 'compose', label: <span><FolderOutlined /> Compose</span>, children: <ComposeTab /> },
          { key: 'volumes', label: <span><DatabaseOutlined /> 存储卷</span>, children: <VolumeTab /> },
          { key: 'networks', label: <span><GlobalOutlined /> 网络</span>, children: <NetworkTab /> },
        ]}
      />
    </div>
  );
}
