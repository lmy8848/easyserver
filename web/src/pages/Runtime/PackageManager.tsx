import { Modal, Table, Button, Space, Tag, Form, Input, Select, Popconfirm } from 'antd';
import {
  PlusOutlined,
  DeleteOutlined,
  SyncOutlined,
  ReloadOutlined,
  AppstoreOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import type { PackageInfo, PackageSearchResult, RuntimeEnvironment } from './types';

interface PackageManagerProps {
  visible: boolean;
  selectedRuntime: RuntimeEnvironment | null;
  packageData: PackageInfo[];
  packageLoading: boolean;
  packageSearchResults: PackageSearchResult[];
  packageVersions: string[];
  packageVersionsLoading: boolean;
  onClose: () => void;
  onScanPackages: () => void;
  onRefreshPackages: () => void;
  onInstallPackage: (values: { name: string; version: string }) => void;
  onSearchPackages: (query: string) => void;
  onSelectPackage: (packageName: string) => void;
  onUpdatePackage: (name: string) => void;
  onUninstallPackage: (name: string) => void;
}

export default function PackageManager({
  visible,
  selectedRuntime,
  packageData,
  packageLoading,
  packageSearchResults,
  packageVersions,
  packageVersionsLoading,
  onClose,
  onScanPackages,
  onRefreshPackages,
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
        <div>
          <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between' }}>
            <Space>
              <Button
                icon={<SearchOutlined />}
                onClick={onScanPackages}
                loading={packageLoading}
              >
                扫描已安装的包
              </Button>
              <Button
                icon={<ReloadOutlined />}
                onClick={onRefreshPackages}
              >
                刷新
              </Button>
            </Space>
          </div>

          <div style={{ marginBottom: 16, padding: 16, background: '#f5f5f5', borderRadius: 4 }}>
            <div style={{ marginBottom: 8, fontWeight: 'bold' }}>安装新包</div>
            <Form form={packageForm} onFinish={onInstallPackage} layout="inline">
              <div style={{ position: 'relative' }}>
                <Form.Item name="name" rules={[{ required: true, message: '请输入包名' }]}>
                  <Input
                    placeholder="输入包名搜索..."
                    size="small"
                    style={{ width: 200 }}
                    onChange={(e) => onSearchPackages(e.target.value)}
                  />
                </Form.Item>
                {packageSearchResults.length > 0 && (
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
                    {packageSearchResults.map((pkg, index) => (
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
                    ))}
                  </div>
                )}
              </div>
              <Form.Item name="version">
                <Select
                  placeholder="选择版本"
                  size="small"
                  style={{ width: 150 }}
                  loading={packageVersionsLoading}
                  allowClear
                  showSearch
                >
                  {packageVersions.map((v, index) => (
                    <Select.Option key={index} value={v}>{v}</Select.Option>
                  ))}
                </Select>
              </Form.Item>
              <Form.Item>
                <Button type="primary" htmlType="submit" size="small" icon={<PlusOutlined />}>
                  安装
                </Button>
              </Form.Item>
            </Form>
          </div>

          {packageData.length === 0 ? (
            <div style={{ textAlign: 'center', padding: 40, color: '#999' }}>
              暂无已安装的包，点击"扫描"按钮检测
            </div>
          ) : (
            <Table
              dataSource={packageData}
              rowKey="id"
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
                        onClick={() => onUpdatePackage(record.name)}
                      >
                        更新
                      </Button>
                      <Popconfirm
                        title={`确定要卸载 ${record.name} 吗？`}
                        onConfirm={() => onUninstallPackage(record.name)}
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
      )}
    </Modal>
  );
}
