import { Card, Tabs } from 'antd';
import { TableOutlined, UserOutlined, CodeOutlined } from '@ant-design/icons';
import TablesTab from './TablesTab';
import UsersTab from './UsersTab';
import ConfigTab from './ConfigTab';
import type { DatabaseListProps } from './types';

// Instance detail container — a Tabs wrapper over the three per-tab components
// (表 / 用户 / 配置文件). One tab one file: TablesTab / UsersTab / ConfigTab,
// each owning its own rows and modals. Instance-level actions live in the
// InstanceHeader card above.
export default function DatabaseList({ tablesTab, usersTab, configTab }: DatabaseListProps) {
  return (
    <Card>
      <Tabs items={[
        { key: 'tables', label: <span><TableOutlined /> 表</span>, children: <TablesTab {...tablesTab} /> },
        { key: 'users', label: <span><UserOutlined /> 用户</span>, children: <UsersTab {...usersTab} /> },
        { key: 'config', label: <span><CodeOutlined /> 配置文件</span>, children: <ConfigTab {...configTab} /> },
      ]} />
    </Card>
  );
}
