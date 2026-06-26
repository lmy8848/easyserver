import { useState, useEffect, useMemo } from 'react';
import {
  Card, Table, Input, Select, Button, Space, DatePicker, message,
  Tag, Tooltip, Modal, Descriptions, Typography, Row, Col, Segmented,
  Badge,
} from 'antd';
import {
  SearchOutlined, DeleteOutlined, ReloadOutlined,
  DownloadOutlined, EyeOutlined, WarningOutlined,
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
  created_at: string;
}

interface ParsedDetail {
  method: string;
  path: string;
  status: string;
  duration: string;
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
  const [actions, setActions] = useState<string[]>([]);
  const [stats, setStats] = useState<AuditStats | null>(null);
  const [statsDays, setStatsDays] = useState(7);
  const [activeTab, setActiveTab] = useState<string>('list');

  // SSH 登录日志
  const [sshLogins, setSSHLogins] = useState<any[]>([]);
  const [sshLoading, setSSHLoading] = useState(false);

  // 筛选条件
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [username, setUsername] = useState('');
  const [actionFilter, setActionFilter] = useState('');
  const [resource, setResource] = useState('');
  const [ipFilter, setIpFilter] = useState('');
  const [dateRange, setDateRange] = useState<[dayjs.Dayjs | null, dayjs.Dayjs | null] | null>(null);

  // 详情弹窗
  const [detailVisible, setDetailVisible] = useState(false);
  const [detailItem, setDetailItem] = useState<AuditLogItem | null>(null);

  const parseDetail = (detail: string): ParsedDetail => {
    try {
      const obj = JSON.parse(detail);
      // 处理嵌套 JSON（detail 字段可能包含转义的 JSON 字符串）
      let inner = obj;
      if (obj.detail && typeof obj.detail === 'string') {
        try {
          inner = JSON.parse(obj.detail);
        } catch {
          inner = obj;
        }
      }
      return {
        method: inner.method || obj.method || '-',
        path: inner.path || obj.path || '-',
        status: String(inner.status || obj.status || '-'),
        duration: inner.duration_ms ? `${inner.duration_ms}ms` : obj.duration_ms ? `${obj.duration_ms}ms` : '-',
      };
    } catch {
      return { method: '-', path: detail, status: '-', duration: '-' };
    }
  };

