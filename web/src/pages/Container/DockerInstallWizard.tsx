import { useState, useEffect } from 'react';
import { Tag, Button, Space, message, Spin, Descriptions } from 'antd';
import {
  PlayCircleOutlined, ReloadOutlined, RocketOutlined, DockerOutlined,
} from '@ant-design/icons';
import api from '../../services/api';
import type { DockerStatus } from './types';

// 安装步骤
const INSTALL_STEPS = [
  { key: 'download', label: '下载安装脚本' },
  { key: 'install', label: '安装 Docker 引擎' },
  { key: 'enable', label: '启用 Docker 服务' },
  { key: 'verify', label: '验证安装结果' },
];

export default function DockerInstallWizard({ onInstalled }: { onInstalled: () => void }) {
  const [installing, setInstalling] = useState(false);
  const [currentStep, setCurrentStep] = useState(-1);
  const [status, setStatus] = useState<DockerStatus | null>(null);
  const [error, setError] = useState('');

  const checkStatus = async () => {
    try {
      const res = await api.get('/docker/status');
      setStatus(res.data?.data);
    } catch {
      // ignore
    }
  };

  useEffect(() => { checkStatus(); }, []);

  // 轮询验证安装结果
  const verifyInstall = async (): Promise<boolean> => {
    for (let i = 0; i < 10; i++) {
      await new Promise(r => setTimeout(r, 2000));
      try {
        const res = await api.get('/docker/status');
        const s = res.data?.data;
        if (s?.installed && s?.running) {
          setStatus(s);
          return true;
        }
      } catch {
        // continue polling
      }
    }
    return false;
  };

  const handleInstall = async () => {
    setInstalling(true);
    setError('');
    setCurrentStep(0);

    try {
      // Step 1: 下载（短暂延迟给用户视觉反馈，后端安装脚本会自动下载）
      await new Promise(r => setTimeout(r, 800));
      setCurrentStep(1);

      // Step 2: 安装（调用后端 API，等待完成）
      await api.post('/docker/install');
      setCurrentStep(2);

      // Step 3: 启用服务（短暂延迟给用户视觉反馈）
      await new Promise(r => setTimeout(r, 300));
      setCurrentStep(3);

      // Step 4: 验证安装
      const verified = await verifyInstall();
      if (verified) {
        message.success('Docker 安装成功！');
        onInstalled();
      } else {
        setError('安装完成但验证失败，请手动检查 Docker 状态');
      }
    } catch (err: unknown) {
      const axiosErr = err as { response?: { data?: { message?: string } }; message?: string };
      const msg = axiosErr?.response?.data?.message || axiosErr?.message || '未知错误';
      setError(`安装失败：${msg}`);
    } finally {
      setInstalling(false);
    }
  };

  const handleStart = async () => {
    try {
      await api.post('/docker/start');
      message.success('Docker 已启动');
      await checkStatus();
      onInstalled();
    } catch {
      message.error('启动 Docker 失败');
    }
  };

  return (
    <div style={{ textAlign: 'center', padding: '60px 0' }}>
      <DockerOutlined style={{ fontSize: 64, color: '#1890ff', marginBottom: 24 }} />
      <h2>Docker 环境配置</h2>

      {status && (
        <Descriptions column={1} style={{ maxWidth: 400, margin: '24px auto', textAlign: 'left' }}>
          <Descriptions.Item label="安装状态">
            <Tag color={status.installed ? 'green' : 'red'}>
              {status.installed ? '已安装' : '未安装'}
            </Tag>
          </Descriptions.Item>
          {status.installed && (
            <>
              <Descriptions.Item label="版本">{status.version || '-'}</Descriptions.Item>
              <Descriptions.Item label="Compose">{status.compose_version || '-'}</Descriptions.Item>
              <Descriptions.Item label="运行状态">
                <Tag color={status.running ? 'green' : 'red'}>
                  {status.running ? '运行中' : '已停止'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="系统">{status.os}</Descriptions.Item>
            </>
          )}
        </Descriptions>
      )}

      {/* 安装进度 */}
      {installing && (
        <div style={{ maxWidth: 400, margin: '24px auto', textAlign: 'left' }}>
          {INSTALL_STEPS.map((step, index) => (
            <div key={step.key} style={{ display: 'flex', alignItems: 'center', marginBottom: 12 }}>
              <div style={{
                width: 24, height: 24, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center',
                background: index < currentStep ? '#52c41a' : index === currentStep ? '#1890ff' : '#d9d9d9',
                color: '#fff', fontSize: 12, marginRight: 12,
              }}>
                {index < currentStep ? '✓' : index === currentStep ? '...' : index + 1}
              </div>
              <span style={{ color: index <= currentStep ? '#000' : '#999' }}>
                {step.label}
                {index === currentStep && <Spin size="small" style={{ marginLeft: 8 }} />}
              </span>
            </div>
          ))}
        </div>
      )}

      {/* 错误提示 */}
      {error && (
        <div style={{ color: '#ff4d4f', margin: '16px 0', padding: '8px 16px', background: '#fff2f0', borderRadius: 4, maxWidth: 400, marginInline: 'auto' }}>
          {error}
        </div>
      )}

      <Space>
        {!status?.installed ? (
          <Button type="primary" icon={<RocketOutlined />} size="large" loading={installing} onClick={handleInstall} disabled={installing}>
            {installing ? '安装中...' : '安装 Docker'}
          </Button>
        ) : !status.running ? (
          <Button type="primary" icon={<PlayCircleOutlined />} size="large" onClick={handleStart}>
            启动 Docker
          </Button>
        ) : (
          <Button type="primary" icon={<ReloadOutlined />} size="large" onClick={onInstalled}>
            进入管理
          </Button>
        )}
        <Button icon={<ReloadOutlined />} onClick={checkStatus} disabled={installing}>刷新状态</Button>
      </Space>
    </div>
  );
}
