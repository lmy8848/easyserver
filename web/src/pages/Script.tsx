import { useState, useEffect, useCallback } from 'react';
import {
  Card, Button, Space, Tag, Modal, Form, Input, Select,
  message, Popconfirm, Table, Empty, Tooltip, Collapse, Spin,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined,
  CodeOutlined, CopyOutlined, FileTextOutlined,
} from '@ant-design/icons';
import type { Script } from '../types';
import { cronApi, templateApi } from '../services/api';

const LANG_OPTIONS = [
  { label: 'Shell', value: 'sh' },
  { label: 'Bash', value: 'bash' },
  { label: 'Python', value: 'python' },
];

const LANG_COLORS: Record<string, string> = {
  sh: 'blue',
  bash: 'green',
  python: 'orange',
};

interface ScriptTemplate {
  name: string;
  language: string;
  description: string;
  content: string;
}

interface TemplateCategory {
  name: string;
  templates: ScriptTemplate[];
}

export default function ScriptPage() {
  const [scripts, setScripts] = useState<Script[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [templateModalVisible, setTemplateModalVisible] = useState(false);
  const [editingScript, setEditingScript] = useState<Script | null>(null);
  const [templateCategories, setTemplateCategories] = useState<TemplateCategory[]>([]);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [form] = Form.useForm();

  const fetchScripts = useCallback(async () => {
    setLoading(true);
    try {
      const res = await cronApi.listScripts();
      setScripts(res.data?.data || []);
    } catch (error: any) {
      message.error(error.message || '加载脚本失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { fetchScripts(); }, [fetchScripts]);

  const fetchTemplates = async () => {
    setTemplatesLoading(true);
    try {
      const res = await templateApi.getScriptTemplates();
      setTemplateCategories(res.data?.data?.categories || []);
    } catch (error: any) {
      message.error(error.message || '加载模板失败');
    } finally {
      setTemplatesLoading(false);
    }
  };

  const handleCreate = () => {
    setEditingScript(null);
    form.resetFields();
    form.setFieldsValue({ language: 'sh' });
    setModalVisible(true);
  };

  const handleCreateFromTemplate = () => {
    fetchTemplates();
    setTemplateModalVisible(true);
  };

  const handleSelectTemplate = (template: ScriptTemplate) => {
    setEditingScript(null);
    form.setFieldsValue({
      name: template.name,
      language: template.language,
      description: template.description,
      content: template.content,
    });
    setTemplateModalVisible(false);
    setModalVisible(true);
  };

  const handleEdit = (script: Script) => {
    setEditingScript(script);
    form.setFieldsValue({
      name: script.name,
      description: script.description,
      content: script.content,
      language: script.language,
    });
    setModalVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (editingScript) {
        await cronApi.updateScript(editingScript.id, values);
        message.success('脚本已更新');
      } else {
        await cronApi.createScript(values);
        message.success('脚本已创建');
      }
      setModalVisible(false);
      fetchScripts();
    } catch (error: any) {
      if (error.message) {
        message.error(error.message);
      }
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await cronApi.deleteScript(id);
      message.success('脚本已删除');
      fetchScripts();
    } catch (error: any) {
      message.error(error.message || '删除脚本失败');
    }
  };

  const handleCopyContent = (content: string) => {
    navigator.clipboard.writeText(content).then(() => {
      message.success('已复制到剪贴板');
    });
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 180,
      render: (name: string) => (
        <Space>
          <CodeOutlined />
          <span>{name}</span>
        </Space>
      ),
    },
    {
      title: '语言',
      dataIndex: 'language',
      key: 'language',
      width: 100,
      render: (lang: string) => (
        <Tag color={LANG_COLORS[lang] || 'default'}>{lang}</Tag>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
    },
    {
      title: '更新时间',
      dataIndex: 'updated_at',
      key: 'updated_at',
      width: 180,
    },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      render: (_: any, record: Script) => (
        <Space>
          <Tooltip title="复制内容">
            <Button
              type="link"
              icon={<CopyOutlined />}
              onClick={() => handleCopyContent(record.content)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
            />
          </Tooltip>
          <Popconfirm
            title="确定删除此脚本？"
            description="此操作不可撤销"
            onConfirm={() => handleDelete(record.id)}
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
          >
            <Tooltip title="删除">
              <Button type="link" icon={<DeleteOutlined />} danger />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Card
        title={<Space><CodeOutlined /> 脚本库</Space>}
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchScripts} loading={loading}>刷新</Button>
            <Button icon={<FileTextOutlined />} onClick={handleCreateFromTemplate}>从模板创建</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>创建脚本</Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={scripts}
          rowKey="id"
          loading={loading}
          size="small"
          locale={{ emptyText: <Empty description="暂无脚本" /> }}
        />
      </Card>

      <Modal
        title={editingScript ? '编辑脚本' : '创建脚本'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleSubmit}
        okText={editingScript ? '保存' : '创建'}
        cancelText="取消"
        width={800}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="脚本名称" rules={[{ required: true, message: '请输入脚本名称' }]}>
            <Input placeholder="e.g. backup-db" />
          </Form.Item>
          <Form.Item name="language" label="语言">
            <Select options={LANG_OPTIONS} />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={2} placeholder="可选描述" />
          </Form.Item>
          <Form.Item name="content" label="脚本内容" rules={[{ required: true, message: '请输入脚本内容' }]}>
            <Input.TextArea
              rows={12}
              placeholder="#!/bin/bash&#10;echo 'Hello World'"
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
          </Form.Item>
        </Form>
      </Modal>

      {/* Template Selection Modal */}
      <Modal
        title="选择脚本模板"
        open={templateModalVisible}
        onCancel={() => setTemplateModalVisible(false)}
        footer={null}
        width={700}
        styles={{ body: { maxHeight: '60vh', overflowY: 'auto' } }}
      >
        {templatesLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <Spin size="large" />
          </div>
        ) : templateCategories.length === 0 ? (
          <Empty description="暂无模板" />
        ) : (
          <Collapse
            defaultActiveKey={templateCategories.map((_, i) => String(i))}
            items={templateCategories.map((category, index) => ({
              key: String(index),
              label: <Space><FileTextOutlined /> {category.name}</Space>,
              children: (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {category.templates.map((template, tIndex) => (
                    <Card
                      key={tIndex}
                      size="small"
                      hoverable
                      onClick={() => handleSelectTemplate(template)}
                      style={{ cursor: 'pointer' }}
                    >
                      <Space direction="vertical" style={{ width: '100%' }}>
                        <Space>
                          <Tag color={LANG_COLORS[template.language] || 'default'}>
                            {template.language}
                          </Tag>
                          <span style={{ fontWeight: 500 }}>{template.name}</span>
                        </Space>
                        <div style={{ color: '#666', fontSize: 13 }}>{template.description}</div>
                      </Space>
                    </Card>
                  ))}
                </div>
              ),
            }))}
          />
        )}
      </Modal>
    </div>
  );
}
