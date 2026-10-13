import { useCallback, useEffect, useState } from 'react';
import {
  Card, Table, Button, Space, Input, Form, Popconfirm, Tag, message, Alert,
} from 'antd';
import { ReloadOutlined, DeleteOutlined, DownloadOutlined, PlusOutlined } from '@ant-design/icons';
import { pluginsApi } from '../../services/plugins';
import type { PluginSource, PluginSourceCatalog } from '../../types';

// 插件源（市场入口）：配置插件源 → 拉取插件清单 → 从源一键安装。
// 源是暴露 index.json 的 HTTP 端点；安装复用面板的 zip 安全校验。
export default function PluginSources({ onInstalled }: { onInstalled?: () => void }) {
  const [sources, setSources] = useState<PluginSource[]>([]);
  const [loading, setLoading] = useState(false);
  const [catalog, setCatalog] = useState<PluginSourceCatalog | null>(null);
  const [catalogSource, setCatalogSource] = useState<PluginSource | null>(null);
  const [fetching, setFetching] = useState(false);
  const [installing, setInstalling] = useState('');
  const [form] = Form.useForm();

  const fetchSources = useCallback(async () => {
    setLoading(true);
    try {
      const res = await pluginsApi.listSources();
      setSources(res.data.data?.items || []);
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '获取插件源失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void fetchSources(); }, [fetchSources]);

  const handleAdd = async () => {
    try {
      const values = await form.validateFields();
      await pluginsApi.addSource(values);
      message.success('插件源已添加');
      form.resetFields();
      void fetchSources();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || (e as Error)?.message;
      if (msg) message.error(msg);
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await pluginsApi.deleteSource(id);
      message.success('插件源已删除');
      if (catalogSource?.id === id) {
        setCatalog(null);
        setCatalogSource(null);
      }
      void fetchSources();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '删除失败');
    }
  };

  const handleFetch = async (src: PluginSource) => {
    setFetching(true);
    try {
      const res = await pluginsApi.fetchSourceCatalog(src.id);
      setCatalog(res.data.data?.catalog || null);
      setCatalogSource(src);
    } catch (e: unknown) {
      setCatalog(null);
      setCatalogSource(null);
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '拉取清单失败');
    } finally {
      setFetching(false);
    }
  };

  const handleInstall = async (item: { name: string; download_url: string }) => {
    setInstalling(item.name);
    try {
      await pluginsApi.installFromSource({ url: item.download_url });
      message.success(`插件 ${item.name} 已安装`);
      onInstalled?.();
    } catch (e: unknown) {
      message.error((e as { response?: { data?: { message?: string } } })?.response?.data?.message || '安装失败');
    } finally {
      setInstalling('');
    }
  };

  return (
    <div>
      <Alert
        type="info" showIcon
        message="插件源是暴露 index.json 的 HTTP 端点（插件市场入口）。添加源后「拉取清单」浏览插件，「安装」下载并安全校验后安装到面板。"
        style={{ marginBottom: 16 }}
      />

      <Card size="small" title="插件源管理" style={{ marginBottom: 16 }}>
        <Form form={form} layout="inline" style={{ marginBottom: 16 }} onFinish={handleAdd}>
          <Form.Item name="name" rules={[{ required: true, message: '请输入源名称' }]}>
            <Input placeholder="源名称（如 official）" style={{ width: 200 }} />
          </Form.Item>
          <Form.Item name="url" rules={[{ required: true, message: '请输入源地址' }]}>
            <Input placeholder="https://plugins.example.com" style={{ width: 320 }} />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" icon={<PlusOutlined />}>添加源</Button>
          </Form.Item>
          <Button icon={<ReloadOutlined />} onClick={fetchSources} loading={loading}>刷新</Button>
        </Form>

        <Table
          size="small"
          rowKey="id"
          loading={loading}
          dataSource={sources}
          pagination={false}
          locale={{ emptyText: '暂无插件源。上方添加一个暴露 index.json 的插件源地址。' }}
          columns={[
            { title: '名称', dataIndex: 'name', key: 'name' },
            { title: '地址', dataIndex: 'url', key: 'url', ellipsis: true },
            {
              title: '状态', dataIndex: 'enabled', key: 'enabled', width: 90,
              render: (en: boolean) => en ? <Tag color="success">启用</Tag> : <Tag>禁用</Tag>,
            },
            {
              title: '操作', key: 'action', width: 200,
              render: (_: unknown, r: PluginSource) => (
                <Space size="small">
                  <Button size="small" loading={fetching} onClick={() => handleFetch(r)}>拉取清单</Button>
                  <Popconfirm title="删除该插件源？" onConfirm={() => handleDelete(r.id)}>
                    <Button size="small" danger icon={<DeleteOutlined />} />
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      {catalogSource && (
        <Card
          size="small"
          title={`插件清单 - ${catalogSource.name}`}
          extra={catalog?.name ? <Tag>{catalog.name}</Tag> : null}
        >
          <Table
            size="small"
            rowKey="name"
            dataSource={catalog?.plugins || []}
            pagination={false}
            locale={{ emptyText: '该源未返回插件（index.json 格式：{name, plugins:[{name, version, description, download_url}]}）' }}
            columns={[
              { title: '插件', dataIndex: 'name', key: 'name' },
              { title: '版本', dataIndex: 'version', key: 'version', width: 100 },
              { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
              {
                title: '操作', key: 'action', width: 110,
                render: (_: unknown, item: { name: string; download_url: string }) => (
                  <Button
                    size="small" type="primary" icon={<DownloadOutlined />}
                    loading={installing === item.name}
                    disabled={!item.download_url}
                    onClick={() => handleInstall(item)}
                  >
                    安装
                  </Button>
                ),
              },
            ]}
          />
        </Card>
      )}
    </div>
  );
}
