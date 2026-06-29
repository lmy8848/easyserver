import { useState, useEffect } from 'react';
import { Modal, Select, Input, message } from 'antd';
import api from '../../services/api';
import type { RuntimeEnvironment } from './types';

interface PackageRegistryModalProps {
  visible: boolean;
  runtime: RuntimeEnvironment | null;
  onClose: () => void;
}

export default function PackageRegistryModal({
  visible,
  runtime,
  onClose,
}: PackageRegistryModalProps) {
  const [manager, setManager] = useState<string>('npm');
  const [registryUrl, setRegistryUrl] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (visible && runtime) {
      const defaultManager = runtime.name === 'node' ? 'npm' : 'pip';
      setManager(defaultManager);
      fetchRegistry(defaultManager);
    }
  }, [visible, runtime]);

  async function fetchRegistry(targetManager: string) {
    if (!runtime) return;
    setLoading(true);
    try {
      const res = await api.get(`/packages/registry?runtime_id=${runtime.id}&manager=${targetManager}`);
      let url = res.data.data?.registry || '';
      // UX improvement: if the registry is the npm default, show it as empty
      // so the placeholder is visible and the user intuitively sees "default"
      if (targetManager === 'npm' || targetManager === 'pnpm') {
        if (url === 'https://registry.npmjs.org/') {
          url = '';
        }
      }
      setRegistryUrl(url);
    } catch (error: any) {
      message.error(error.message || '获取镜像配置失败');
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    if (!runtime) return;
    setSaving(true);
    try {
      await api.post('/packages/registry', {
        runtime_id: runtime.id,
        manager: manager,
        registry: registryUrl,
      });
      message.success('配置保存成功');
      onClose();
    } catch (error: any) {
      message.error(error.message || '保存镜像配置失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title="配置包管理器镜像"
      open={visible}
      onCancel={onClose}
      onOk={handleSave}
      okText="保存"
      cancelText="取消"
      confirmLoading={saving}
      destroyOnHidden
    >
      <div style={{ marginBottom: 16 }}>
        {runtime?.name === 'node' && (
          <div style={{ marginBottom: 16 }}>
            <div style={{ marginBottom: 8 }}>包管理器:</div>
            <Select
              value={manager}
              onChange={(val) => {
                setManager(val);
                fetchRegistry(val);
              }}
              style={{ width: 120 }}
            >
              <Select.Option value="npm">npm</Select.Option>
              <Select.Option value="pnpm">pnpm</Select.Option>
            </Select>
          </div>
        )}
        <div style={{ marginBottom: 8 }}>镜像地址:</div>
        <Input
          value={registryUrl}
          onChange={e => setRegistryUrl(e.target.value)}
          placeholder="留空则恢复默认配置"
          disabled={loading}
        />
      </div>
    </Modal>
  );
}