  const fetchActions = async () => {
    try {
      const res = await auditApi.getActions();
      setActions(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch actions:', error);
    }
  };

  const fetchLogs = async () => {
    setLoading(true);
    try {
      const params: any = { page, page_size: pageSize };
      if (username) params.username = username;
      if (actionFilter) params.action = actionFilter;
      if (resource) params.resource = resource;
      if (ipFilter) params.ip = ipFilter;
      if (dateRange?.[0]) params.start_date = dateRange[0].format('YYYY-MM-DD');
      if (dateRange?.[1]) params.end_date = dateRange[1].format('YYYY-MM-DD');

      const res = await auditApi.list(params);
      setLogs(res.data.data?.items || []);
      setTotal(res.data.data?.total || 0);
    } catch (error) {
      console.error('Failed to fetch audit logs:', error);
    } finally {
      setLoading(false);
    }
  };

  const fetchStats = async () => {
    try {
      const res = await auditApi.getStats(statsDays);
      setStats(res.data.data || null);
    } catch (error) {
      console.error('Failed to fetch stats:', error);
    }
  };

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
    fetchActions();
    fetchStats(); // 初始加载统计数据，显示异常告警角标
  }, []);

  useEffect(() => {
    fetchLogs();
  }, [page, pageSize, actionFilter]);

  useEffect(() => {
    if (activeTab === 'stats' || activeTab === 'alerts') {
      fetchStats();
    }
    if (activeTab === 'ssh') {
      fetchSSHLogins();
    }
  }, [activeTab, statsDays]);

  const handleSearch = () => {
    setPage(1);
    fetchLogs();
  };

  const handleExport = async () => {
    try {
      const params: any = {};
      if (username) params.username = username;
      if (actionFilter) params.action = actionFilter;
      if (resource) params.resource = resource;
      if (ipFilter) params.ip = ipFilter;
      if (dateRange?.[0]) params.start_date = dateRange[0].format('YYYY-MM-DD');
      if (dateRange?.[1]) params.end_date = dateRange[1].format('YYYY-MM-DD');

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

  const getMethodColor = (method: string) => {
    const colors: Record<string, string> = {
      POST: 'blue', PUT: 'orange', DELETE: 'red', GET: 'green',
      // 服务器级别操作
      TERMINAL_OPEN: 'purple', TERMINAL_CLOSE: 'purple',
      FILE_MKDIR: 'cyan', FILE_UPLOAD: 'cyan', FILE_DOWNLOAD: 'cyan',
      FILE_RENAME: 'cyan', FILE_DELETE: 'red', FILE_MOVE: 'cyan', FILE_COPY: 'cyan', FILE_EDIT: 'cyan',
      SECURITY_LOGIN_SUCCESS: 'green', SECURITY_LOGIN_FAILED: 'red',
      SECURITY_LOGOUT: 'orange', SECURITY_PASSWORD_CHANGED: 'blue',
      SYSTEM_SERVER_START: 'gold', SYSTEM_SERVER_STOP: 'gold',
      SYSTEM_SERVICE_FAILED: 'red', SYSTEM_DISK_WARNING: 'orange',
      SERVICE_START: 'lime', SERVICE_STOP: 'orange', SERVICE_RESTART: 'blue',
    };
    return colors[method] || 'default';
  };

  const getActionLabel = (action: string) => {
    const labels: Record<string, string> = {
      TERMINAL_OPEN: '终端打开', TERMINAL_CLOSE: '终端关闭',
      FILE_MKDIR: '创建目录', FILE_UPLOAD: '上传文件', FILE_DOWNLOAD: '下载文件',
      FILE_RENAME: '重命名', FILE_DELETE: '删除', FILE_MOVE: '移动', FILE_COPY: '复制', FILE_EDIT: '编辑文件',
      SECURITY_LOGIN_SUCCESS: '登录成功', SECURITY_LOGIN_FAILED: '登录失败',
      SECURITY_LOGOUT: '退出登录', SECURITY_PASSWORD_CHANGED: '修改密码',
      SYSTEM_SERVER_START: '服务启动', SYSTEM_SERVER_STOP: '服务停止',
      SYSTEM_SERVICE_FAILED: '服务异常', SYSTEM_DISK_WARNING: '磁盘警告',
      SERVICE_START: '启动服务', SERVICE_STOP: '停止服务', SERVICE_RESTART: '重启服务',
    };
    return labels[action] || action;
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

  const columns = [
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (text: string) => <span style={{ fontSize: 12 }}>{text}</span>,
    },
    {
      title: '用户',
      dataIndex: 'username',
      key: 'username',
      width: 100,
      render: (text: string) => <strong>{text || '-'}</strong>,
    },
    {
      title: '操作类型',
      dataIndex: 'action',
      key: 'action',
      width: 120,
      render: (action: string) => (
        <Tag color={getMethodColor(action)}>
          {getActionLabel(action)}
        </Tag>
      ),
    },
    {
      title: '方法',
      key: 'method',
      width: 80,
      render: (_: any, record: AuditLogItem) => {
        const detail = parseDetail(record.detail);
        if (detail.method === '-') return '-';
        return <Tag color={getMethodColor(detail.method)}>{detail.method}</Tag>;
      },
    },
    {
      title: '路径',
      key: 'path',
      ellipsis: true,
      render: (_: any, record: AuditLogItem) => {
        const detail = parseDetail(record.detail);
        return <Tooltip title={detail.path}><span style={{ fontSize: 12 }}>{detail.path}</span></Tooltip>;
      },
    },
    {
      title: '状态',
      key: 'status',
      width: 80,
      render: (_: any, record: AuditLogItem) => {
        const status = parseDetail(record.detail).status;
        const code = parseInt(status);
        const isAlert = code >= 400;
        return (
          <Badge dot={isAlert} offset={[-2, 2]}>
            <Tag color={getStatusColor(status)}>{status}</Tag>
          </Badge>
        );
      },
    },
    {
      title: '耗时',
      key: 'duration',
      width: 80,
      render: (_: any, record: AuditLogItem) => <span style={{ fontSize: 12 }}>{parseDetail(record.detail).duration}</span>,
    },
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
      width: 130,
    },
    {
      title: '详情',
      key: 'action',
      width: 60,
      render: (_: any, record: AuditLogItem) => (
        <Tooltip title="查看详情">
          <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => showDetail(record)} />
        </Tooltip>
      ),
    },
  ];

  return (
    <div>
      <Card
        title="操作日志"
        extra={
          <Space>
            <Segmented
              options={[
                { label: '日志列表', value: 'list' },
                { label: '统计分析', value: 'stats' },
                { label: <Badge key="alerts-badge" count={stats?.alerts?.length || 0} size="small"><span style={{ padding: '0 8px' }}>异常告警</span></Badge>, value: 'alerts' },
                { label: 'SSH 登录', value: 'ssh' },
              ]}
              value={activeTab}
              onChange={(v) => setActiveTab(v as string)}
            />
            <Button icon={<DownloadOutlined />} onClick={handleExport}>导出CSV</Button>
            <Button icon={<ReloadOutlined />} onClick={() => { fetchLogs(); fetchStats(); }}>刷新</Button>
            <Button danger icon={<DeleteOutlined />} onClick={handleClean}>清理90天前</Button>
          </Space>
        }
      >
        {activeTab === 'list' && (
          <>
            <Space wrap style={{ marginBottom: 16 }}>
              <Input placeholder="用户名" value={username} onChange={e => setUsername(e.target.value)} style={{ width: 120 }} allowClear />
              <Select placeholder="操作类型" value={actionFilter || undefined} onChange={v => setActionFilter(v || '')} style={{ width: 120 }} allowClear options={actions?.map(a => ({ label: a, value: a })) || []} />
              <Input placeholder="资源路径" value={resource} onChange={e => setResource(e.target.value)} style={{ width: 180 }} allowClear />
              <Input placeholder="IP 地址" value={ipFilter} onChange={e => setIpFilter(e.target.value)} style={{ width: 140 }} allowClear />
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

        {activeTab === 'alerts' && (
          <>
            <Space style={{ marginBottom: 16 }}>
              <WarningOutlined style={{ color: '#ff4d4f', fontSize: 16 }} />
              <Text>显示近{statsDays}天内状态码 ≥ 400 的异常操作</Text>
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
            <Table
              dataSource={stats?.alerts || []}
              rowKey="id"
              size="small"
              pagination={{ pageSize: 50, showTotal: (total) => `共 ${total} 条`, showQuickJumper: false, showSizeChanger: false }}
              columns={[
                { title: '时间', dataIndex: 'created_at', width: 160 },
                { title: '用户', dataIndex: 'username', width: 100 },
                { title: '操作', dataIndex: 'action', width: 80, render: (v: string) => <Tag color={getMethodColor(v)}>{v}</Tag> },
                { title: '资源', dataIndex: 'resource', ellipsis: true },
                {
                  title: '状态', dataIndex: 'status', width: 80,
                  render: (v: number) => <Tag color={v >= 500 ? 'error' : 'warning'}>{v}</Tag>,
                },
                { title: 'IP', dataIndex: 'ip', width: 130 },
              ]}
            />
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
              rowKey={(record, index) => `${record.username}-${record.time}-${index}`}
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
            <Descriptions.Item label="资源路径"><Text copyable>{detailItem.resource}</Text></Descriptions.Item>
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
                      try { obj.detail = JSON.parse(obj.detail); } catch {}
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
