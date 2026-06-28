import { useState, useEffect, useCallback, useRef } from 'react';
import {
  Modal, Form, Input, Select, message, Tag, Row, Col, Table,
} from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import type { Key } from 'antd/es/table/interface';
import type { FirewallRule, FirewallStatus, FirewallRuleTemplate, FirewallLogEntry } from '../../types';
import { firewallApi } from '../../services/api';
import { CHAIN_OPTIONS, PROTOCOL_OPTIONS, ACTION_OPTIONS, IP_VERSION_OPTIONS, actionColor, disabledRowStyle } from './types';
import FirewallStatusCard from './FirewallStatus';
import FirewallRules from './FirewallRules';
import FirewallTemplates from './FirewallTemplates';
import FirewallLogs from './FirewallLogs';

export default function FirewallPage() {
  const [status, setStatus] = useState<FirewallStatus | null>(null);
  const [rules, setRules] = useState<FirewallRule[]>([]);
  const [systemRules, setSystemRules] = useState<FirewallRule[]>([]);
  const [loading, setLoading] = useState(false);
  const [statusLoading, setStatusLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editingRule, setEditingRule] = useState<FirewallRule | null>(null);
  const [form] = Form.useForm();
  const [operating, setOperating] = useState('');
  const [policyChanging, setPolicyChanging] = useState('');
  const [templateModalVisible, setTemplateModalVisible] = useState(false);
  const [templates, setTemplates] = useState<FirewallRuleTemplate[]>([]);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [selectedRowKeys, setSelectedRowKeys] = useState<Key[]>([]);
  const [bulkOperating, setBulkOperating] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [importModalVisible, setImportModalVisible] = useState(false);
  const [importing, setImporting] = useState(false);
  const [importData, setImportData] = useState<{ version: number; exported_at: string; rules: any[] } | null>(null);
  const [importFileName, setImportFileName] = useState('');
  const [logs, setLogs] = useState<FirewallLogEntry[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logLines, setLogLines] = useState(100);
  const [autoRefreshLogs, setAutoRefreshLogs] = useState(false);
  const autoRefreshTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchStatus = useCallback(async () => {
    setStatusLoading(true);
    try {
      const res = await firewallApi.getStatus();
      setStatus(res.data?.data || null);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取状态失败'));
    } finally {
      setStatusLoading(false);
    }
  }, []);

  const fetchRules = useCallback(async () => {
    setLoading(true);
    try {
      const [dbRes, sysRes] = await Promise.all([
        firewallApi.listRules(),
        firewallApi.getSystemRules(),
      ]);
      setRules(dbRes.data?.data || []);
      setSystemRules(sysRes.data?.data || []);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取规则失败'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchStatus();
    fetchRules();

    // Inject disabled row style
    const styleElement = document.createElement('style');
    styleElement.textContent = disabledRowStyle;
    document.head.appendChild(styleElement);
    return () => {
      document.head.removeChild(styleElement);
    };
  }, [fetchStatus, fetchRules]);

  const fetchLogs = useCallback(async () => {
    setLogsLoading(true);
    try {
      const res = await firewallApi.getLogs(logLines);
      setLogs(res.data?.data || []);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取日志失败'));
    } finally {
      setLogsLoading(false);
    }
  }, [logLines]);

  // Auto-refresh effect for logs
  useEffect(() => {
    if (autoRefreshLogs) {
      fetchLogs();
      autoRefreshTimerRef.current = setInterval(fetchLogs, 5000);
    } else {
      if (autoRefreshTimerRef.current) {
        clearInterval(autoRefreshTimerRef.current);
        autoRefreshTimerRef.current = null;
      }
    }
    return () => {
      if (autoRefreshTimerRef.current) {
        clearInterval(autoRefreshTimerRef.current);
      }
    };
  }, [autoRefreshLogs, fetchLogs]);

  const handleToggleFirewall = async () => {
    if (status?.enabled) {
      // Confirm before disabling
      Modal.confirm({
        title: '危险操作确认',
        icon: <ExclamationCircleOutlined />,
        content: (
          <div>
            <p>您即将 <b>禁用防火墙</b>。</p>
            <p style={{ color: '#ff4d4f' }}>
              警告：禁用防火墙将清除所有规则。如果默认策略为 DROP，您可能会失去对服务器的访问权限！
            </p>
            <p>请确认您已了解风险。</p>
          </div>
        ),
        okText: '确认禁用',
        cancelText: '取消',
        okButtonProps: { danger: true },
        onOk: async () => {
          setOperating('firewall');
          try {
            await firewallApi.disable();
            message.success('防火墙已禁用');
            fetchStatus();
          } catch (error: unknown) {
            message.error((error instanceof Error ? error.message : '操作失败'));
          } finally {
            setOperating('');
          }
        },
      });
    } else {
      setOperating('firewall');
      try {
        await firewallApi.enable();
        message.success('防火墙已启用');
        fetchStatus();
      } catch (error: unknown) {
        message.error((error instanceof Error ? error.message : '操作失败'));
      } finally {
        setOperating('');
      }
    }
  };

  const handleCreate = () => {
    setEditingRule(null);
    form.resetFields();
    form.setFieldsValue({ chain: 'INPUT', protocol: 'tcp', action: 'ACCEPT', ip_version: 'ipv4' });
    setModalVisible(true);
  };

  const handleEdit = (rule: FirewallRule) => {
    setEditingRule(rule);
    form.setFieldsValue({
      chain: rule.chain,
      protocol: rule.protocol,
      port: rule.port,
      action: rule.action,
      source: rule.source,
      ip_version: rule.ip_version || 'ipv4',
      remark: rule.remark,
    });
    setModalVisible(true);
  };

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (editingRule) {
        await firewallApi.updateRule(editingRule.id, values);
        message.success('规则已更新');
      } else {
        await firewallApi.createRule(values);
        message.success('规则已创建');
      }
      setModalVisible(false);
      fetchRules();
    } catch (error: unknown) {
      if ((error instanceof Error ? error.message : String(error))) {
        message.error((error instanceof Error ? error.message : String(error)));
      }
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await firewallApi.deleteRule(id);
      message.success('规则已删除');
      fetchRules();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '删除失败'));
    }
  };

  const handleToggleRule = async (rule: FirewallRule) => {
    setOperating(`rule-${rule.id}`);
    try {
      if (rule.enabled) {
        await firewallApi.disableRule(rule.id);
        message.success('规则已禁用');
      } else {
        await firewallApi.enableRule(rule.id);
        message.success('规则已启用');
      }
      fetchRules();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '操作失败'));
    } finally {
      setOperating('');
    }
  };

  const handleMoveUp = async (id: number) => {
    setOperating(`move-${id}`);
    try {
      await firewallApi.moveRuleUp(id);
      fetchRules();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '上移失败'));
    } finally {
      setOperating('');
    }
  };

  const handleMoveDown = async (id: number) => {
    setOperating(`move-${id}`);
    try {
      await firewallApi.moveRuleDown(id);
      fetchRules();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '下移失败'));
    } finally {
      setOperating('');
    }
  };

  const fetchTemplates = async () => {
    setTemplatesLoading(true);
    try {
      const res = await firewallApi.getTemplates();
      setTemplates(res.data?.data || []);
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '获取模板失败'));
    } finally {
      setTemplatesLoading(false);
    }
  };

  const handleOpenTemplates = () => {
    setTemplateModalVisible(true);
    fetchTemplates();
  };

  const handleApplyTemplate = (template: FirewallRuleTemplate) => {
    Modal.confirm({
      title: '应用模板规则',
      content: (
        <div>
          <p>确定要应用以下模板吗？</p>
          <p><b>{template.name}</b></p>
          <p>协议: {template.protocol.toUpperCase()} | 端口: {template.port} | 动作: {template.action}</p>
          <p style={{ color: '#8c8c8c' }}>{template.remark}</p>
        </div>
      ),
      okText: '应用',
      cancelText: '取消',
      onOk: async () => {
        try {
          await firewallApi.applyTemplate(template.name);
          message.success(`模板 "${template.name}" 已应用`);
          setTemplateModalVisible(false);
          fetchRules();
          fetchStatus();
        } catch (error: unknown) {
          message.error((error instanceof Error ? error.message : '应用模板失败'));
        }
      },
    });
  };

  const handleBulkEnable = async () => {
    if (selectedRowKeys.length === 0) return;
    setBulkOperating(true);
    try {
      const res = await firewallApi.bulkEnableRules(selectedRowKeys as number[]);
      const data = res.data?.data;
      if (data) {
        if (data.failed > 0) {
          message.warning(`批量启用完成: 成功 ${data.succeeded} 个, 失败 ${data.failed} 个`);
        } else {
          message.success(`批量启用成功: ${data.succeeded} 个规则`);
        }
      }
      setSelectedRowKeys([]);
      fetchRules();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '批量启用失败'));
    } finally {
      setBulkOperating(false);
    }
  };

  const handleBulkDisable = async () => {
    if (selectedRowKeys.length === 0) return;
    setBulkOperating(true);
    try {
      const res = await firewallApi.bulkDisableRules(selectedRowKeys as number[]);
      const data = res.data?.data;
      if (data) {
        if (data.failed > 0) {
          message.warning(`批量禁用完成: 成功 ${data.succeeded} 个, 失败 ${data.failed} 个`);
        } else {
          message.success(`批量禁用成功: ${data.succeeded} 个规则`);
        }
      }
      setSelectedRowKeys([]);
      fetchRules();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '批量禁用失败'));
    } finally {
      setBulkOperating(false);
    }
  };

  const handleBulkDelete = async () => {
    if (selectedRowKeys.length === 0) return;
    setBulkOperating(true);
    try {
      const res = await firewallApi.bulkDeleteRules(selectedRowKeys as number[]);
      const data = res.data?.data;
      if (data) {
        if (data.failed > 0) {
          message.warning(`批量删除完成: 成功 ${data.succeeded} 个, 失败 ${data.failed} 个`);
        } else {
          message.success(`批量删除成功: ${data.succeeded} 个规则`);
        }
      }
      setSelectedRowKeys([]);
      fetchRules();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '批量删除失败'));
    } finally {
      setBulkOperating(false);
    }
  };

  const handleExport = async () => {
    setExporting(true);
    try {
      const res = await firewallApi.exportRules();
      const blob = new Blob([res.data], { type: 'application/json' });
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      const date = new Date().toISOString().slice(0, 10);
      a.href = url;
      a.download = `firewall-rules-${date}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      window.URL.revokeObjectURL(url);
      message.success('规则已导出');
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '导出失败'));
    } finally {
      setExporting(false);
    }
  };

  const handleImportFileChange = (info: any) => {
    const file = info.file.originFileObj || info.file;
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (e) => {
      try {
        const text = e.target?.result as string;
        const data = JSON.parse(text);

        if (!data.version || !Array.isArray(data.rules)) {
          message.error('无效的导入文件格式');
          return;
        }

        setImportData(data);
        setImportFileName(file.name);
        setImportModalVisible(true);
      } catch {
        message.error('无法解析文件，请确认是有效的 JSON 文件');
      }
    };
    reader.readAsText(file);
  };

  const handleConfirmImport = async () => {
    if (!importData) return;
    setImporting(true);
    try {
      const res = await firewallApi.importRules(importData);
      const result = res.data?.data;
      if (result) {
        if (result.failed > 0) {
          const errorDetails = result.errors?.length > 0 ? `: ${result.errors.join('; ')}` : '';
          message.warning(`导入完成: 成功 ${result.succeeded} 个, 失败 ${result.failed} 个${errorDetails}`);
        } else {
          message.success(`导入成功: ${result.succeeded} 个规则`);
        }
      }
      setImportModalVisible(false);
      setImportData(null);
      setImportFileName('');
      fetchRules();
      fetchStatus();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '导入失败'));
    } finally {
      setImporting(false);
    }
  };

  const handleChangePolicy = (chain: 'INPUT' | 'OUTPUT', policy: string) => {
    const chainLabel = chain === 'INPUT' ? '入站(INPUT)' : '出站(OUTPUT)';
    const policyLabel = policy === 'DROP' ? '拒绝(DROP)' : '允许(ACCEPT)';

    if (policy === 'DROP') {
      Modal.confirm({
        title: '危险操作确认',
        icon: <ExclamationCircleOutlined />,
        content: (
          <div>
            <p>您即将将 <b>{chainLabel}</b> 默认策略设置为 <b>{policyLabel}</b>。</p>
            <p style={{ color: '#ff4d4f' }}>
              警告：此操作将拒绝所有未明确放行的流量。如果 SSH 端口(22)和面板端口没有对应的 ACCEPT 规则，您可能会失去对服务器的访问权限！
            </p>
            <p>请确认您已了解风险。</p>
          </div>
        ),
        okText: '确认修改',
        cancelText: '取消',
        okButtonProps: { danger: true },
        onOk: () => doChangePolicy(chain, policy),
      });
    } else {
      doChangePolicy(chain, policy);
    }
  };

  const doChangePolicy = async (chain: string, policy: string) => {
    setPolicyChanging(chain);
    try {
      await firewallApi.setDefaultPolicy({ chain, policy });
      message.success(`默认策略已修改: ${chain} -> ${policy}`);
      fetchStatus();
    } catch (error: unknown) {
      message.error((error instanceof Error ? error.message : '修改默认策略失败'));
    } finally {
      setPolicyChanging('');
    }
  };

  return (
    <div>
      <FirewallStatusCard
        status={status}
        statusLoading={statusLoading}
        operating={operating}
        policyChanging={policyChanging}
        onToggleFirewall={handleToggleFirewall}
        onChangePolicy={handleChangePolicy}
      />

      <FirewallRules
        rules={rules}
        systemRules={systemRules}
        loading={loading}
        operating={operating}
        selectedRowKeys={selectedRowKeys}
        bulkOperating={bulkOperating}
        exporting={exporting}
        importing={importing}
        onCreate={handleCreate}
        onEdit={handleEdit}
        onDelete={handleDelete}
        onToggleRule={handleToggleRule}
        onMoveUp={handleMoveUp}
        onMoveDown={handleMoveDown}
        onBulkEnable={handleBulkEnable}
        onBulkDisable={handleBulkDisable}
        onBulkDelete={handleBulkDelete}
        onExport={handleExport}
        onImportFileChange={handleImportFileChange}
        onOpenTemplates={handleOpenTemplates}
        onRefresh={fetchRules}
        onSelectedRowKeysChange={(keys) => setSelectedRowKeys(keys)}
      />

      <FirewallLogs
        logs={logs}
        loading={logsLoading}
        logLines={logLines}
        autoRefresh={autoRefreshLogs}
        onLogLinesChange={setLogLines}
        onAutoRefreshChange={setAutoRefreshLogs}
        onRefresh={fetchLogs}
      />

      {/* Create/Edit Modal */}
      <Modal
        title={editingRule ? '编辑规则' : '添加规则'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleSubmit}
        okText={editingRule ? '保存' : '添加'}
        cancelText="取消"
        style={{ top: 20 }}
      >
        <Form form={form} layout="vertical">
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="chain" label="链" rules={[{ required: true, message: '请选择链' }]}>
                <Select options={CHAIN_OPTIONS.map(c => ({ label: c, value: c }))} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="action" label="动作" rules={[{ required: true, message: '请选择动作' }]}>
                <Select options={ACTION_OPTIONS.map(a => ({ label: a, value: a }))} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="protocol" label="协议">
                <Select options={PROTOCOL_OPTIONS.map(p => ({ label: p.toUpperCase(), value: p }))} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="port" label="端口">
                <Input placeholder="80 或 8000-9000" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col span={12}>
              <Form.Item name="ip_version" label="IP 版本">
                <Select options={IP_VERSION_OPTIONS} />
              </Form.Item>
            </Col>
            <Col span={12}>
              <Form.Item name="source" label="来源 IP">
                <Input placeholder="192.168.1.0/24 或 2001:db8::/32" />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="remark" label="备注">
            <Input placeholder="规则描述（可选）" />
          </Form.Item>
        </Form>
      </Modal>

      {/* Import Confirm Modal */}
      <Modal
        title="导入防火墙规则"
        open={importModalVisible}
        onCancel={() => { setImportModalVisible(false); setImportData(null); setImportFileName(''); }}
        onOk={handleConfirmImport}
        okText="确认导入"
        cancelText="取消"
        confirmLoading={importing}
        style={{ top: 20 }}
      >
        {importData && (
          <div>
            <p><b>文件:</b> {importFileName}</p>
            <p><b>规则数量:</b> {importData.rules.length} 条</p>
            <p><b>导出时间:</b> {importData.exported_at}</p>
            <div style={{ maxHeight: 300, overflow: 'auto', marginTop: 12 }}>
              <Table
                dataSource={importData.rules.map((r, i) => ({ ...r, _key: i }))}
                rowKey="_key"
                size="small"
                pagination={false}
                columns={[
                  { title: '链', dataIndex: 'chain', key: 'chain', width: 80, render: (chain: string) => <Tag>{chain}</Tag> },
                  { title: '协议', dataIndex: 'protocol', key: 'protocol', width: 70 },
                  { title: '端口', dataIndex: 'port', key: 'port', width: 80, render: (port: string) => port || '-' },
                  { title: '动作', dataIndex: 'action', key: 'action', width: 80, render: (action: string) => <Tag color={actionColor(action)}>{action}</Tag> },
                  { title: '来源', dataIndex: 'source', key: 'source', render: (source: string) => source || '-' },
                  { title: '备注', dataIndex: 'remark', key: 'remark' },
                ]}
              />
            </div>
          </div>
        )}
      </Modal>

      <FirewallTemplates
        visible={templateModalVisible}
        templates={templates}
        loading={templatesLoading}
        onClose={() => setTemplateModalVisible(false)}
        onApply={handleApplyTemplate}
      />
    </div>
  );
}
