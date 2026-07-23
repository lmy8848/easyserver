import { useState, useEffect, useMemo, useCallback } from 'react';
import {
  Card, Table, Input, Select, Button, Space, DatePicker, message,
  Tag, Tooltip, Modal, Descriptions, Typography, Row, Col, Segmented,
  Badge,
} from 'antd';
import {
  SearchOutlined, DeleteOutlined, ReloadOutlined,
  DownloadOutlined, EyeOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import { auditApi, systemApi } from '../services/api';
import dayjs from 'dayjs';

const { RangePicker } = DatePicker;
const { Text } = Typography;

// 样式常量
const RAW_DATA_STYLE: React.CSSProperties = {
  background: '#f5f5f5',
  padding: 12,
  borderRadius: 4,
  fontSize: 12,
  maxHeight: 300,
  overflow: 'auto',
  margin: 0,
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-all',
  maxWidth: '100%',
};

interface AuditLogItem {
  id: number;
  user_id: number;
  username: string;
  action: string;
  resource: string;
  detail: string;
  ip: string;
  user_agent: string;
  type: string;
  created_at: string;
}

interface ParsedDetail {
  method: string;
  path: string;
  status: string;
  duration: string;
  summary: string;
  success?: boolean;
  body?: any;
  params?: Record<string, string>;
  query?: Record<string, string>;
}

interface AuditStats {
  user_stats: { username: string; count: number }[];
  action_stats: { action: string; count: number }[];
  day_stats: { day: string; count: number }[];
  status_stats: { status: string; count: number }[];
  alerts: { id: number; username: string; action: string; resource: string; status: number; ip: string; created_at: string }[];
}

export default function AuditLog() {
  const [logs, setLogs] = useState<AuditLogItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [stats, setStats] = useState<AuditStats | null>(null);
  const [statsDays, setStatsDays] = useState(7);
  const [activeTab, setActiveTab] = useState<string>('operation');

  // SSH 登录日志
  const [sshLogins, setSSHLogins] = useState<any[]>([]);
  const [sshLoading, setSSHLoading] = useState(false);

  // 筛选条件
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [username, setUsername] = useState('');
  const [actionFilter, setActionFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [resource, setResource] = useState('');
  const [ipFilter, setIpFilter] = useState('');
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null);

  // 详情弹窗
  const [detailVisible, setDetailVisible] = useState(false);
  const [detailItem, setDetailItem] = useState<AuditLogItem | null>(null);

  const parseDetail = (detail: string): ParsedDetail => {
    try {
      const obj = JSON.parse(detail);
      let inner = obj;
      if (obj.detail && typeof obj.detail === 'string') {
        try { inner = JSON.parse(obj.detail); } catch { inner = obj; }
      }
      return {
        method: inner.method || obj.method || '-',
        path: inner.path || obj.path || '-',
        status: String(inner.status || obj.status || '-'),
        duration: inner.duration_ms ? `${inner.duration_ms}ms` : obj.duration_ms ? `${obj.duration_ms}ms` : '-',
        summary: inner.summary || inner.command || inner.file_path || obj.summary || obj.command || obj.file_path || '-',
        success: inner.success ?? obj.success,
        body: inner.body || obj.body,
        params: inner.params || obj.params,
        query: inner.query || obj.query,
      };
    } catch {
      return { method: '-', path: detail, status: '-', duration: '-', summary: detail };
    }
  };

  const fetchLogs = useCallback(async () => {
    if (activeTab !== 'operation' && activeTab !== 'request') return;
    setLoading(true);
    try {
      const params: { page: number; page_size: number; username?: string; action?: string; resource?: string; ip?: string; status?: string; start_date?: string; end_date?: string; type?: string } = { page, page_size: pageSize };
      if (username) params['username'] = username;
      if (actionFilter) params['action'] = actionFilter;
      if (resource) params['resource'] = resource;
      if (ipFilter) params['ip'] = ipFilter;
      if (statusFilter) params['status'] = statusFilter;
      if (dateRange?.[0]) params['start_date'] = dateRange[0].format('YYYY-MM-DD');
      if (dateRange?.[1]) params['end_date'] = dateRange[1].format('YYYY-MM-DD');
      params['type'] = activeTab;

      const res = await auditApi.list(params);
      setLogs(res.data.data?.items || []);
      setTotal(res.data.data?.total || 0);
    } catch (error) {
      console.error('Failed to fetch audit logs:', error);
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, username, actionFilter, statusFilter, resource, ipFilter, dateRange, activeTab]);

  const fetchStats = useCallback(async () => {
    try {
      const res = await auditApi.getStats(statsDays);
      setStats(res.data.data || null);
    } catch (error) {
      console.error('Failed to fetch stats:', error);
    }
  }, [statsDays]);

  const fetchSSHLogins = async () => {
    setSSHLoading(true);
    try {
      const res = await systemApi.getSSHLogins(200);
      setSSHLogins(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch SSH logins:', error);
    } finally {
      setSSHLoading(false);
    }
  };

  useEffect(() => {
    fetchStats(); // 初始加载统计数据，显示异常告警角标
  }, [fetchStats]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  useEffect(() => {
    if (activeTab === 'stats') {
      fetchStats();
    }
    if (activeTab === 'ssh') {
      fetchSSHLogins();
    }
  }, [activeTab, statsDays, fetchStats]);

  const handleSearch = () => {
    setPage(1);
    fetchLogs();
  };

  const handleExport = async () => {
    try {
      const params: any = {};
      if (username) params['username'] = username;
      if (actionFilter) params['action'] = actionFilter;
      if (resource) params['resource'] = resource;
      if (ipFilter) params['ip'] = ipFilter;
      if (dateRange?.[0]) params['start_date'] = dateRange[0].format('YYYY-MM-DD');
      if (dateRange?.[1]) params['end_date'] = dateRange[1].format('YYYY-MM-DD');
      if (activeTab === 'operation' || activeTab === 'request') params['type'] = activeTab;

      const res = await auditApi.export(params);
      const blob = new Blob([res.data as BlobPart], { type: 'text/csv' });
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `audit_logs_${dayjs().format('YYYYMMDD_HHmmss')}.csv`;
      a.click();
      window.URL.revokeObjectURL(url);
      message.success('导出成功');
    } catch (error) {
      message.error('导出失败');
    }
  };

  const handleClean = async () => {
    try {
      const res = await auditApi.clean(90);
      message.success(`已清理 ${res.data.data?.deleted || 0} 条记录`);
      fetchLogs();
    } catch (error) {
      message.error('清理失败');
    }
  };

  const showDetail = (item: AuditLogItem) => {
    setDetailItem(item);
    setDetailVisible(true);
  };

  // 6 类动词颜色（中间件自动推导的 operation 日志 action）
  const verbColors: Record<string, string> = {
    '创建': 'blue', '删除': 'red', '修改': 'orange',
    '执行': 'purple', '认证': 'cyan', '其他': 'default',
  };

  // HTTP method 颜色（request 日志 action 列存 method）
  const methodColors: Record<string, string> = {
    POST: 'blue', PUT: 'orange', DELETE: 'red', GET: 'green', PATCH: 'orange',
  };

  const getActionColor = (action: string) => verbColors[action] || methodColors[action] || 'default';

  const getActionLabel = (action: string) => action;

  const getMethodColor = (method: string) => methodColors[method] || 'default';

  const getResourceText = (record: AuditLogItem) => {
    // operation 日志的 resource 列存类别（中间件按路由推导）；request 日志存 path。
    // 具体 resource 名在摘要列展示。
    return record.resource;
  };

  const getStatusColor = (status: string) => {
    const code = parseInt(status);
    if (code >= 200 && code < 300) return 'success';
    if (code >= 400 && code < 500) return 'warning';
    if (code >= 500) return 'error';
    return 'default';
  };

  // 统计图表配置 (memoized to prevent unnecessary re-renders)
  const userChartOption = useMemo(() => ({
    title: { text: '用户操作统计', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: { trigger: 'axis' as const },
    xAxis: {
      type: 'category' as const,
      data: stats?.user_stats?.map(s => s.username) || [],
      axisLabel: { rotate: 30 },
    },
    yAxis: { type: 'value' as const },
    series: [{
      type: 'bar',
      data: stats?.user_stats?.map(s => s.count) || [],
      itemStyle: { color: '#1890ff' },
    }],
  }), [stats?.user_stats]);

  const actionChartOption = useMemo(() => ({
    title: { text: '操作类型分布', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: { trigger: 'item' as const },
    series: [{
      type: 'pie',
      radius: '60%',
      data: stats?.action_stats?.map(s => ({ name: s.action, value: s.count })) || [],
    }],
  }), [stats?.action_stats]);

  const dayChartOption = useMemo(() => ({
    title: { text: '每日操作趋势', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: { trigger: 'axis' as const },
    xAxis: {
      type: 'category' as const,
      data: stats?.day_stats?.map(s => s.day) || [],
    },
    yAxis: { type: 'value' as const },
    series: [{
      type: 'line',
      data: stats?.day_stats?.map(s => s.count) || [],
      smooth: true,
      areaStyle: { opacity: 0.3 },
    }],
  }), [stats?.day_stats]);

  const statusChartOption = useMemo(() => ({
    title: { text: '响应状态分布', left: 'center', textStyle: { fontSize: 14 } },
    tooltip: { trigger: 'item' as const },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      data: stats?.status_stats?.map(s => ({
        name: s.status,
        value: s.count,
        itemStyle: {
          color: s.status === '2xx' ? '#52c41a' : s.status === '4xx' ? '#faad14' : s.status === '5xx' ? '#ff4d4f' : '#d9d9d9',
        },
      })) || [],
    }],
  }), [stats?.status_stats]);

  const columns = useMemo(() => {
    const timeCol = {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (text: string) => <span style={{ fontSize: 12 }}>{text}</span>,
    };
    const userCol = {
      title: '用户',
      dataIndex: 'username',
      key: 'username',
      width: 100,
      render: (text: string) => <strong>{text || '-'}</strong>,
    };
    const actionCol = {
      title: '操作类型',
      dataIndex: 'action',
      key: 'action',
      width: 120,
      render: (action: string) => (
        <Tag color={getActionColor(action)}>
          {getActionLabel(action)}
        </Tag>
      ),
    };
    const resourceCol = {
      title: '资源',
      dataIndex: 'resource',
      key: 'resource',
      width: 150,
      ellipsis: true,
      render: (_: string, record: AuditLogItem) => {
        const text = getResourceText(record);
        return <Tooltip title={text}><span style={{ fontSize: 12 }}>{text || '-'}</span></Tooltip>;
      },
    };
    const resultCol = {
      title: '结果',
      key: 'result',
      width: 80,
      render: (_: unknown, record: AuditLogItem) => {
        const detail = parseDetail(record.detail);
        let isSuccess = true;
        if (detail.status && detail.status !== '-') {
          isSuccess = parseInt(detail.status) < 400;
        } else if (detail.success !== undefined) {
          isSuccess = detail.success;
        }
        return <Tag color={isSuccess ? 'success' : 'error'}>{isSuccess ? '成功' : '失败'}</Tag>;
      },
    };
    const methodCol = {
      title: '方法',
      key: 'method',
      width: 80,
      render: (_: unknown, record: AuditLogItem) => {
        const detail = parseDetail(record.detail);
        if (detail.method === '-') return '-';
        return <Tag color={getMethodColor(detail.method)}>{detail.method}</Tag>;
      },
    };
    const pathCol = {
      title: '路径',
      key: 'path',
      ellipsis: true,
      render: (_: unknown, record: AuditLogItem) => {
        const detail = parseDetail(record.detail);
        return <Tooltip title={detail.path}><span style={{ fontSize: 12 }}>{detail.path}</span></Tooltip>;
      },
    };
    const statusCol = {
      title: '状态',
      key: 'status',
      width: 80,
      render: (_: unknown, record: AuditLogItem) => {
        const status = parseDetail(record.detail).status;
        const code = parseInt(status);
        const isAlert = code >= 400;
        return (
          <Badge dot={isAlert} offset={[-2, 2]}>
            <Tag color={getStatusColor(status)}>{status}</Tag>
          </Badge>
        );
      },
    };
    const durationCol = {
      title: '耗时',
      key: 'duration',
      width: 80,
      render: (_: unknown, record: AuditLogItem) => <span style={{ fontSize: 12 }}>{parseDetail(record.detail).duration}</span>,
    };
    const ipCol = {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
      width: 130,
    };
    const detailCol = {
      title: '详情',
      key: 'detail',
      width: 60,
      render: (_: unknown, record: AuditLogItem) => (
        <Tooltip title="查看详情">
          <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => showDetail(record)} />
        </Tooltip>
      ),
    };
    const summaryCol = {
      title: '摘要',
      key: 'summary',
      ellipsis: true,
      render: (_: unknown, record: AuditLogItem) => {
        const d = parseDetail(record.detail);
        return d.summary && d.summary !== '-' ? d.summary : '-';
      },
    };
    // operation 日志只展示资源和摘要列；request 日志展示方法/路径/状态/耗时/IP
    const showRequestCols = activeTab === 'request';
    const showResourceCol = activeTab === 'operation';
    return [
      timeCol, userCol,
      ...(showResourceCol ? [actionCol, resourceCol, resultCol, summaryCol] : []),
      ...(showRequestCols ? [methodCol, pathCol, statusCol, durationCol, ipCol, detailCol] : []),
    ];
  }, [activeTab]);

  return (
    <div>
      <Card
        title="审计日志"
        extra={
          <Space>
            <Segmented
              options={[
                { label: '操作日志', value: 'operation' },
                { label: '请求日志', value: 'request' },
                { label: '统计分析', value: 'stats' },
                { label: 'SSH 登录', value: 'ssh' },
              ]}
              value={activeTab}
              onChange={(v) => {
                setActiveTab(v as string);
                setUsername('');
                setActionFilter('');
                setStatusFilter('');
                setResource('');
                setIpFilter('');
                setDateRange(null);
                setPage(1);
              }}
            />
            <Button icon={<DownloadOutlined />} onClick={handleExport}>导出</Button>
            <Button icon={<ReloadOutlined />} onClick={() => { fetchLogs(); fetchStats(); }}>刷新</Button>
            <Button danger icon={<DeleteOutlined />} onClick={handleClean}>清理90天前</Button>
          </Space>
        }
      >
        {(activeTab === 'operation' || activeTab === 'request') && (
          <>
            <Space wrap style={{ marginBottom: 16 }}>
              <Input placeholder="用户名" value={username} onChange={e => setUsername(e.target.value)} style={{ width: 120 }} allowClear />
              <Select
                placeholder={activeTab === 'request' ? '请求方法' : '操作类型'}
                value={actionFilter || undefined}
                onChange={v => setActionFilter(v || '')}
                style={{ width: 120 }}
                allowClear
                options={(activeTab === 'request' ? ['GET', 'POST', 'PUT', 'DELETE', 'PATCH'] : ['创建', '删除', '修改', '执行', '认证', '其他']).map(a => ({ label: a, value: a }))}
              />
              <Select
                placeholder={activeTab === 'request' ? '状态' : '结果'}
                value={statusFilter || undefined}
                onChange={v => setStatusFilter(v || '')}
                style={{ width: 100 }}
                allowClear
                options={activeTab === 'request' 
                  ? [{ label: '2xx', value: '2xx' }, { label: '4xx', value: '4xx' }, { label: '5xx', value: '5xx' }]
                  : [{ label: '成功', value: 'success' }, { label: '失败', value: 'failed' }]}
              />
              <Input placeholder={activeTab === 'request' ? '请求路径' : '操作资源'} value={resource} onChange={e => setResource(e.target.value)} style={{ width: 180 }} allowClear />
              {activeTab === 'request' && <Input placeholder="IP 地址" value={ipFilter} onChange={e => setIpFilter(e.target.value)} style={{ width: 140 }} allowClear />}
              <RangePicker value={dateRange as any} onChange={(dates) => setDateRange(dates as any)} placeholder={['开始日期', '结束日期']} />
              <Button type="primary" icon={<SearchOutlined />} onClick={handleSearch}>搜索</Button>
            </Space>
            <Table
              columns={columns}
              dataSource={logs}
              rowKey="id"
              loading={loading}
              pagination={{
                current: page, pageSize, total,
                showSizeChanger: true,
                pageSizeOptions: ['20', '50', '100'],
                showTotal: (t) => `共 ${t} 条记录`,
                onChange: (p, ps) => { setPage(p); setPageSize(ps); },
              }}
              size="small"
              scroll={{ x: 1000 }}
            />
          </>
        )}

        {activeTab === 'stats' && (
          <>
            <Space style={{ marginBottom: 16 }}>
              <span>统计范围：</span>
              <Segmented
                options={[
                  { label: '近7天', value: 7 },
                  { label: '近30天', value: 30 },
                  { label: '近90天', value: 90 },
                ]}
                value={statsDays}
                onChange={(v) => setStatsDays(v as number)}
              />
            </Space>
            <Row gutter={[16, 16]}>
              <Col xs={24} lg={12}>
                <Card><ReactECharts option={userChartOption} style={{ height: 300 }} /></Card>
              </Col>
              <Col xs={24} lg={12}>
                <Card><ReactECharts option={actionChartOption} style={{ height: 300 }} /></Card>
              </Col>
              <Col xs={24} lg={12}>
                <Card><ReactECharts option={dayChartOption} style={{ height: 300 }} /></Card>
              </Col>
              <Col xs={24} lg={12}>
                <Card><ReactECharts option={statusChartOption} style={{ height: 300 }} /></Card>
              </Col>
            </Row>
          </>
        )}



        {activeTab === 'ssh' && (
          <>
            <Space style={{ marginBottom: 16 }}>
              <Text>系统 SSH 登录历史</Text>
              <Button icon={<ReloadOutlined />} onClick={fetchSSHLogins} loading={sshLoading}>刷新</Button>
            </Space>
            <Table
              dataSource={sshLogins}
              rowKey={(r, index) => `${r.username}-${r.ip}-${r.time}-${r.terminal}-${index}`}
              loading={sshLoading}
              size="small"
              pagination={{ pageSize: 50 }}
              columns={[
                { title: '用户名', dataIndex: 'username', width: 100 },
                { title: 'IP 地址', dataIndex: 'ip', width: 150 },
                { title: '登录时间', dataIndex: 'time', width: 200 },
                { title: '终端', dataIndex: 'terminal', width: 100 },
                {
                  title: '类型', dataIndex: 'type', width: 100,
                  render: (v: string) => {
                    const colorMap: Record<string, string> = {
                      active: 'green', login: 'blue', failed: 'red', console: 'orange',
                    };
                    const labelMap: Record<string, string> = {
                      active: '在线', login: '登录', failed: '失败', console: '控制台',
                    };
                    return <Tag color={colorMap[v] || 'default'}>{labelMap[v] || v}</Tag>;
                  },
                },
              ]}
            />
          </>
        )}
      </Card>

      {/* 详情弹窗 */}
      <Modal
        title={`操作详情 #${detailItem?.id}`}
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={700}
      >
        {detailItem && (
          <Descriptions bordered column={1} size="small">
            <Descriptions.Item label="时间">{detailItem.created_at}</Descriptions.Item>
            <Descriptions.Item label="用户">{detailItem.username || '-'}</Descriptions.Item>
            <Descriptions.Item label="IP 地址">{detailItem.ip || '-'}</Descriptions.Item>
            <Descriptions.Item label="操作方法">
              <Tag color={getMethodColor(parseDetail(detailItem.detail).method)}>{parseDetail(detailItem.detail).method}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="请求路径"><Text copyable>{parseDetail(detailItem.detail).path}</Text></Descriptions.Item>
            <Descriptions.Item label="响应状态">
              <Tag color={getStatusColor(parseDetail(detailItem.detail).status)}>{parseDetail(detailItem.detail).status}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="耗时">{parseDetail(detailItem.detail).duration}</Descriptions.Item>
            <Descriptions.Item label="User-Agent">
              <Text ellipsis={{ tooltip: detailItem.user_agent }} style={{ maxWidth: 500 }}>{detailItem.user_agent || '-'}</Text>
            </Descriptions.Item>
            <Descriptions.Item label="原始数据">
              <pre style={RAW_DATA_STYLE}>
                {(() => {
                  try {
                    const obj = JSON.parse(detailItem.detail);
                    // 解析嵌套的 JSON 字符串
                    if (obj.detail && typeof obj.detail === 'string') {
                      try { obj.detail = JSON.parse(obj.detail); } catch { /* detail is not JSON */ }
                    }
                    return JSON.stringify(obj, null, 2);
                  } catch { return detailItem.detail; }
                })()}
              </pre>
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </div>
  );
}
