import {
  Card, Button, Space, Tag, Modal, Form, Input, Select, InputNumber,
  Popconfirm, Row, Col, Table, Tabs, Empty, Spin,
} from 'antd';
import {
  DatabaseOutlined, PlusOutlined, DeleteOutlined, ReloadOutlined,
  StopOutlined, PlayCircleOutlined,
  FileTextOutlined, UserOutlined, KeyOutlined,
  CodeOutlined, ArrowLeftOutlined, TableOutlined,
} from '@ant-design/icons';
import STYLES from './styles';
import type { DatabaseListProps, Database as DBType, DBUser } from './types';
import { getServiceStatusColor, ServiceStatusTag } from '../../utils/status';

export default function DatabaseList({
  server, version, databases, dbsLoading, dbUsers, usersLoading, operating,
  onBack, onEnterDatabase, onRefreshDatabases, onRefreshUsers,
  onDeleteDB, onDeleteUser, onStartVersion, onStopVersion, onRestartVersion,
  dbModalVisible, onDbModalVisibleChange, dbForm, onCreateDB,
  userModalVisible, onUserModalVisibleChange, userForm, onCreateUser,
  grantVisible, grantUser, grantForm, onGrantVisibleChange, onGrant, onOpenGrant,
  dbConfig, dbConfigLoading, onFetchDBConfig, onSaveDBConfig, onUpdateDBParam,
  logVisible, logVersion, logContent, logLoading, logFollow, logRef,
  onLogVisibleChange, onLogFollowChange,
  showConfig, showLogs,
}: DatabaseListProps) {

  const statusColor = (status: string) => {
    const colorName = getServiceStatusColor(status);
    const colorMap: Record<string, string> = {
      success: '#52c41a', error: '#ff4d4f', warning: '#faad14', default: '#999',
    };
    return colorMap[colorName] || '#999';
  };

  const statusTag = (status: string) => <ServiceStatusTag status={status} />;

  const versionDatabases = databases.filter(d => d.db_version_id === version.id);

  const dbColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60, responsive: ['md'] as ('md' | 'lg' | 'xl' | 'xs' | 'sm' | 'xxl' | 'xxxl')[] },
    { title: '数据库名', dataIndex: 'name', key: 'name', render: (t: string) => <strong>{t}</strong> },
    { title: '字符集', dataIndex: 'charset', key: 'charset', width: 100, responsive: ['lg'] as ('md' | 'lg' | 'xl' | 'xs' | 'sm' | 'xxl' | 'xxxl')[] },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true, responsive: ['md'] as ('md' | 'lg' | 'xl' | 'xs' | 'sm' | 'xxl' | 'xxxl')[] },
    {
      title: '操作', key: 'action', width: 200,
      render: (_: unknown, record: DBType) => (
        <Space size="small">
          <Button type="link" size="small" icon={<TableOutlined />} onClick={() => onEnterDatabase(record)}>管理</Button>
          <Popconfirm title="确定删除此数据库？" onConfirm={() => onDeleteDB(record.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const userColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', width: 60, responsive: ['md'] as ('md' | 'lg' | 'xl' | 'xs' | 'sm' | 'xxl' | 'xxxl')[] },
    { title: '用户名', dataIndex: 'username', key: 'username', render: (t: string) => <strong>{t}</strong> },
    { title: '主机', dataIndex: 'host', key: 'host', width: 120, responsive: ['lg'] as ('md' | 'lg' | 'xl' | 'xs' | 'sm' | 'xxl' | 'xxxl')[] },
    { title: '权限', dataIndex: 'privileges', key: 'privileges', ellipsis: true, responsive: ['md'] as ('md' | 'lg' | 'xl' | 'xs' | 'sm' | 'xxl' | 'xxxl')[] },
    {
      title: '操作', key: 'action', width: 180,
      render: (_: unknown, record: DBUser) => (
        <Space size="small">
          <Button type="link" size="small" icon={<KeyOutlined />}
            onClick={() => onOpenGrant(record)}>授权</Button>
          <Popconfirm title="确定删除此用户？" onConfirm={() => onDeleteUser(record.id)}>
            <Button type="link" size="small" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Card style={{ marginBottom: 16 }}>
        <Row justify="space-between" align="middle">
          <Col>
            <Space size="middle">
              <Button icon={<ArrowLeftOutlined />} onClick={onBack}>返回</Button>
              <DatabaseOutlined style={{ fontSize: 24, color: statusColor(version.status) }} />
              <div>
                <Space>
                  <span style={{ fontSize: 18, fontWeight: 'bold' }}>{server.display_name} {version.version}</span>
                  {statusTag(version.status)}
                </Space>
                <div style={STYLES.versionInfo}>
                  <Space size="middle">
                    <span>服务: <strong>{version.service_name}</strong></span>
                    <span>端口: <strong>{version.port}</strong></span>
                  </Space>
                </div>
              </div>
            </Space>
          </Col>
          <Col>
            <Space wrap>
              {version.status === 'running' ? (
                <>
                  <Button icon={<StopOutlined />} danger loading={operating === `stop-${version.id}`}
                    onClick={() => onStopVersion(version)}>停止</Button>
                  <Button icon={<ReloadOutlined />} loading={operating === `restart-${version.id}`}
                    onClick={() => onRestartVersion(version)}>重启</Button>
                </>
              ) : (
                <Button type="primary" icon={<PlayCircleOutlined />} loading={operating === `start-${version.id}`}
                  onClick={() => onStartVersion(version)}>启动</Button>
              )}
              <Button icon={<CodeOutlined />} onClick={() => showConfig(version)}>配置文件</Button>
              <Button icon={<FileTextOutlined />} onClick={() => showLogs(version)}>服务日志</Button>
            </Space>
          </Col>
        </Row>
        {version.status === 'running' && (
          <div style={{ marginTop: 12, padding: '8px 0', borderTop: '1px solid #f0f0f0' }}>
            <Space size="large">
              {version.pid && version.pid > 0 && <span>PID: <strong>{version.pid}</strong></span>}
              {version.memory_bytes && version.memory_bytes > 0 && <span>内存: <strong>{(version.memory_bytes / 1024 / 1024).toFixed(1)} MB</strong></span>}
              {version.uptime && <span>运行时间: <strong>{version.uptime}</strong></span>}
              {version.connections !== undefined && <span>连接数: <strong>{version.connections}</strong></span>}
              <span>配置: <Tag>{version.config_file || 'N/A'}</Tag></span>
            </Space>
          </div>
        )}
      </Card>

      <Card>
        <Tabs items={[
          {
            key: 'databases',
            label: <span><DatabaseOutlined /> 数据库 ({versionDatabases.length})</span>,
            children: (
              <div>
                <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                  <Button icon={<ReloadOutlined />} loading={dbsLoading} onClick={onRefreshDatabases}>刷新</Button>
                  <Button type="primary" icon={<PlusOutlined />}
                    onClick={() => { dbForm.resetFields(); onDbModalVisibleChange(true); }}
                    disabled={version.status !== 'running'}>创建数据库</Button>
                </div>
                <Table columns={dbColumns} dataSource={versionDatabases} rowKey="id" loading={dbsLoading} size="small"
                  locale={{ emptyText: <Empty description="暂无数据库" /> }} />
              </div>
            ),
          },
          {
            key: 'users',
            label: <span><UserOutlined /> 用户 ({dbUsers.length})</span>,
            children: (
              <div>
                <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                  <Button icon={<ReloadOutlined />} loading={usersLoading} onClick={onRefreshUsers}>刷新</Button>
                  <Button type="primary" icon={<PlusOutlined />}
                    onClick={() => { userForm.resetFields(); onUserModalVisibleChange(true); }}
                    disabled={version.status !== 'running'}>创建用户</Button>
                </div>
                <Table columns={userColumns} dataSource={dbUsers} rowKey="id" loading={usersLoading} size="small"
                  locale={{ emptyText: <Empty description="暂无用户" /> }} />
              </div>
            ),
          },
          ...(server.name === 'mysql' || server.name === 'postgresql' || server.name === 'redis' ? [{
            key: 'config',
            label: <span><CodeOutlined /> 配置文件</span>,
            children: (
              <div>
                {!dbConfig ? (
                  <div style={{ textAlign: 'center', padding: 40 }}>
                    <Button type="primary" icon={<CodeOutlined />} loading={dbConfigLoading}
                      onClick={onFetchDBConfig}>加载配置</Button>
                    <p style={{ color: '#999', marginTop: 12 }}>读取服务器上的配置文件</p>
                  </div>
                ) : dbConfig.found === false ? (
                  <Empty description={`未找到 ${server.display_name} 配置文件`} />
                ) : (
                  <div>
                    <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <Space>
                        <Tag>{dbConfig.config?.file_path}</Tag>
                        <span style={{ color: '#8c8c8c', fontSize: 12 }}>修改后需重启 {server.display_name} 生效</span>
                      </Space>
                      <Space>
                        <Button icon={<ReloadOutlined />} onClick={onFetchDBConfig}>重新加载</Button>
                        <Button type="primary" onClick={onSaveDBConfig}>保存配置</Button>
                      </Space>
                    </div>
                    <Tabs
                      type="card"
                      items={(dbConfig.config?.sections || []).map((section: any) => ({
                        key: section.name,
                        label: section.name === 'main' ? server.display_name : `[${section.name}]`,
                        children: (
                          <div>
                            {(dbConfig.sections?.[section.name]?.meta || []).map((param: any) => (
                              <div key={param.key} style={{ marginBottom: 16 }}>
                                <div style={{ marginBottom: 4 }}>
                                  <strong>{param.label}</strong>
                                  <span style={{ color: '#8c8c8c', marginLeft: 8, fontSize: 12 }}>{param.key}</span>
                                  {param.unit && <Tag style={{ marginLeft: 8 }}>{param.unit}</Tag>}
                                </div>
                                <div style={{ color: '#666', fontSize: 12, marginBottom: 4 }}>{param.description}</div>
                                {param.type === 'select' ? (
                                  <Select
                                    value={section.params?.[param.key] || param.default}
                                    onChange={(val) => onUpdateDBParam(section.name, param.key, val)}
                                    style={{ width: 300 }}
                                  >
                                    {(param.options || []).map((opt: string) => (
                                      <Select.Option key={opt} value={opt}>{opt}</Select.Option>
                                    ))}
                                  </Select>
                                ) : param.type === 'number' ? (
                                  <InputNumber
                                    value={Number(section.params?.[param.key]) || Number(param.default)}
                                    onChange={(val) => onUpdateDBParam(section.name, param.key, String(val || ''))}
                                    style={{ width: 300 }}
                                  />
                                ) : (
                                  <Input
                                    value={section.params?.[param.key] || ''}
                                    onChange={(e) => onUpdateDBParam(section.name, param.key, e.target.value)}
                                    placeholder={param.default}
                                    style={{ width: 400 }}
                                  />
                                )}
                              </div>
                            ))}
                            {/* Show extra params not in common list */}
                            {Object.entries(section.params || {}).filter(([key]) =>
                              !(dbConfig.sections?.[section.name]?.meta || []).some((m: any) => m.key === key)
                            ).map(([key, value]) => (
                              <div key={key} style={{ marginBottom: 16 }}>
                                <div style={{ marginBottom: 4 }}>
                                  <Tag color="default">{key}</Tag>
                                </div>
                                <Input
                                  value={value as string}
                                  onChange={(e) => onUpdateDBParam(section.name, key, e.target.value)}
                                  style={{ width: 400 }}
                                />
                              </div>
                            ))}
                          </div>
                        ),
                      }))}
                    />
                  </div>
                )}
              </div>
            ),
          }] : []),
        ]} />
      </Card>

      {/* Modals */}
      <Modal title="创建数据库" open={dbModalVisible} onCancel={() => onDbModalVisibleChange(false)}
        onOk={onCreateDB} okText="创建" cancelText="取消">
        <Form form={dbForm} layout="vertical">
          <Form.Item label="版本"><Input value={`${server.display_name} ${version.version}`} disabled /></Form.Item>
          <Form.Item name="name" label="数据库名" rules={[{ required: true }]}><Input placeholder="如：my_app" /></Form.Item>
          <Form.Item name="charset" label="字符集" initialValue="utf8mb4">
            <Select><Select.Option value="utf8mb4">utf8mb4</Select.Option><Select.Option value="utf8">utf8</Select.Option></Select>
          </Form.Item>
          <Form.Item name="description" label="描述"><Input placeholder="可选" /></Form.Item>
        </Form>
      </Modal>
      <Modal title="创建用户" open={userModalVisible} onCancel={() => onUserModalVisibleChange(false)}
        onOk={onCreateUser} okText="创建" cancelText="取消">
        <Form form={userForm} layout="vertical">
          <Form.Item name="username" label="用户名" rules={[{ required: true }]}><Input placeholder="如：app_user" /></Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true }, { min: 6 }]}><Input.Password /></Form.Item>
          <Form.Item name="host" label="主机" initialValue="localhost">
            <Select><Select.Option value="localhost">localhost</Select.Option><Select.Option value="%">任意主机（%）</Select.Option></Select>
          </Form.Item>
        </Form>
      </Modal>
      <Modal title={`授权 - ${grantUser?.username || ''}`} open={grantVisible} onCancel={() => onGrantVisibleChange(false)}
        onOk={onGrant} okText="授权" cancelText="取消">
        <Form form={grantForm} layout="vertical">
          <Form.Item name="db_version_id" hidden initialValue={version.id}><Input /></Form.Item>
          <Form.Item name="database" label="数据库" rules={[{ required: true }]}>
            <Select>{databases.filter(d => d.db_version_id === version.id).map(db => <Select.Option key={db.id} value={db.name}>{db.name}</Select.Option>)}</Select>
          </Form.Item>
          <Form.Item name="privileges" label="权限" rules={[{ required: true }]}>
            <Select mode="multiple">
              <Select.Option value="ALL PRIVILEGES">全部权限</Select.Option>
              <Select.Option value="SELECT">SELECT</Select.Option>
              <Select.Option value="INSERT">INSERT</Select.Option>
              <Select.Option value="UPDATE">UPDATE</Select.Option>
              <Select.Option value="DELETE">DELETE</Select.Option>
            </Select>
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title={<Space><FileTextOutlined /><span>{server.display_name} {logVersion?.version} - 服务日志</span>{logLoading && <Spin size="small" />}</Space>}
        open={logVisible} onCancel={() => onLogVisibleChange(false)}
        footer={<Row justify="space-between"><Col><Space><span style={{ color: '#8c8c8c', fontSize: 12 }}>每 5 秒自动刷新</span><span style={{ color: logFollow ? '#52c41a' : '#8c8c8c', fontSize: 12 }}>{logFollow ? '● 自动滚动' : '○ 已暂停'}</span></Space></Col><Col><Space><Button size="small" type={logFollow ? 'primary' : 'default'} onClick={() => onLogFollowChange(!logFollow)}>{logFollow ? 'Follow ON' : 'Follow OFF'}</Button><Button size="small" onClick={() => onLogVisibleChange(false)}>关闭</Button></Space></Col></Row>}
        width="90vw" style={{ maxWidth: 960 }}>
        <div ref={logRef} style={{ ...STYLES.logContainer }}>
          {logContent.split('\n').map((line, i) => (
            <div key={i} style={STYLES.logLine}>
              <span style={STYLES.logLineNumber}>{i + 1}</span>
              <span style={STYLES.logLineText}>{line || ' '}</span>
            </div>
          ))}
        </div>
      </Modal>
    </div>
  );
}
