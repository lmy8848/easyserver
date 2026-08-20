import { useState, useEffect, useCallback } from 'react';
import {
  Card, Button, Space, Modal, Form, Input,
  message, Popconfirm, Table, Empty, Tooltip, Collapse,
  Select, Tag, Drawer,
} from 'antd';
import {
  PlusOutlined, ReloadOutlined, DeleteOutlined, EditOutlined,
  CodeOutlined, FileTextOutlined, DownloadOutlined,
  PlayCircleOutlined, StopOutlined, HistoryOutlined,
} from '@ant-design/icons';
import type { Script, ScriptLogLine } from '../types';
import { cronApi } from '../services/cron';
import { SCRIPT_TEMPLATES, type ScriptTemplate } from '../constants/templates';
import { LogViewer, useLogBuffer } from '../components/LogViewer';

// 历史日志可选条数
const HISTORY_LIMITS = [50, 200, 500];

export default function ScriptPage() {
  const [scripts, setScripts] = useState<Script[]>([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [templateModalVisible, setTemplateModalVisible] = useState(false);
  const [editingScript, setEditingScript] = useState<Script | null>(null);
  const [form] = Form.useForm();

  // ── 运行中脚本 id（刷新后显示「运行中」标记）──
  const [runningIds, setRunningIds] = useState<number[]>([]);

  // ── 日志 Drawer（实时 + 历史 合一）──
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [stream, setStream] = useState(false); // 是否连 SSE 实时（执行/运行中脚本才连）
  const [drawerScript, setDrawerScript] = useState<Script | null>(null);
  const [historyLimit, setHistoryLimit] = useState(200);
  const logBuffer = useLogBuffer();

  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);

  const fetchScripts = useCallback(async () => {
    setLoading(true);
    try {
      const res = await cronApi.listScripts(page, pageSize);
      setScripts(res.data?.data?.items ?? []);
      setTotal(res.data?.data?.total ?? 0);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '加载脚本失败'));
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  const fetchRunning = useCallback(async () => {
    try {
      const res = await cronApi.getRunningScripts();
      setRunningIds(res.data?.data || []);
    } catch {
      // 忽略，运行中标记非关键
    }
  }, []);

  useEffect(() => {
    fetchScripts();
    fetchRunning();
  }, [fetchScripts, fetchRunning]);

  const handleCreate = () => {
    setEditingScript(null);
    form.resetFields();
    setModalVisible(true);
  };

  const handleCreateFromTemplate = () => {
    setTemplateModalVisible(true);
  };

  const handleSelectTemplate = (template: ScriptTemplate) => {
    setEditingScript(null);
    form.setFieldsValue({
      name: template.name,
      description: template.description,
      content: template.content,
    });
    setTemplateModalVisible(false);
    setModalVisible(true);
  };

  const handleEdit = async (script: Script) => {
    setEditingScript(script);
    // 列表不加载文件内容，编辑时需拉取完整脚本内容
    form.setFieldsValue({
      name: script.name,
      description: script.description,
      content: script.content,
    });
    setModalVisible(true);
    try {
      const res = await cronApi.getScript(script.id);
      const full = res.data?.data;
      if (full) {
        form.setFieldsValue({ name: full.name, description: full.description, content: full.content });
      }
    } catch {
      // 内容拉取失败时保留列表已有字段（name/description），content 留空提示
    }
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
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await cronApi.deleteScript(id);
      message.success('脚本已删除');
      fetchScripts();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除脚本失败'));
    }
  };

  const handleDownload = async (script: Script) => {
    try {
      const res = await cronApi.getScript(script.id);
      const full = res.data?.data;
      if (!full?.content) throw new Error('脚本内容为空');
      const blob = new Blob([full.content], { type: 'text/plain;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = script.name.endsWith('.sh') ? script.name : `${script.name}.sh`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '下载脚本失败');
    }
  };

  // ── 历史日志拉取（Drawer 打开时作为底，运行中续看时补历史）──
  const fetchHistory = useCallback(async (script: Script, limit: number) => {
    try {
      const res = await cronApi.getScriptLogs(script.id, limit);
      const items: ScriptLogLine[] = res.data?.data || [];
      logBuffer.setEntries(
        items.map((l) => ({
          text: l.message,
          time: l.time,
          level: l.stream === 'stderr' ? 'stderr' : 'stdout',
        }))
      );
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '加载历史日志失败');
    }
  }, [logBuffer]);

  // ── 执行脚本：先经 REST 启动，再打开 Drawer 连 SSE 订阅实时输出（两步解耦）──
  const handleRun = async (script: Script) => {
    try {
      await cronApi.runScript(script.id); // 独立启动步骤
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '启动脚本失败');
      return;
    }
    logBuffer.clear();
    setDrawerScript(script);
    setStream(true); // 订阅实时输出（脚本已由上述 REST 启动）
    setDrawerVisible(true);
    // 乐观标记运行中：无论新启动还是复用已运行实例，执行后脚本都在跑。
    setRunningIds((ids) => (ids.includes(script.id) ? ids : [...ids, script.id]));
  };

  // ── 查看日志（打开同一 Drawer：运行中连 SSE 实时续看，否则只看历史）──
  const handleViewLogs = (script: Script) => {
    logBuffer.clear();
    setDrawerScript(script);
    const isRunning = runningIds.includes(script.id);
    setStream(isRunning); // 仅运行中才连 SSE
    setDrawerVisible(true);
    fetchHistory(script, historyLimit);
  };

  const handleStop = () => {
    if (!drawerScript) return;
    cronApi.stopScript(drawerScript.id).catch((error: unknown) => {
      message.error(error instanceof Error ? error.message : '停止脚本失败');
    });
  };

  const handleDrawerClose = () => {
    setDrawerVisible(false);
    setDrawerScript(null);
    logBuffer.clear();
    setStream(false);
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 220,
      render: (name: string, record: Script) => (
        <Space>
          <CodeOutlined />
          <span>{name}</span>
          {runningIds.includes(record.id) && (
            <Tag color="green">运行中</Tag>
          )}
        </Space>
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
      width: 280,
      render: (_: unknown, record: Script) => (
        <Space>
          {runningIds.includes(record.id) ? (
            <Tooltip title="停止">
              <Button
                type="link"
                danger
                icon={<StopOutlined />}
                onClick={async () => {
                  await cronApi.stopScript(record.id);
                  fetchRunning();
                }}
              />
            </Tooltip>
          ) : (
            <Tooltip title="执行">
              <Button
                type="link"
                icon={<PlayCircleOutlined />}
                onClick={() => handleRun(record)}
              />
            </Tooltip>
          )}
          <Tooltip title="日志">
            <Button
              type="link"
              icon={<HistoryOutlined />}
              onClick={() => handleViewLogs(record)}
            />
          </Tooltip>
          <Tooltip title="下载">
            <Button
              type="link"
              icon={<DownloadOutlined />}
              onClick={() => handleDownload(record)}
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
            <Button icon={<ReloadOutlined />} onClick={() => { fetchScripts(); fetchRunning(); }} loading={loading}>刷新</Button>
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
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 个脚本`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
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
        {SCRIPT_TEMPLATES.length === 0 ? (
          <Empty description="暂无模板" />
        ) : (
          <Collapse
            defaultActiveKey={SCRIPT_TEMPLATES.map((_, i) => String(i))}
            items={SCRIPT_TEMPLATES.map((category, index) => ({
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
                      <Space orientation="vertical" style={{ width: '100%' }}>
                        <span style={{ fontWeight: 500 }}>{template.name}</span>
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

      {/* 日志 Drawer：实时 + 历史 合一（按运行状态决定是否连 SSE） */}
      <Drawer
        open={drawerVisible}
        title={
          <Space>
            <HistoryOutlined />
            <span>日志：{drawerScript?.name}</span>
          </Space>
        }
        onClose={handleDrawerClose}
        width={800}
        destroyOnHidden
        styles={{ body: { padding: 0, display: 'flex', flexDirection: 'column' } }}
      >
        <LogViewer
          buffer={logBuffer}
          streamUrl={
            drawerVisible && drawerScript && stream
              ? cronApi.scriptLogsStreamPath(drawerScript.id)
              : undefined
          }
          streamEnabled={drawerVisible && !!drawerScript && stream}
          downloadFileName={drawerScript ? `script_${drawerScript.name}` : 'script_log'}
          onDone={() => {
            setRunningIds((ids) => ids.filter((i) => i !== drawerScript?.id));
          }}
          headerExtra={
            <Space style={{ marginLeft: 8 }}>
              <Select
                value={historyLimit}
                onChange={(v) => {
                  setHistoryLimit(v);
                  if (drawerScript) fetchHistory(drawerScript, v);
                }}
                options={HISTORY_LIMITS.map((n) => ({ value: n, label: `最近 ${n} 条` }))}
                style={{ width: 120 }}
              />
              {drawerScript && runningIds.includes(drawerScript.id) && (
                <Button danger icon={<StopOutlined />} onClick={handleStop}>
                  停止
                </Button>
              )}
            </Space>
          }
          style={{ flex: 1, border: 'none', borderRadius: 0, height: '100%' }}
        />
      </Drawer>
    </div>
  );
}