import { useState, useEffect, useMemo, useCallback } from 'react';
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
  type: string;
  created_at: string;
}

interface ParsedDetail {
  method: string;
  path: string;
  status: string;
  duration: string;
  summary: string;
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
  const [actions, setActions] = useState<string[]>([]);
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
        summary: inner.command || inner.detail || inner.file_path || obj.command || obj.detail || obj.file_path || '-',
        body: inner.body || obj.body,
        params: inner.params || obj.params,
        query: inner.query || obj.query,
      };
    } catch {
      return { method: '-', path: detail, status: '-', duration: '-', summary: detail };
    }
  };

  const fetchActions = useCallback(async () => {
    try {
      const res = await auditApi.getActions(activeTab === 'operation' || activeTab === 'request' ? activeTab : undefined);
      setActions(res.data.data || []);
    } catch (error) {
      console.error('Failed to fetch actions:', error);
    }
  }, [activeTab]);

  const fetchLogs = useCallback(async () => {
    if (activeTab !== 'operation' && activeTab !== 'request') return;
    setLoading(true);
    try {
      const params: { page: number; page_size: number; username?: string; action?: string; resource?: string; ip?: string; start_date?: string; end_date?: string; type?: string } = { page, page_size: pageSize };
      if (username) params['username'] = username;
      if (actionFilter) params['action'] = actionFilter;
      if (resource) params['resource'] = resource;
      if (ipFilter) params['ip'] = ipFilter;
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
  }, [page, pageSize, username, actionFilter, resource, ipFilter, dateRange, activeTab]);

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
    fetchActions();
    fetchStats(); // 初始加载统计数据，显示异常告警角标
  }, [fetchStats, fetchActions]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  useEffect(() => {
    if (activeTab === 'stats' || activeTab === 'alerts') {
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

  const getActionColor = (action: string) => {
    const act = action.toUpperCase();
    
    // Explicit overrides
    if (act === 'SECURITY_LOGIN_SUCCESS' || act === 'AUTH_LOGIN') return 'green';
    if (act === 'SECURITY_LOGIN_FAILED') return 'red';
    if (act === 'TERMINAL_OPEN' || act === 'TERMINAL_CLOSE') return 'purple';

    // Substring matching
    if (act.includes('DELETE') || act.includes('DROP') || act.includes('STOP') || act.includes('KICK') || act.includes('UNINSTALL') || act.includes('DISABLE') || act.includes('DOWN') || act.includes('LOGOUT') || act.includes('CLEAN') || act.includes('FAILED')) {
      return 'red';
    }
    if (act.includes('UPDATE') || act.includes('MODIFY') || act.includes('EDIT') || act.includes('RENAME') || act.includes('RESTART') || act.includes('SET') || act.includes('CHMOD') || act.includes('CHOWN') || act.includes('SAVE') || act.includes('RELOAD') || act.includes('PAUSE') || act.includes('RESTORE') || act.includes('CHANGE') || act.includes('CONFIG') || act.includes('MOVE') || act.includes('MARK')) {
      return 'orange';
    }
    if (act.includes('CREATE') || act.includes('ADD') || act.includes('NEW') || act.includes('INSERT') || act.includes('START') || act.includes('INSTALL') || act.includes('ENABLE') || act.includes('UPLOAD') || act.includes('COPY') || act.includes('PULL') || act.includes('MKDIR') || act.includes('GRANT') || act.includes('UP') || act.includes('EXECUTE') || act.includes('EXEC') || act.includes('RUN') || act.includes('APPLY') || act.includes('IMPORT') || act.includes('DOWNLOAD') || act.includes('COMPRESS') || act.includes('EXTRACT')) {
      return 'green';
    }
    
    return 'blue';
  };

  const getMethodColor = (method: string) => {
    const colors: Record<string, string> = {
      POST: 'blue', PUT: 'orange', DELETE: 'red', GET: 'green',
    };
    return colors[method] || 'default';
  };

  const getResourceText = (record: AuditLogItem) => {
    if (record.type !== 'operation') {
      return record.resource;
    }
    const action = record.action || '';
    const parsed = parseDetail(record.detail);
    const body = parsed.body || {};
    const params = parsed.params || {};
    const query = parsed.query || {};

    if (action === 'FILE_DELETE') {
      if (body.paths && Array.isArray(body.paths)) return `文件: ${body.paths.join(', ')}`;
    }
    if (action.startsWith('FILE_') && action !== 'FILE_DELETE') {
      const path = body.path || query['path'] || body.old_path || body.target;
      if (path) return `路径: ${path}`;
    }
    if (action.startsWith('PROCESS_GROUP_')) {
      if (params['id']) return `守护进程组 (ID: ${params['id']})`;
    } else if (action.startsWith('PROCESS_')) {
      if (params['id']) return `守护进程 (ID: ${params['id']})`;
    }
    if (action.startsWith('CONTAINER_')) {
      if (params['id']) return `容器 (ID: ${params['id']})`;
    }
    if (action.startsWith('DATABASE_') && action.includes('VERSION')) {
      if (params['vid']) return `数据库版本 (ID: ${params['vid']})`;
    }
    if (action.startsWith('DATABASE_') && action.includes('RECORD')) {
      if (params['did']) return `数据记录 (库 ID: ${params['did']})`;
    }
    if (action.startsWith('DATABASE_') && (action.includes('USER') || action.includes('PRIVILEGES') || action.includes('TABLE') || action.includes('BACKUP') || action.includes('SQL'))) {
      if (params['id']) return `数据库服务器 (ID: ${params['id']})`;
      if (params['did']) return `数据库 (ID: ${params['did']})`;
    }
    if (action.startsWith('DEPLOY_') && action.includes('SERVER')) {
      if (params['id']) return `发布服务器 (ID: ${params['id']})`;
    }
    if (action.startsWith('DEPLOY_') && action.includes('TASK')) {
      if (params['id']) return `发布任务 (ID: ${params['id']})`;
    }
    if (action.startsWith('ENV_') || action.startsWith('RUNTIME_')) {
      if (params['id']) return `环境配置 (ID: ${params['id']})`;
    }
    return record.resource;
  };

  const getActionLabel = (action: string) => {
    const labels: Record<string, string> = {
      TERMINAL_OPEN: '打开终端', TERMINAL_CLOSE: '关闭终端',
      FILE_MKDIR: '创建目录', FILE_UPLOAD: '上传文件', FILE_DOWNLOAD: '下载文件',
      FILE_RENAME: '重命名文件', FILE_DELETE: '删除文件', FILE_MOVE: '移动文件', FILE_COPY: '复制文件', FILE_EDIT: '编辑文件',
      FILE_CHMOD: '修改权限', FILE_CHOWN: '修改所有者', FILE_COMPRESS: '压缩文件', FILE_EXTRACT: '解压文件',
      SECURITY_LOGIN_SUCCESS: '登录成功', SECURITY_LOGIN_FAILED: '登录失败',
      SECURITY_LOGOUT: '退出登录', SECURITY_PASSWORD_CHANGED: '修改密码',
      SECURITY_PASSWORD_CHANGE_FAILED: '修改密码失败', SECURITY_TOTP_ENABLED: '开启两步验证', SECURITY_TOTP_DISABLED: '关闭两步验证',
      SECURITY_SESSION_KICKED: '踢出下线', SECURITY_ALL_OTHER_SESSIONS_KICKED: '下线其他设备',
      SYSTEM_SERVER_START: '面板启动', SYSTEM_SERVER_STOP: '面板停止',
      SYSTEM_SERVICE_FAILED: '服务异常', SYSTEM_DISK_WARNING: '磁盘警告',
      SERVICE_START: '启动服务', SERVICE_STOP: '停止服务', SERVICE_RESTART: '重启服务',
      SERVICE_ENABLE: '启用服务', SERVICE_DISABLE: '禁用服务',
      RUNTIME_INSTALL: '安装运行环境', RUNTIME_UNINSTALL: '卸载运行环境', RUNTIME_SET_DEFAULT: '设置默认运行环境',
      CRON_TASKS_CREATE: '创建计划任务', CRON_TASKS_UPDATE: '更新计划任务', CRON_TASKS_DELETE: '删除计划任务',
      CRON_TASKS_ENABLE: '启用计划任务', CRON_TASKS_DISABLE: '禁用计划任务', CRON_TASKS_RUN: '运行计划任务',
      CRON_SCRIPTS_CREATE: '创建计划任务脚本', CRON_SCRIPTS_UPDATE: '更新计划任务脚本', CRON_SCRIPTS_DELETE: '删除计划任务脚本',
      CRON_DOCS_CREATE: '创建计划任务文档', CRON_DOCS_UPDATE: '更新计划任务文档', CRON_DOCS_DELETE: '删除计划任务文档',
      DOCKER_INSTALL: '安装Docker', DOCKER_START: '启动Docker', DOCKER_STOP: '停止Docker', DOCKER_RESTART: '重启Docker',
      CONTAINERS_CREATE: '创建容器', CONTAINERS_START: '启动容器', CONTAINERS_STOP: '停止容器', CONTAINERS_RESTART: '重启容器', CONTAINERS_DELETE: '删除容器',
      IMAGES_PULL: '拉取镜像', IMAGES_DELETE: '删除镜像',
      COMPOSE_UP: '启动Compose', COMPOSE_DOWN: '停止Compose', COMPOSE_RESTART: '重启Compose',
      VOLUMES_CREATE: '创建数据卷', VOLUMES_DELETE: '删除数据卷', NETWORKS_CREATE: '创建网络', NETWORKS_DELETE: '删除网络',
      CLOUD_INSTANCES_START: '启动云主机', CLOUD_INSTANCES_STOP: '停止云主机', CLOUD_INSTANCES_RESTART: '重启云主机',
      CLOUD_FIREWALL_ADD: '添加云防火墙规则', CLOUD_FIREWALL_DELETE: '删除云防火墙规则', CLOUD_SNAPSHOTS_CREATE: '创建云快照', CLOUD_SNAPSHOTS_APPLY: '恢复云快照',
      WEBSERVERS_CREATE: '创建Web服务', WEBSERVERS_DELETE: '删除Web服务', WEBSERVERS_INSTALL: '安装Web服务', WEBSERVERS_UNINSTALL: '卸载Web服务', WEBSERVERS_START: '启动Web服务', WEBSERVERS_STOP: '停止Web服务', WEBSERVERS_RESTART: '重启Web服务', WEBSERVERS_UPDATE_CONFIG: '更新Web服务配置',
      WEBSITES_CREATE: '创建网站', WEBSITES_UPDATE: '更新网站', WEBSITES_DELETE: '删除网站', WEBSITES_ENABLE: '启用网站', WEBSITES_DISABLE: '禁用网站', WEBSITES_APPLY_SSL: '申请网站SSL',
      DBSERVERS_INSTALL: '安装数据库', DBSERVERS_UNINSTALL: '卸载数据库', DATABASES_CREATE: '创建数据库', DATABASES_DELETE: '删除数据库',
      FIREWALL_ENABLE: '启用防火墙', FIREWALL_DISABLE: '禁用防火墙', FIREWALL_SET_DEFAULT_POLICY: '设置防火墙默认策略',
      FIREWALL_RULES_CREATE: '创建防火墙规则', FIREWALL_RULES_IMPORT: '导入防火墙规则', FIREWALL_RULES_BULK_ENABLE: '批量启用防火墙规则', FIREWALL_RULES_BULK_DISABLE: '批量禁用防火墙规则', FIREWALL_RULES_BULK_DELETE: '批量删除防火墙规则', FIREWALL_RULES_UPDATE: '更新防火墙规则', FIREWALL_RULES_DELETE: '删除防火墙规则', FIREWALL_RULES_ENABLE: '启用防火墙规则', FIREWALL_RULES_DISABLE: '禁用防火墙规则', FIREWALL_RULES_MOVE_UP: '上移防火墙规则', FIREWALL_RULES_MOVE_DOWN: '下移防火墙规则',
      FIREWALL_TEMPLATES_APPLY: '应用防火墙模板',
      AUTH_LOGOUT: '登出账号', AUTH_CHANGE_PASSWORD: '修改密码', AUTH_TOTP_ENABLE: '启用二次验证', AUTH_TOTP_DISABLE: '禁用二次验证',
      DATABASE_START_VERSION: '启动数据库版本', DATABASE_STOP_VERSION: '停止数据库版本', DATABASE_RESTART_VERSION: '重启数据库版本', DATABASE_UPDATE_PORT: '修改数据库端口',
      DATABASE_CREATE_USER: '创建数据库用户', DATABASE_DELETE_USER: '删除数据库用户', DATABASE_GRANT_PRIVILEGES: '授权数据库用户',
      DATABASE_EXECUTE_SQL: '执行 SQL', DATABASE_INSERT_RECORD: '插入数据记录', DATABASE_UPDATE_RECORD: '更新数据记录', DATABASE_DELETE_RECORD: '删除数据记录',
      DATABASE_CREATE_TABLE: '创建数据表', DATABASE_DROP_TABLE: '删除数据表',
      DATABASE_CREATE_BACKUP: '创建数据库备份', DATABASE_RESTORE_BACKUP: '恢复数据库备份', DATABASE_DELETE_BACKUP: '删除数据库备份',
      DATABASE_SAVE_MYSQL_CONFIG: '保存 MySQL 配置', DATABASE_SAVE_POSTGRES_CONFIG: '保存 PostgreSQL 配置', DATABASE_SAVE_REDIS_CONFIG: '保存 Redis 配置',
      AUTH_LOGIN: '登录系统', AUTH_VERIFY_TOTP: '验证 TOTP', AUTH_VERIFY_BACKUP: '验证备用码', AUTH_SETUP_TOTP: '设置 TOTP', AUTH_KICK_SESSION: '踢出会话', AUTH_KICK_ALL_SESSIONS: '踢出所有其他设备',
      PROCESS_CREATE: '创建守护进程', PROCESS_UPDATE: '更新守护进程', PROCESS_DELETE: '删除守护进程',
      PROCESS_START: '启动守护进程', PROCESS_STOP: '停止守护进程', PROCESS_RESTART: '重启守护进程',
      PROCESS_BATCH_START: '批量启动守护进程', PROCESS_BATCH_STOP: '批量停止守护进程', PROCESS_BATCH_RESTART: '批量重启守护进程',
      PROCESS_GROUP_CREATE: '创建守护进程组', PROCESS_GROUP_UPDATE: '更新守护进程组', PROCESS_GROUP_DELETE: '删除守护进程组', PROCESS_IMPORT: '导入守护进程',
      ENV_CREATE_CONFIG: '创建环境变量', ENV_UPDATE_CONFIG: '更新环境变量', ENV_DELETE_CONFIG: '删除环境变量',
      ENV_CREATE_PATH: '创建 PATH', ENV_DELETE_PATH: '删除 PATH',
      ENV_CREATE_GLOBAL: '创建全局变量', ENV_UPDATE_GLOBAL: '更新全局变量', ENV_DELETE_GLOBAL: '删除全局变量',
      DEPLOY_CREATE_SERVER: '创建发布服务器', DEPLOY_UPDATE_SERVER: '更新发布服务器', DEPLOY_DELETE_SERVER: '删除发布服务器', DEPLOY_TEST_SERVER: '测试发布服务器',
      DEPLOY_CREATE_TASK: '创建发布任务', DEPLOY_DELETE_TASK: '删除发布任务', DEPLOY_EXECUTE_TASK: '执行发布任务', DEPLOY_ROLLBACK_VERSION: '发布版本回滚',
      CONTAINER_CONFIGURE_MIRROR: '配置 Docker 镜像源', CONTAINER_PAUSE: '暂停容器', CONTAINER_UNPAUSE: '恢复容器',
      CONTAINER_EXEC: '进入容器终端', CONTAINER_COPY_TO: '复制文件到容器', CONTAINER_COPY_FROM: '从容器复制文件',
      CONTAINER_RENAME: '重命名容器', CONTAINER_UPDATE: '更新容器', COMPOSE_SAVE_CONFIG: '保存 Compose 配置',
      SSH_SAVE_CONFIG: '保存 SSH 配置', SSH_TEST_CONFIG: '测试 SSH 配置', SSH_RELOAD: '重载 SSH', SSH_KILL_SESSION: '断开 SSH 会话',
      RUNTIME_UPDATE_MIRROR: '更新运行环境镜像源', RUNTIME_CREATE_MIRROR: '创建运行环境镜像源', RUNTIME_DELETE_MIRROR: '删除运行环境镜像源',
      PACKAGE_INSTALL: '安装环境包', PACKAGE_UNINSTALL: '卸载环境包', PACKAGE_UPDATE: '更新环境包', PACKAGE_SET_REGISTRY: '设置环境包镜像源',
      FIREWALL_DELETE_SYSTEM_RULE: '删除内置防火墙规则',
      SETTINGS_UPDATE_SERVER: '更新面板设置', SETTINGS_UPDATE_AUTH: '更新安全设置', SETTINGS_UPDATE_MONITOR: '更新监控设置', SETTINGS_UPDATE_AUDIT: '更新审计设置',
      SETTINGS_UPDATE_NOTIFY: '更新通知设置', SETTINGS_TEST_WEBHOOK: '测试 Webhook', ALERTS_UPDATE_RULES: '更新告警规则',
      SETTINGS_UPDATE_CLOUD: '更新云账号设置', SETTINGS_TEST_CLOUD: '测试云账号', PANEL_RESTART: '重启面板',
      NOTIFICATION_MARK_READ: '标记通知已读', NOTIFICATION_MARK_ALL_READ: '标记所有通知已读', NOTIFICATION_DELETE: '删除通知',
      AUDIT_CLEAN: '清空审计日志', FILE_SAVE_CONTENT: '保存文件内容',
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
      ellipsis: true,
      render: (_: string, record: AuditLogItem) => {
        const text = getResourceText(record);
        return <Tooltip title={text}><span style={{ fontSize: 12 }}>{text || '-'}</span></Tooltip>;
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
      render: (_: unknown, record: AuditLogItem) => parseDetail(record.detail).summary,
    };
    // operation 日志只展示资源和摘要列；request 日志展示方法/路径/状态/耗时
    const showRequestCols = activeTab === 'request';
    const showResourceCol = activeTab === 'operation';
    return [
      timeCol, userCol, actionCol,
      ...(showResourceCol ? [resourceCol, summaryCol] : []),
      ...(showRequestCols ? [methodCol, pathCol, statusCol, durationCol] : []),
      ipCol,
      detailCol,
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
                { label: <Badge key="alerts-badge" count={stats?.alerts?.length || 0} size="small"><span style={{ padding: '0 8px' }}>异常告警</span></Badge>, value: 'alerts' },
                { label: 'SSH 登录', value: 'ssh' },
              ]}
              value={activeTab}
              onChange={(v) => {
                setActiveTab(v as string);
                setUsername('');
                setActionFilter('');
                setResource('');
                setIpFilter('');
                setDateRange(null);
                setPage(1);
              }}
            />
            <Button icon={<DownloadOutlined />} onClick={handleExport}>导出CSV</Button>
            <Button icon={<ReloadOutlined />} onClick={() => { fetchLogs(); fetchStats(); }}>刷新</Button>
            <Button danger icon={<DeleteOutlined />} onClick={handleClean}>清理90天前</Button>
          </Space>
        }
      >
        {(activeTab === 'operation' || activeTab === 'request') && (
          <>
            <Space wrap style={{ marginBottom: 16 }}>
              <Input placeholder="用户名" value={username} onChange={e => setUsername(e.target.value)} style={{ width: 120 }} allowClear />
              <Select placeholder={activeTab === 'request' ? '请求方法' : '操作类型'} value={actionFilter || undefined} onChange={v => setActionFilter(v || '')} style={{ width: 120 }} allowClear options={actions?.map(a => ({ label: a, value: a })) || []} />
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
                { title: '操作', dataIndex: 'action', width: 80, render: (v: string) => <Tag color={getActionColor(v)}>{v}</Tag> },
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
              rowKey={(r) => `${r.username}-${r.time}-${r.action}`}
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
