import { useCallback, useEffect, useState } from 'react';
import {
  Card, Button, Table, Tag, Space, message, Popconfirm, Tooltip, Badge, Tabs, Upload, Alert,
} from 'antd';
import {
  ReloadOutlined, PoweroffOutlined, PlayCircleOutlined, DeleteOutlined, ImportOutlined, BookOutlined, SettingOutlined,
} from '@ant-design/icons';
import { pluginsApi } from '../../services/plugins';
import type { PluginRecord } from '../../types';
import PluginManual from './PluginManual';
import PluginConfigModal from './PluginConfigModal';

export default function Plugins() {
  const [plugins, setPlugins] = useState<PluginRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [operating, setOperating] = useState('');
  const [installing, setInstalling] = useState(false);
  const [configTarget, setConfigTarget] = useState<PluginRecord | null>(null);

  const fetchPlugins = useCallback(async () => {
    setLoading(true);
    try {
      const res = await pluginsApi.list();
      setPlugins(res.data.data?.items || []);
    } catch (e: unknown) {
      message.error(e instanceof Error ? e.message : '获取插件列表失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void fetchPlugins(); }, [fetchPlugins]);

  const act = async (key: string, fn: () => Promise<unknown>, okMsg: string) => {
    setOperating(key);
    try {
      await fn();
      message.success(okMsg);
      await fetchPlugins();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '操作失败');
    } finally {
      setOperating('');
    }
  };

  const handleInstall = (opt: { file: unknown }) => {
    const fd = new FormData();
    fd.append('file', opt.file as File);
    setInstalling(true);
    pluginsApi.upload(fd).then((res) => {
      message.success(`插件 ${res.data.data?.plugin}@${res.data.data?.version} 已安装`);
      setInstalling(false);
      void fetchPlugins();
    }).catch((e: unknown) => {
      setInstalling(false);
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '安装失败');
    });
  };

  const listTab = (
    <>
      <Alert
        message="导入安装：上传插件 zip 包（含 plugin.yaml 与可执行文件），校验通过后自动安装到 /opt/easyserver/plugins/ 并注册。安装后需在列表中「启用」才会启动进程。"
        type="info" showIcon style={{ marginBottom: 16 }}
      />
      <Table
        size="small"
        loading={loading}
        dataSource={plugins}
        rowKey="name"
        pagination={false}
        locale={{ emptyText: '暂无插件。点击右上角「导入插件」上传 zip 包，或将插件目录放入 /opt/easyserver/plugins/ 后重启面板。' }}
        columns={[
          { title: '名称', dataIndex: 'name', key: 'name', render: (n: string) => <strong>{n}</strong> },
          { title: '版本', dataIndex: 'version', key: 'version', width: 90 },
          { title: 'API', dataIndex: 'api_version', key: 'api_version', width: 70 },
          {
            title: '状态', dataIndex: 'status', key: 'status', width: 110,
            render: (s: string) => <Badge status={s === 'running' ? 'success' : s === 'error' || s === 'degraded' ? 'error' : 'default'} text={s} />,
          },
          {
            title: '启用', dataIndex: 'enabled', key: 'enabled', width: 90,
            render: (en: boolean) => en ? <Tag color="success">是</Tag> : <Tag>否</Tag>,
          },
          {
            title: '权限', dataIndex: 'permissions_approved', key: 'perms', ellipsis: true,
            render: (perms: string[]) => perms?.length ? <Space size={4} wrap>{perms.map((p) => <Tag key={p} style={{ margin: 0 }}>{p}</Tag>)}</Space> : '-',
          },
          { title: '重启次数', dataIndex: 'restart_count', key: 'restarts', width: 90 },
          {
            title: '最后错误', dataIndex: 'last_error', key: 'last_error', ellipsis: true,
            render: (v: string) => v || '-',
          },
          {
            title: '操作', key: 'action', width: 190,
            render: (_: unknown, r: PluginRecord) => (
              <Space size="small">
                <Tooltip title="配置">
                  <Button
                    type="link" size="small" icon={<SettingOutlined />}
                    onClick={() => setConfigTarget(r)}
                  />
                </Tooltip>
                <Tooltip title={r.enabled ? '启动进程' : '启用并启动'}>
                  <Button
                    type="link" size="small" icon={<PlayCircleOutlined />}
                    loading={operating === `start-${r.name}`}
                    onClick={() => act(`start-${r.name}`, () => (r.enabled ? pluginsApi.start(r.name) : pluginsApi.enable(r.name)), '已启动')}
                  />
                </Tooltip>
                <Tooltip title="停止进程">
                  <Button
                    type="link" size="small" icon={<PoweroffOutlined />}
                    disabled={!r.enabled}
                    loading={operating === `stop-${r.name}`}
                    onClick={() => act(`stop-${r.name}`, () => pluginsApi.stop(r.name), '已停止')}
                  />
                </Tooltip>
                <Popconfirm title="卸载将删除注册/配置/密钥（插件目录文件保留），确认？" onConfirm={() => act(`rm-${r.name}`, () => pluginsApi.remove(r.name), '已卸载')}>
                  <Tooltip title="卸载">
                    <Button type="link" size="small" danger icon={<DeleteOutlined />} loading={operating === `rm-${r.name}`} />
                  </Tooltip>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />
      <div style={{ marginTop: 12, fontSize: 12, opacity: 0.65 }}>
        插件以独立进程运行（崩溃自动重启，不影响面板）；启用即表示批准其声明的全部权限。
        若插件贡献了菜单页面，会在侧边栏「插件」分组出现。
      </div>
    </>
  );

  return (
    <Card
      title="插件管理"
      extra={
        <Space>
          <Upload
            accept=".zip"
            showUploadList={false}
            disabled={installing}
            customRequest={handleInstall}
          >
            <Button icon={<ImportOutlined />} loading={installing}>导入插件</Button>
          </Upload>
          <Button icon={<ReloadOutlined />} onClick={fetchPlugins} loading={loading}>刷新</Button>
        </Space>
      }
    >
      <Tabs
        items={[
          { key: 'list', label: '插件列表', children: listTab },
          { key: 'manual', label: <span><BookOutlined /> 插件开发手册</span>, children: <PluginManual /> },
        ]}
      />
      <PluginConfigModal plugin={configTarget!} open={!!configTarget} onClose={() => setConfigTarget(null)} />
    </Card>
  );
}
