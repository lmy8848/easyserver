import React, { useState } from 'react';
import { Modal, Form, Select, Input, Button, Space, Tag, message } from 'antd';
import { SyncOutlined, LoadingOutlined } from '@ant-design/icons';
import type { VersionInfo, Dependencies } from './types';

interface VersionListProps {
  visible: boolean;
  onClose: () => void;
  selectedRuntime: string;
  versionsLoading: boolean;
  availableVersions: VersionInfo[];
  aliasSuggestions: string[];
  dependencies: Dependencies | null;
  depsLoading: boolean;
  onInstall: (values: { name: string; version: string }) => void;
  onRuntimeChange: (value: string) => void;
  onRefreshVersions: (runtimeName: string, forceRefresh?: boolean) => void;
  onAliasClick: (alias: string) => Promise<string | void> | void;
}

export default function VersionList({
  visible,
  onClose,
  selectedRuntime,
  versionsLoading,
  availableVersions,
  aliasSuggestions,
  dependencies,
  depsLoading,
  onInstall,
  onRuntimeChange,
  onRefreshVersions,
  onAliasClick,
}: VersionListProps) {
  const [form] = Form.useForm();
  const [aliasLoading, setAliasLoading] = useState<string | null>(null);

  const handleClose = () => {
    onClose();
    form.resetFields();
  };

  const handleAliasClickWrapper = async (alias: string) => {
    if (aliasLoading) return;
    setAliasLoading(alias);
    const currentRuntime = form.getFieldValue('name');
    try {
      const resolved = await onAliasClick(alias);
      if (form.getFieldValue('name') !== currentRuntime) return;
      if (resolved) {
        const targetVersion = availableVersions.find(v => v.version === resolved);
        if (targetVersion?.installed) {
          message.warning(`版本 ${resolved} 已安装`);
          return;
        }
        form.setFields([
          {
            name: 'version',
            value: resolved,
            errors: [],
          }
        ]);
        form.validateFields(['version']).catch(() => {});
      }
    } catch (error) {
      message.error('获取版本失败');
    } finally {
      setAliasLoading(null);
    }
  };

  const renderExtra = () => {
    if (!selectedRuntime) return null;
    if (versionsLoading) return null;

    return (
      <div style={{ marginTop: 8 }}>
        <Button
          type="link"
          size="small"
          icon={<SyncOutlined />}
          onClick={() => onRefreshVersions(selectedRuntime, true)}
          loading={versionsLoading}
          style={{ padding: '0', marginBottom: 8 }}
        >
          {availableVersions.length > 0 ? '刷新版本列表' : '从网络获取版本列表'}
        </Button>
        {aliasSuggestions.length > 0 && (
          <div>
            <span style={{ fontSize: 12, color: '#999' }}>快速选择: </span>
            <Space size={4} wrap>
              {aliasSuggestions.map(alias => (
                <Tag
                  key={alias}
                  color="blue"
                  style={{ cursor: aliasLoading ? 'not-allowed' : 'pointer', opacity: aliasLoading && aliasLoading !== alias ? 0.5 : 1 }}
                  icon={aliasLoading === alias ? <LoadingOutlined /> : null}
                  onClick={() => handleAliasClickWrapper(alias)}
                >
                  {alias}
                </Tag>
              ))}
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
    >
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
            <Select.Option value="java">Java</Select.Option>
            <Select.Option value="node">Node.js</Select.Option>
            <Select.Option value="go">Go</Select.Option>
            <Select.Option value="python">Python</Select.Option>
            <Select.Option value="php">PHP</Select.Option>
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
                    {v.lts && <Tag color="orange" style={{ fontSize: 10 }}>LTS</Tag>}
                  </Space>
                ),
                value: v.version,
                disabled: v.installed,
              }))}
            />
          ) : selectedRuntime ? (
            <Input placeholder="输入版本号或别名，例如：17、lts、latest" />
          ) : (
            <Input placeholder="请先选择运行环境" disabled />
          )}
        </Form.Item>

        {/* 依赖检测结果 */}
        {selectedRuntime && (
          <Form.Item label="依赖检测">
            {depsLoading ? (
              <Space>
                <SyncOutlined spin />
                <span>正在检测依赖...</span>
              </Space>
            ) : dependencies ? (
              <div>
                {dependencies.missing.length === 0 ? (
                  <Tag color="green">所有必需依赖已满足</Tag>
                ) : (
                  <div>
                    <Tag color="red">缺少必需依赖</Tag>
                    <div style={{ marginTop: 8 }}>
                      <span style={{ color: '#ff4d4f' }}>缺失: </span>
                      {dependencies.missing.map(dep => (
                        <Tag key={dep} color="red" style={{ marginBottom: 4 }}>{dep}</Tag>
                      ))}
                    </div>
                    <div style={{ marginTop: 4, color: '#999', fontSize: 12 }}>
                      请先安装缺失的依赖后再安装运行环境
                    </div>
                  </div>
                )}
                {dependencies.installed.length > 0 && (
                  <div style={{ marginTop: 4 }}>
                    <span style={{ color: '#52c41a' }}>已安装: </span>
                    {dependencies.installed.map(dep => (
                      <Tag key={dep} color="green" style={{ marginBottom: 4 }}>{dep}</Tag>
                    ))}
                  </div>
                )}
                {dependencies.optional && dependencies.optional.length > 0 && (
                  <div style={{ marginTop: 4 }}>
                    <span style={{ color: '#faad14' }}>可选: </span>
                    {dependencies.optional.map(dep => (
                      <Tag key={dep} color="warning" style={{ marginBottom: 4 }}>{dep}</Tag>
                    ))}
                  </div>
                )}
              </div>
            ) : (
              <span style={{ color: '#999' }}>选择运行环境后自动检测</span>
            )}
          </Form.Item>
        )}

        <Form.Item>
          <Button
            type="primary"
            htmlType="submit"
            block
            disabled={depsLoading || versionsLoading || !!aliasLoading || (dependencies ? dependencies.missing.length > 0 : false)}
          >
            开始安装
          </Button>
        </Form.Item>
      </Form>
    </Modal>
  );
}
