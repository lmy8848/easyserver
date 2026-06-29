import { useState } from 'react';
import { Modal, Form, Select, Input, Button, Space, Tag, message, Alert } from 'antd';
import { SyncOutlined } from '@ant-design/icons';
import type { VersionInfo, CatalogEntry } from './types';

interface VersionListProps {
  visible: boolean;
  onClose: () => void;
  selectedRuntime: string;
  versionsLoading: boolean;
  availableVersions: VersionInfo[];
  catalog: CatalogEntry[];
  installing: boolean;
  onInstall: (values: { name: string; version: string }) => void;
  onRuntimeChange: (value: string) => void;
  onRefreshVersions: (runtimeName: string) => void;
}

// pickLatestForMajor returns the newest sub-version under `major`.
// availableVersions comes from /runtime/:name/remote-versions sorted newest-first,
// so the first prefix match wins. major like "20" / "3.12" / "1.24".
function pickLatestForMajor(versions: VersionInfo[], major: string): VersionInfo | undefined {
  return versions.find(v => v.version === major || v.version.startsWith(major + '.'));
}

export default function VersionList({
  visible,
  onClose,
  selectedRuntime,
  versionsLoading,
  availableVersions,
  catalog,
  installing,
  onInstall,
  onRuntimeChange,
  onRefreshVersions,
}: VersionListProps) {
  const [form] = Form.useForm();
  const [majorLoading, setMajorLoading] = useState<string | null>(null);

  const handleClose = () => {
    onClose();
    form.resetFields();
  };

  const handleMajorClick = (major: string) => {
    if (majorLoading) return;
    setMajorLoading(major);
    try {
      let versionToSet = major;
      // 只有在加载完毕且有数据时，才进行严格的匹配和已装检测
      if (!versionsLoading && availableVersions.length > 0) {
        const target = pickLatestForMajor(availableVersions, major);
        if (!target) {
          message.warning(`未找到 ${major} 的可用版本，请刷新版本列表`);
          return;
        }
        if (target.installed) {
          message.warning(`版本 ${target.version} 已安装`);
          return;
        }
        versionToSet = target.version;
      }

      form.setFields([{ name: 'version', value: versionToSet, errors: [] }]);
      form.validateFields(['version']).catch(() => {});
    } finally {
      setMajorLoading(null);
    }
  };

  const currentMajors = catalog.find(c => c.lang === selectedRuntime)?.majors || [];

  const renderExtra = () => {
    if (!selectedRuntime) return null;

    return (
      <div style={{ marginTop: 8 }}>
        <Button
          type="link"
          size="small"
          icon={<SyncOutlined />}
          onClick={() => onRefreshVersions(selectedRuntime)}
          loading={versionsLoading}
          style={{ padding: '0', marginBottom: 8 }}
        >
          刷新版本列表
        </Button>
        {currentMajors.length > 0 && (
          <div>
            <span style={{ fontSize: 12, color: '#999' }}>快速选择主版本（自动取最新子版本）: </span>
            <Space size={4} wrap>
              {currentMajors.map(major => {
                let enabled = true;
                let installed = false;
                
                // 如果远程版本已加载完，才做精确校验和置灰
                if (!versionsLoading && availableVersions.length > 0) {
                  const hit = pickLatestForMajor(availableVersions, major);
                  enabled = !!hit && !hit.installed;
                  installed = !!hit?.installed;
                }

                return (
                  <Tag
                    key={major}
                    color={enabled ? 'blue' : 'default'}
                    style={{
                      cursor: enabled ? 'pointer' : 'not-allowed',
                      opacity: enabled ? 1 : 0.45,
                    }}
                    onClick={() => enabled && handleMajorClick(major)}
                  >
                    {major}{installed ? '（已装）' : ''}
                  </Tag>
                );
              })}
            </Space>
          </div>
        )}
      </div>
    );
  };

  return (
    <Modal
      title="安装运行环境"
      open={visible}
      onCancel={handleClose}
      footer={null}
      destroyOnHidden
    >
      {selectedRuntime === 'php' && (
        <Alert
          message="源码编译提醒"
          description={
            <span>
              该环境需要从源码编译安装。系统将自动为你安装编译所需的系统依赖。<br/>
              <b>注意：</b> 根据服务器性能，编译过程可能需要几分钟到十几分钟不等，请耐心等待。
            </span>
          }
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}
      <Form form={form} onFinish={onInstall} layout="vertical">
        <Form.Item
          name="name"
          label="运行环境"
          rules={[{ required: true, message: '请选择运行环境' }]}
        >
          <Select placeholder="选择运行环境" onChange={(val) => {
            form.setFieldsValue({ version: undefined });
            onRuntimeChange(val);
          }}>
            {catalog.map(c => (
              <Select.Option key={c.lang} value={c.lang}>{c.display}</Select.Option>
            ))}
          </Select>
        </Form.Item>
        <Form.Item
          name="version"
          label={
            <Space>
              <span>版本号</span>
              {selectedRuntime && versionsLoading && <Tag color="blue">加载中...</Tag>}
              {selectedRuntime && !versionsLoading && availableVersions.length > 0 && (
                <Tag color="green">{selectedRuntime} 可用版本 {availableVersions.length} 个</Tag>
              )}
            </Space>
          }
          rules={[{ required: true, message: '请选择版本号' }]}
          extra={renderExtra()}
        >
          {selectedRuntime && versionsLoading ? (
            <Select placeholder="正在获取版本列表..." loading={true} />
          ) : availableVersions.length > 0 ? (
            <Select
              placeholder={`选择 ${selectedRuntime} 版本号`}
              showSearch
              filterOption={(input, option) => {
                const value = option?.value as string;
                return value?.toLowerCase().includes(input.toLowerCase()) ?? false;
              }}
              options={availableVersions.map(v => ({
                label: (
                  <Space>
                    <span>{v.version}</span>
                    {v.installed && <Tag color="green" style={{ fontSize: 10 }}>已安装</Tag>}
                    {v.is_default && <Tag color="blue" style={{ fontSize: 10 }}>默认</Tag>}
                  </Space>
                ),
                value: v.version,
                disabled: v.installed,
              }))}
            />
          ) : selectedRuntime ? (
            <Input placeholder="输入具体版本号，例如 20.11.0" />
          ) : (
            <Input placeholder="请先选择运行环境" disabled />
          )}
        </Form.Item>

        <Form.Item>
          <Button
            type="primary"
            htmlType="submit"
            block
            disabled={versionsLoading || !!majorLoading}
            loading={installing}
          >
            开始安装
          </Button>
        </Form.Item>
      </Form>
    </Modal>
  );
}
