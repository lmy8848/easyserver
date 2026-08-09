import { useState, useEffect, useCallback } from 'react';
import {
  Card, Form, Input, Button, message, Divider, List, Popconfirm, Tag, Alert,
} from 'antd';
import { DeleteOutlined, PlusOutlined, ReloadOutlined, LogoutOutlined } from '@ant-design/icons';
import api from '../../services/api';
import { useAsyncRun } from '../../hooks/useAsyncRun';
import { withEngine } from './types';

export default function RegistryTab({ engine }: { engine: string }) {
  const [configForm] = Form.useForm();
  const [authForm] = Form.useForm();
  const [loading, setLoading] = useState(true);
  const [loggedIn, setLoggedIn] = useState<string[]>([]);
  const [saveLoading, runSave] = useAsyncRun();
  const [loginLoading, runLogin] = useAsyncRun();
  const [logoutLoading, setLogoutLoading] = useState<string>('');

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [cfgRes, authRes] = await Promise.all([
        api.get(withEngine('/container/registry', engine)),
        api.get(withEngine('/container/registry/auth', engine)),
      ]);
      const cfg = cfgRes.data?.data || {};
      configForm.setFieldsValue({
        mirrors: (cfg.mirrors || []).map((v: string) => ({ value: v })),
        insecure: (cfg.insecure_registries || []).map((v: string) => ({ value: v })),
      });
      setLoggedIn(authRes.data?.data?.registries || []);
    } catch {
      message.error('加载镜像仓库配置失败');
    } finally {
      setLoading(false);
    }
  }, [engine, configForm]);

  useEffect(() => { load(); }, [load]);

  const handleSave = async () => {
    try {
      const values = await configForm.validateFields();
      const mirrors = (values.mirrors || []).map((i: { value?: string }) => i.value?.trim()).filter(Boolean);
      const insecure_registries = (values.insecure || []).map((i: { value?: string }) => i.value?.trim()).filter(Boolean);
      await runSave(() => api.post(withEngine('/container/registry', engine), { mirrors, insecure_registries }));
      message.success('镜像仓库配置已保存');
    } catch {
      message.error('保存失败');
    }
  };

  const handleLogin = async () => {
    try {
      const values = await authForm.validateFields();
      await runLogin(() => api.post(withEngine('/container/registry/login', engine), {
        server: values.server, username: values.username, password: values.password,
      }));
      message.success('登录成功');
      authForm.resetFields();
      await load();
    } catch {
      message.error('登录失败');
    }
  };

  const handleLogout = async (server: string) => {
    setLogoutLoading(server);
    try {
      await api.post(withEngine(`/container/registry/logout?server=${encodeURIComponent(server)}`, engine));
      message.success('已退出登录');
      await load();
    } catch {
      message.error('退出失败');
    } finally {
      setLogoutLoading('');
    }
  };

  return (
    <Card
      loading={loading}
      extra={<Button icon={<ReloadOutlined />} onClick={load}>刷新</Button>}
    >
      <Form form={configForm} layout="vertical">
        <Form.Item label="镜像加速源"
          tooltip="拉取镜像时的加速地址，可配多个，按顺序尝试。Docker 对应 daemon.json 的 registry-mirrors。">
          <Form.List name="mirrors">
            {(fields, { add, remove }) => (
              <>
                {fields.map(({ key, name, ...restField }) => (
                  <div key={key} style={{ display: 'flex', gap: 8, alignItems: 'baseline', marginBottom: 8 }}>
                    <Form.Item {...restField} name={[name, 'value']} style={{ marginBottom: 0, flex: 1 }}>
                      <Input placeholder="https://docker.mirrors.ustc.edu.cn" style={{ width: '100%' }} />
                    </Form.Item>
                    <Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(name)} />
                  </div>
                ))}
                <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>添加加速源</Button>
              </>
            )}
          </Form.List>
        </Form.Item>

        <Form.Item label="非安全镜像仓库"
          tooltip="无需 HTTPS 认证的私有仓库，如 registry.local:5000。">
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

        <Button type="primary" loading={saveLoading} onClick={handleSave}>保存配置</Button>
      </Form>

      <Divider style={{ margin: '24px 0 16px' }}>私有仓库登录</Divider>
      <Alert type="info" showIcon style={{ marginBottom: 12 }}
        message="凭据仅用于当前引擎拉取私有镜像，密码经 stdin 传输不落日志。" />

      <Form form={authForm} layout="inline" style={{ rowGap: 12 }}>
        <Form.Item name="server" label="仓库" rules={[{ required: true }]}>
          <Input placeholder="registry.example.com" style={{ width: 220 }} />
        </Form.Item>
        <Form.Item name="username" label="用户名" rules={[{ required: true }]}>
          <Input placeholder="用户名" style={{ width: 140 }} />
        </Form.Item>
        <Form.Item name="password" label="密码" rules={[{ required: true }]}>
          <Input.Password placeholder="密码" style={{ width: 140 }} />
        </Form.Item>
        <Form.Item>
          <Button type="primary" loading={loginLoading} onClick={handleLogin}>登录</Button>
        </Form.Item>
      </Form>

      {loggedIn.length > 0 && (
        <List
          size="small"
          header={<span style={{ fontWeight: 500 }}>已登录仓库</span>}
          style={{ marginTop: 16 }}
          dataSource={loggedIn}
          renderItem={(server) => (
            <List.Item
              actions={[
                <Popconfirm key="logout" title={`确定退出 ${server}？`} onConfirm={() => handleLogout(server)} okText="退出" cancelText="取消">
                  <Button size="small" icon={<LogoutOutlined />} loading={logoutLoading === server}>退出</Button>
                </Popconfirm>,
              ]}
            >
              <Tag color="green">{server}</Tag>
            </List.Item>
          )}
        />
      )}
    </Card>
  );
}