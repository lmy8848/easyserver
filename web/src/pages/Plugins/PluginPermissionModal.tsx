import { useEffect, useState } from 'react';
import { Modal, Checkbox, Alert, message } from 'antd';
import { pluginsApi } from '../../services/plugins';
import type { PluginRecord } from '../../types';

// 插件启用时的权限审批（§11 权限模型）：列出声明权限，管理员可收缩子集，
// 不可超出插件声明范围；批准后由后端重新生成 token 并启动插件。
export default function PluginPermissionModal({ plugin, open, onClose }: {
  plugin: PluginRecord | null;
  open: boolean;
  onClose: () => void;
}) {
  const [declared, setDeclared] = useState<string[]>([]);
  const [checked, setChecked] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || !plugin) return;
    setLoading(true);
    setDeclared([]);
    setChecked([]);
    pluginsApi.getManifest(plugin.name)
      .then((res) => {
        const perms = (res.data.data?.['permissions'] as string[]) || [];
        setDeclared(perms);
        setChecked(perms); // 默认全选，与「启用即全批」一致，可勾选收缩
      })
      .catch((e: unknown) => {
        message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '获取权限清单失败');
      })
      .finally(() => setLoading(false));
  }, [open, plugin]);

  const handleOk = async () => {
    if (!plugin) return;
    setSaving(true);
    try {
      await pluginsApi.approve(plugin.name, checked);
      message.success('权限已批准，插件已启动');
      onClose();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '批准失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title={`批准插件「${plugin?.name}」的权限`}
      open={open}
      onCancel={onClose}
      onOk={handleOk}
      confirmLoading={saving}
      okText="批准并启动"
      width={480}
    >
      <Alert
        type="info" showIcon style={{ marginBottom: 12 }}
        message="启用插件即批准勾选的能力；只勾选需要项可最小化授权（不能超出插件声明）。"
      />
      <Checkbox.Group
        style={{ display: 'flex', flexDirection: 'column', gap: 10 }}
        value={checked}
        onChange={(v) => setChecked(v as string[])}
        options={declared.map((p) => ({ label: <code>{p}</code>, value: p }))}
      />
      {loading && <div style={{ marginTop: 8, color: 'var(--text-muted, #999)' }}>加载权限清单…</div>}
    </Modal>
  );
}
