import { Modal, Table, Button, Space, Tag, Form, Input, Select, Popconfirm, Spin, Empty } from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  SyncOutlined,
  ReloadOutlined,
  AppstoreOutlined,
  SearchOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import type { PackageInfo, PackageSearchResult, RuntimeEnvironment, CatalogEntry } from './types';

interface PackageManagerProps {
  catalog: CatalogEntry[];
  visible: boolean;
  selectedRuntime: RuntimeEnvironment | null;
  packageData: PackageInfo[];
  packageLoading: boolean;
  packageInstalling: boolean;
  packageSearchResults: PackageSearchResult[];
  packageSearchLoading: boolean;
  packageVersions: string[];
  packageVersionsLoading: boolean;
  updatingPackageName: string | null;
  onClose: () => void;
  onRefreshPackages: () => void;
  onConfigRegistry: () => void;
  onInstallPackage: (values: { name: string; version: string; manager?: string }) => void;
  onSearchPackages: (query: string) => void;
  onSelectPackage: (packageName: string) => void;
  onUpdatePackage: (pkg: PackageInfo) => void;
  onUninstallPackage: (pkg: PackageInfo) => void;
}

export default function PackageManager({
  catalog,
  visible,
  selectedRuntime,
  packageData,
  packageLoading,
  packageInstalling,
  packageSearchResults,
  packageSearchLoading,
  packageVersions,
  packageVersionsLoading,
  updatingPackageName,
  onClose,
  onRefreshPackages,
  onConfigRegistry,
  onInstallPackage,
  onSearchPackages,
  onSelectPackage,
  onUpdatePackage,
  onUninstallPackage,
}: PackageManagerProps) {
  const [packageForm] = Form.useForm();

  const handleSelectPackage = (packageName: string) => {
    packageForm.setFieldsValue({ name: packageName });
    onSelectPackage(packageName);
  };

  const handleClose = () => {
    onClose();
    packageForm.resetFields();
  };

  return (
    <Modal
      title={
        <Space>
          <AppstoreOutlined />
          <span>包管理</span>
          {selectedRuntime && (
            <Tag color="blue">
              {selectedRuntime.name} {selectedRuntime.version}
            </Tag>
          )}
        </Space>
      }
      open={visible}
      onCancel={handleClose}
      footer={null}
      width={800}
    >
      {selectedRuntime && (
        !catalog.find(c => c.lang === selectedRuntime.name)?.supports_global_pkgs ? (
          <div style={{ padding: '40px 0' }}>
            <Empty description={`当前运行环境 (${selectedRuntime.name}) 暂不支持面板全局包管理`} />
          </div>
        ) : (
        <div>
          <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
            <Space>
              <Button
                icon={<ReloadOutlined />}
                onClick={onRefreshPackages}
                loading={packageLoading}
              >
                刷新
              </Button>
              {['node', 'python'].includes(selectedRuntime?.name || '') && (
                <Button
                  icon={<SettingOutlined />}
                  onClick={onConfigRegistry}
                >
                  配置镜像
                </Button>
              )}
            </Space>
          </div>

          <div style={{ marginBottom: 16, padding: 16, background: '#f5f5f5', borderRadius: 4 }}>
            <div style={{ marginBottom: 8, fontWeight: 'bold' }}>安装新包</div>
            <Form form={packageForm} onFinish={onInstallPackage} layout="inline" initialValues={{ manager: 'npm' }}>
              {selectedRuntime?.name === 'node' && (
                <Form.Item name="manager">
                  <Select size="small" style={{ width: 80 }}>
                    <Select.Option value="npm">npm</Select.Option>
                    <Select.Option value="pnpm">pnpm</Select.Option>
                  </Select>
                </Form.Item>
              )}
              <div style={{ position: 'relative' }}>
                <Form.Item name="name" rules={[{ required: true, message: '请输入包名' }]}>
                  <Input.Search
                    placeholder="输入包名搜索..."
                    size="small"
                    style={{ width: 240 }}
                    enterButton={<SearchOutlined />}
                    onChange={(e) => onSearchPackages(e.target.value)}
                    onSearch={(value) => onSearchPackages(value)}
                    onPressEnter={(e) => e.preventDefault()}
                  />
                </Form.Item>
                {(packageSearchLoading || packageSearchResults.length > 0) && (
                  <div style={{
                    position: 'absolute',
                    top: '100%',
                    left: 0,
                    right: 0,
                    background: 'white',
                    border: '1px solid #d9d9d9',
                    borderRadius: 4,
                    maxHeight: 200,
                    overflow: 'auto',
                    zIndex: 1000,
                  }}>
                    {packageSearchLoading ? (
                      <div style={{ padding: 12, textAlign: 'center', color: '#999' }}>
                        <Spin size="small" /> <span style={{ marginLeft: 8 }}>搜索中...</span>
                      </div>
                    ) : (
                      packageSearchResults.map((pkg, index) => (
                        <div
                          key={index}
                          style={{
                            padding: '8px 12px',
                            cursor: 'pointer',
                            borderBottom: '1px solid #f0f0f0',
                          }}
                          onClick={() => handleSelectPackage(pkg.name)}
                        >
                          <div style={{ fontWeight: 'bold' }}>{pkg.name}</div>
                          <div style={{ fontSize: 12, color: '#999' }}>{pkg.description}</div>
                        </div>
                      ))
                    )}
                  </div>
                )}
              </div>
              <Form.Item name="version">
                <Select
                  placeholder="选择版本（默认最新）"
                  size="small"
                  style={{ width: 170 }}
                  loading={packageVersionsLoading}
                  allowClear
                  showSearch
                  onOpenChange={(open) => {
                    if (open) {
                      const currentName = packageForm.getFieldValue('name');
                      if (currentName) {
                        onSelectPackage(currentName);
                      }
                    }
                  }}
                >
                  {packageVersions.length > 0 && (
                    <Select.Option key="latest" value="latest">latest</Select.Option>
                  )}
                  {packageVersions.map((v) => (
                    <Select.Option key={v} value={v}>{v}</Select.Option>
                  ))}
                </Select>
              </Form.Item>
              <Form.Item>
                <Button type="primary" htmlType="submit" size="small" icon={<PlusOutlined />} loading={packageInstalling}>
                  安装
                </Button>
              </Form.Item>
            </Form>
          </div>

          {packageData.length === 0 ? (
            <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>
              暂无已安装的包，点击"刷新"获取最新列表
            </div>
          ) : (
            <Table
              dataSource={packageData}
              rowKey={(r) => `${r.source}:${r.name}`}
              loading={packageLoading}
              pagination={{ pageSize: 10 }}
              size="small"
              columns={[
                { title: '包名', dataIndex: 'name', key: 'name' },
                {
                  title: '版本',
                  dataIndex: 'version',
                  key: 'version',
                  render: (v: string) => <Tag color="blue">{v}</Tag>,
                },
                {
                  title: '范围',
                  dataIndex: 'scope',
                  key: 'scope',
                  render: (s: string) => <Tag color={s === 'global' ? 'green' : 'orange'}>{s}</Tag>,
                },
                { title: '来源', dataIndex: 'source', key: 'source' },
                {
                  title: '操作',
                  key: 'action',
                  width: 120,
                  render: (_: unknown, record: PackageInfo) => (
                    <Space>
                      <Button
                        type="link"
                        size="small"
                        icon={<SyncOutlined />}
                        loading={updatingPackageName === record.name}
                        onClick={() => onUpdatePackage(record)}
                      >
                        更新
                      </Button>
                      <Popconfirm
                        title={`确定要卸载 ${record.name} 吗？`}
                        onConfirm={() => onUninstallPackage(record)}
                      >
                        <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                          卸载
                        </Button>
                      </Popconfirm>
                    </Space>
                  ),
                },
              ]}
            />
          )}
        </div>
        )
      )}
    </Modal>
  );
}
