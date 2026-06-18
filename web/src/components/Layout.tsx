import { useState, useEffect } from 'react';
import { Layout as AntLayout, Menu, Dropdown, Avatar, Space, Typography } from 'antd';
import {
  DashboardOutlined,
  SettingOutlined,
  CodeOutlined,
  FolderOutlined,
  UserOutlined,
  CloudOutlined,
  RocketOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  FileTextOutlined,
  ToolOutlined,
  ApiOutlined,
  ControlOutlined,
  GlobalOutlined,
  DatabaseOutlined,
} from '@ant-design/icons';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import ErrorBoundary from './ErrorBoundary';
import { useAuthStore } from '../store/useAuthStore';
import { hasPermission, PERMISSIONS, ROLE_LABELS } from '../utils/permissions';

const { Header, Sider, Content } = AntLayout;
const { Text } = Typography;

interface MenuItem {
  key: string;
  icon: React.ReactNode;
  label: string;
  permission?: readonly string[];
}

const menuItems: MenuItem[] = [
  {
    key: '/',
    icon: <DashboardOutlined />,
    label: '系统监控',
    permission: PERMISSIONS.MONITOR_VIEW,
  },
  {
    key: '/services',
    icon: <SettingOutlined />,
    label: '服务管理',
    permission: PERMISSIONS.SERVICE_VIEW,
  },
  {
    key: '/terminal',
    icon: <CodeOutlined />,
    label: '终端访问',
    permission: PERMISSIONS.TERMINAL_ACCESS,
  },
  {
    key: '/files',
    icon: <FolderOutlined />,
    label: '文件管理',
    permission: PERMISSIONS.FILE_VIEW,
  },
  {
    key: '/deploy',
    icon: <RocketOutlined />,
    label: '部署同步',
    permission: PERMISSIONS.DEPLOY_MANAGE,
  },
  {
    key: '/runtime',
    icon: <ApiOutlined />,
    label: '运行环境',
    permission: PERMISSIONS.DEPLOY_MANAGE, // admin only
  },
  {
    key: '/env-config',
    icon: <ControlOutlined />,
    label: '环境配置',
    permission: PERMISSIONS.DEPLOY_MANAGE, // admin only
  },
  {
    key: '/websites',
    icon: <GlobalOutlined />,
    label: '网站管理',
    permission: PERMISSIONS.WEBSITE_MANAGE, // admin only
  },
  {
    key: '/databases',
    icon: <DatabaseOutlined />,
    label: '数据库管理',
    permission: PERMISSIONS.DB_MANAGE, // admin only
  },
  {
    key: '/users',
    icon: <UserOutlined />,
    label: '用户管理',
    permission: PERMISSIONS.USER_MANAGE,
  },
  {
    key: '/cloud',
    icon: <CloudOutlined />,
    label: '腾讯云',
    permission: PERMISSIONS.CLOUD_VIEW,
  },
  {
    key: '/audit',
    icon: <FileTextOutlined />,
    label: '操作日志',
    permission: PERMISSIONS.AUDIT_VIEW,
  },
  {
    key: '/settings',
    icon: <ToolOutlined />,
    label: '面板设置',
    permission: PERMISSIONS.USER_MANAGE, // admin only
  },
];

export default function Layout() {
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout, loadUser } = useAuthStore();

  useEffect(() => {
    loadUser();
  }, []);

  const handleMenuClick = (info: { key: string }) => {
    navigate(info.key);
  };

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  // 根据用户角色过滤菜单
  const filteredMenuItems = menuItems
    .filter(item => {
      if (!item.permission) return true;
      return hasPermission(user?.role, item.permission);
    })
    .map(item => ({
      key: item.key,
      icon: item.icon,
      label: item.label,
    }));

  const userMenuItems = [
    {
      key: 'profile',
      label: '个人信息',
      icon: <UserOutlined />,
    },
    {
      key: 'password',
      label: '修改密码',
      icon: <SettingOutlined />,
    },
    {
      type: 'divider' as const,
    },
    {
      key: 'logout',
      label: '退出登录',
      icon: <LogoutOutlined />,
      danger: true,
    },
  ];

  const handleUserMenuClick = (info: { key: string }) => {
    switch (info.key) {
      case 'logout':
        handleLogout();
        break;
      case 'password':
        // TODO: show change password modal
        break;
    }
  };

  return (
    <AntLayout style={{ minHeight: '100vh' }}>
      <Sider
        trigger={null}
        collapsible
        collapsed={collapsed}
        theme="dark"
        width={200}
        style={{ position: 'fixed', height: '100vh', left: 0, top: 0, bottom: 0, zIndex: 10, overflow: 'hidden' }}
      >
        <div style={{
          height: 64,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#fff',
          fontSize: collapsed ? 16 : 20,
          fontWeight: 'bold',
        }}>
          {collapsed ? 'ES' : 'EasyServer'}
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={filteredMenuItems}
          onClick={handleMenuClick}
          style={{ flex: 1 }}
        />
        <div
          onClick={() => setCollapsed(!collapsed)}
          style={{
            height: 48,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            cursor: 'pointer',
            color: 'rgba(255,255,255,0.65)',
            borderTop: '1px solid rgba(255,255,255,0.1)',
            transition: 'color 0.2s',
          }}
          onMouseEnter={e => (e.currentTarget.style.color = '#fff')}
          onMouseLeave={e => (e.currentTarget.style.color = 'rgba(255,255,255,0.65)')}
        >
          {collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
        </div>
      </Sider>

      <AntLayout style={{ marginLeft: collapsed ? 48 : 200, transition: 'margin-left 0.2s' }}>
        <Header style={{
          background: '#fff',
          padding: '0 24px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          boxShadow: '0 1px 4px rgba(0,0,0,0.08)',
          position: 'sticky',
          top: 0,
          zIndex: 9,
        }}>
          <Space>
            <Text type="secondary">
              {ROLE_LABELS[user?.role || ''] || '未知角色'}
            </Text>
            <Dropdown
              menu={{ items: userMenuItems, onClick: handleUserMenuClick }}
              placement="bottomRight"
            >
              <Space style={{ cursor: 'pointer' }}>
                <Avatar icon={<UserOutlined />} />
                <Text>{user?.username || '用户'}</Text>
              </Space>
            </Dropdown>
          </Space>
        </Header>

        <Content style={{
          margin: 24,
          padding: 24,
          background: '#fff',
          borderRadius: 8,
          minHeight: 280,
        }}>
          <ErrorBoundary>
            <Outlet />
          </ErrorBoundary>
        </Content>
      </AntLayout>
    </AntLayout>
  );
}
