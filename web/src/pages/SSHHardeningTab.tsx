import { useState, useEffect } from 'react';
import {
  Card, Button, Space, message, Table, Input,
  Tag, Popconfirm, Alert, Descriptions, Statistic, Row, Col,
} from 'antd';
import {
  ReloadOutlined, PlusOutlined, DeleteOutlined, ThunderboltOutlined,
} from '@ant-design/icons';
import api from '../services/api';

interface AuthorizedKey { comment: string; type: string; key: string; }
interface Jail { name: string; failed: number; banned: number; }
interface Fail2banStatus { installed: boolean; active: boolean; enabled: boolean; jails: Jail[]; }

export default function SSHHardeningTab() {
  const [hardening, setHardening] = useState(false);
  const [keys, setKeys] = useState<AuthorizedKey[]>([]);
  const [keysLoading, setKeysLoading] = useState(false);
  const [addKey, setAddKey] = useState('');
  const [genName, setGenName] = useState('easyserver-key');
  const [genType, setGenType] = useState('ed25519');
  const [fail2ban, setFail2ban] = useState<Fail2banStatus | null>(null);
  const [failLoading, setFailLoading] = useState(false);

  const loadKeys = async () => {
    setKeysLoading(true);
    try {
      const res = await api.get('/ssh/authorized-keys');
      setKeys(res.data.data?.keys || []);
    } catch { message.error('加载公钥失败'); }
    finally { setKeysLoading(false); }
  };

  const loadFail2ban = async () => {
    setFailLoading(true);
    try {
      const res = await api.get('/ssh/fail2ban');
      setFail2ban(res.data.data);
    } catch { message.error('加载 fail2ban 状态失败'); }
    finally { setFailLoading(false); }
  };

  useEffect(() => { loadKeys(); loadFail2ban(); }, []);

  const onHarden = async () => {
    setHardening(true);
    try {
      await api.post('/ssh/harden', {
        port: 0,
        disable_root_login: true,
        disable_password_auth: true,
        max_auth_tries: 5,
        allow_users: '',
      });
      message.success('SSH 加固成功，配置已应用并重载');
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '加固失败';
      message.error(msg);
    } finally { setHardening(false); }
  };

  const onAddKey = async () => {
    if (!addKey.trim()) { message.warning('请输入公钥'); return; }
    try {
      await api.post('/ssh/authorized-keys', { key: addKey.trim() });
      message.success('公钥已添加');
      setAddKey('');
      loadKeys();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '添加失败');
    }
  };

  const onRemoveKey = async (comment: string) => {
    try {
      await api.delete('/ssh/authorized-keys', { params: { comment } });
      message.success('公钥已删除');
      loadKeys();
    } catch { message.error('删除失败'); }
  };

  const onGenerate = async () => {
    try {
      const res = await api.post('/ssh/keys/generate', { name: genName, key_type: genType });
      const priv = res.data.data?.private_key || '';
      // 下载私钥
      const blob = new Blob([priv], { type: 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${genName || 'easyserver-key'}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      message.success('密钥已生成，私钥已下载，公钥已自动授权');
      loadKeys();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '生成失败');
    }
  };

  const onInstallFail2ban = async () => {
    try {
      await api.post('/ssh/fail2ban/install');
      message.success('fail2ban 已安装');
      loadFail2ban();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '安装失败');
    }
  };

  const onReloadFail2ban = async () => {
    try {
      await api.post('/ssh/fail2ban/reload');
      message.success('fail2ban 已重载');
      loadFail2ban();
    } catch { message.error('重载失败'); }
  };

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Card title={<span><ThunderboltOutlined /> 一键加固</span>}>
        <Alert
          message="一键应用推荐安全配置：禁用 root 登录、禁用密码登录（仅密钥）、MaxAuthTries=5"
          description="自动备份配置 + sshd -t 测试 + 失败回滚，避免锁死。"
          type="info" showIcon style={{ marginBottom: 16 }}
        />
        <Alert
          message="禁用密码登录前需先在下方「SSH 公钥管理」配置至少一个公钥，否则将无法登录"
          type="warning" showIcon style={{ marginBottom: 16 }}
        />
        <Alert
          message="自定义端口 / AllowUsers / 其他 sshd 参数请用「配置」Tab"
          type="info" showIcon style={{ marginBottom: 16 }}
        />
        <Button type="primary" icon={<ThunderboltOutlined />} onClick={onHarden} loading={hardening}>
          一键应用加固
        </Button>
      </Card>

      <Card title="SSH 公钥管理" extra={<Button icon={<ReloadOutlined />} onClick={loadKeys} loading={keysLoading}>刷新</Button>}>
        <Table
          size="small"
          dataSource={keys}
          rowKey={(r) => r.comment || r.key}
          loading={keysLoading}
          locale={{ emptyText: '暂无授权公钥' }}
          columns={[
            { title: '类型', dataIndex: 'type', key: 'type', width: 120, render: (t: string) => <Tag>{t}</Tag> },
            { title: '指纹', dataIndex: 'key', key: 'key', ellipsis: true },
            { title: '备注', dataIndex: 'comment', key: 'comment', ellipsis: true },
            { title: '操作', key: 'action', width: 80, render: (_: unknown, r: AuthorizedKey) => (
              <Popconfirm title="确定删除该公钥？" onConfirm={() => onRemoveKey(r.comment)}>
                <Button type="link" size="small" danger icon={<DeleteOutlined />} />
              </Popconfirm>
            ) },
          ]}
        />
        <Space style={{ marginTop: 16, width: '100%' }} direction="vertical">
          <Input.TextArea
            rows={2} placeholder="粘贴公钥（ssh-ed25519 AAAA... comment）"
            value={addKey} onChange={(e) => setAddKey(e.target.value)}
          />
          <Button icon={<PlusOutlined />} onClick={onAddKey}>添加公钥</Button>
          <Space>
            <Input placeholder="密钥名" value={genName} onChange={(e) => setGenName(e.target.value)} style={{ width: 160 }} />
            <Input placeholder="类型" value={genType} onChange={(e) => setGenType(e.target.value)} style={{ width: 100 }} />
            <Button onClick={onGenerate}>生成密钥对（下载私钥）</Button>
          </Space>
        </Space>
      </Card>

      <Card title="fail2ban 防暴力破解" extra={<Button icon={<ReloadOutlined />} onClick={loadFail2ban} loading={failLoading}>刷新</Button>}>
        {fail2ban && (
          <>
            <Descriptions size="small" column={3} bordered style={{ marginBottom: 16 }}>
              <Descriptions.Item label="已安装">{fail2ban.installed ? <Tag color="green">是</Tag> : <Tag>否</Tag>}</Descriptions.Item>
              <Descriptions.Item label="运行中">{fail2ban.active ? <Tag color="green">是</Tag> : <Tag>否</Tag>}</Descriptions.Item>
              <Descriptions.Item label="开机启用">{fail2ban.enabled ? <Tag color="green">是</Tag> : <Tag>否</Tag>}</Descriptions.Item>
            </Descriptions>
            {fail2ban.installed ? (
              <>
                <Row gutter={16} style={{ marginBottom: 16 }}>
                  {(fail2ban.jails || []).map((j) => (
                    <Col key={j.name} span={8}>
                      <Card size="small" title={j.name}>
                        <Statistic title="失败次数" value={j.failed} />
                        <Statistic title="已封禁" value={j.banned} />
                      </Card>
                    </Col>
                  ))}
                  {(!fail2ban.jails || fail2ban.jails.length === 0) && <Col><Tag>无活动 jail</Tag></Col>}
                </Row>
                <Button icon={<ReloadOutlined />} onClick={onReloadFail2ban}>重载配置</Button>
              </>
            ) : (
              <Space>
                <Alert title="fail2ban 未安装" type="warning" showIcon />
                <Button type="primary" onClick={onInstallFail2ban}>一键安装并启用 sshd 防护</Button>
              </Space>
            )}
          </>
        )}
      </Card>
    </Space>
  );
}
