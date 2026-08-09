import { useEffect } from 'react';
import {
  Modal, Form, Input, Button, Space, message, Divider, Alert,
} from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import api from '../../services/api';
import { useAsyncRun } from '../../hooks/useAsyncRun';
import { withEngine } from './types';

interface RegistrySettingsModalProps {
  engine: string;
  open: boolean;
  onClose: () => void;
}

export default function RegistrySettingsModal({ engine, open, onClose }: RegistrySettingsModalProps) {
  const [configForm] = Form.useForm();
  const [authForm] = Form.useForm();
  const [saveLoading, runSave] = useAsyncRun();
  const [loginLoading, runLogin] = useAsyncRun();
  const [logoutLoading, runLogout] = useAsyncRun();

  useEffect(() => {
    if (!open) return;
    const load = async () => {
      try {
        const res = await api.get(withEngine('/container/registry', engine));
        const cfg = res.data?.data || {};
        configForm.setFieldsValue({
          mirror: cfg.mirror || '',
          insecure: (cfg.insecure_registries || []).map((v: string) => ({ value: v })),
        });
      } catch {
        message.error('加载镜像仓库配置失败');
      }
    };
    load();
  }, [open, engine, configForm]);

  const handleSave = async () => {
    try {
      const values = await configForm.validateFields();
      const insecure_registries = (values.insecure || [])
        .map((i: { value?: string }) => i.value?.trim())
        .filter(Boolean);
      await runSave(() => api.post(withEngine('/container/registry', engine), {
        mirror: values.mirror || '',
        insecure_registries,
      }));
      message.success('镜像仓库配置已保存');
    } catch {
      message.error('保存失败');
    }
  };

  const handleLogin = async () => {
    try {
      const values = await authForm.validateFields();
      await runLogin(() => api.post(withEngine('/container/registry/login', engine), {
        server: values.server,
        username: values.username,
        password: values.password,
      }));
      message.success('登录成功');
      authForm.resetFields();
    } catch {
      message.error('登录失败');
    }
  };

  const handleLogout = async () => {
    const values = await authForm.validateFields(['server']).catch(() => null);
    if (!values) return;
    try {
      await runLogout(() => api.post(withEngine(`/container/registry/logout?server=${encodeURIComponent(values.server)}`, engine)));
      message.success('已退出登录');
    } catch {
      message.error('退出失败');
    }
  };

  return (
    <Modal title={`镜像仓库设置（${engine === 'podman' ? 'Podman' : 'Docker'}）`} open={open} onCancel={onClose}
      onOk={handleSave} confirmLoading={saveLoading} width={560} destroyOnHidden>
      <Form form={configForm} layout="vertical" style={{ marginTop: 8 }}>
        <Form.Item name="mirror" label="镜像加速源" tooltip="留空则清除加速源">
          <Input placeholder="如 https://docker.mirrors.ustc.edu.cn" />
        </Form.Item>
        <Form.Item label="非安全镜像仓库" tooltip="无需 HTTPS 认证的私有仓库，如 registry.local:5000">
          <Form.List name="insecure">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => (
                  <div key={key} style={{ display: 'flex', gap: 8, alignItems: 'baseline', marginBottom: 8 }}>
                    <Form.Item {...restField} name={[name, 'value']} style={{ marginBottom: 0, flex: 1 }}>
                      <Input placeholder="registry.example.com:5000" style={{ width: '100%' }} />
                    </Form.Item>
                    <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(name)} />
                  </div>
                ))}
                <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>添加仓库</Button>
              </>
            )}
          </Form.List>
        </Form.Item>
      </Form>

      <Divider style={{ margin: '16px 0' }}>私有仓库登录</Divider>
      <Alert type="info" showIcon style={{ marginBottom: 12 }}
        message="登录凭据仅用于当前引擎拉取私有镜像，密码经 stdin 传输不落日志。" />

      <Form form={authForm} layout="vertical">
        <Form.Item name="server" label="仓库地址" rules={[{ required: true }]}>
          <Input placeholder="registry.example.com" />
        </Form.Item>
        <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
          <Input placeholder="用户名" />
        </Form.Item>
        <Form.Item name="password" label="密码" rules={[{ required: true }]}>
          <Input.Password placeholder="密码" />
        </Form.Item>
        <Space>
          <Button type="primary" loading={loginLoading} onClick={handleLogin}>登录</Button>
          <Button danger loading={logoutLoading} onClick={handleLogout}>退出登录</Button>
        </Space>
      </Form>
    </Modal>
  );
}