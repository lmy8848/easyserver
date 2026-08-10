import { useState } from 'react';
import {
  Modal, Form, Select, InputNumber, Input, List, Tag, Space, Spin, Pagination, Empty, message,
} from 'antd';
import { dbServerApi } from '../../services/api';
import type { EngineInfo, VersionTemplate } from './types';

// Sentinel option value — picking "更多版本" opens the Docker Hub pager modal
// instead of selecting a version.
const MORE_VERSIONS = '__more_versions__';

interface InstallModalProps {
  server: EngineInfo;
  visible: boolean;
  onVisibleChange: (visible: boolean) => void;
  versionTemplates: VersionTemplate[];
  form: any;
  busy: string;
  onInstall: () => void;
  portCheck: { available: boolean; message: string; process?: string } | null;
  onCheckPort: (port: number) => void;
}

// Install form + embedded Docker Hub tag pager (opened from the "更多版本"
// Select option). image is resolved from presets or the picker, never typed.
export default function InstallModal({
  server, visible, onVisibleChange, versionTemplates, form, busy, onInstall, portCheck, onCheckPort,
}: InstallModalProps) {
  const DOCKER_PAGE_SIZE = 10;
  const [dockerVisible, setDockerVisible] = useState(false);
  const [dockerTags, setDockerTags] = useState<string[]>([]);
  const [dockerTotal, setDockerTotal] = useState(0);
  const [dockerPage, setDockerPage] = useState(1);
  const [dockerLoading, setDockerLoading] = useState(false);

  const openDockerTags = async () => {
    setDockerVisible(true);
    setDockerLoading(true);
    try {
      const res = await dbServerApi.listDockerTags(server.db_type, 1, DOCKER_PAGE_SIZE);
      setDockerTags(res.data?.data?.items || []);
      setDockerTotal(res.data?.data?.total || 0);
      setDockerPage(1);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '查询 Docker Hub 失败');
      setDockerTags([]);
      setDockerTotal(0);
    } finally { setDockerLoading(false); }
  };

  const fetchDockerPage = async (page: number) => {
    setDockerLoading(true);
    try {
      const res = await dbServerApi.listDockerTags(server.db_type, page, DOCKER_PAGE_SIZE);
      setDockerTags(res.data?.data?.items || []);
      setDockerPage(res.data?.data?.page || page);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '查询 Docker Hub 失败');
    } finally { setDockerLoading(false); }
  };

  // Picking a published tag fills version + image into the install form; the
  // image is the fully-qualified `docker.io/<base_image>:<tag>` so the backend
  // never resolves (or has to resolve) short names.
  const pickDockerTag = (tag: string) => {
    form.setFieldsValue({ version: tag, image: `docker.io/${server.base_image}:${tag}` });
    setDockerVisible(false);
  };

  return (
    <>
      <Modal title={`安装${server.display_name}`} open={visible} onCancel={() => onVisibleChange(false)}
        onOk={onInstall} okText="安装" cancelText="取消" confirmLoading={busy === 'install-version'}>
        <Form form={form} layout="vertical">
          {/* image is resolved from the preset catalogue or the Docker Hub
              picker; hidden because users never type image names. */}
          <Form.Item name="image" hidden><Input /></Form.Item>
          <Form.Item name="version" label="选择版本" rules={[{ required: true, message: '请选择版本' }]}>
            <Select
              placeholder="选择要安装的版本"
              onChange={(v) => {
                // "更多版本" is a sentinel: open the Docker Hub pager instead of
                // treating it as a version, and clear the selection (empty string
                // triggers the required rule if the user closes the pager without
                // picking anything).
                if (v === MORE_VERSIONS) {
                  form.setFieldValue('version', '');
                  openDockerTags();
                  return;
                }
                const t = versionTemplates.find(t => t.version === v);
                form.setFieldsValue({ image: t ? t.image : `docker.io/${server.base_image}:${v}` });
              }}
            >
              {versionTemplates.map(t => (
                <Select.Option key={t.version} value={t.version}>
                  <strong>{t.version}</strong><span style={{ color: '#999', marginLeft: 8, fontSize: 12 }}>{t.description}</span>
                </Select.Option>
              ))}
              <Select.Option value={MORE_VERSIONS}>
                <span style={{ color: '#1677ff' }}>＋ 更多版本（查询 Docker Hub）</span>
              </Select.Option>
            </Select>
          </Form.Item>
          <Form.Item name="container_engine" label="容器引擎" initialValue="podman">
            <Select options={[{ value: 'docker', label: 'Docker' }, { value: 'podman', label: 'Podman（rootful）' }]} />
          </Form.Item>
          <Form.Item name="bind_address" label="监听地址" initialValue="127.0.0.1">
            <Select options={[
              { value: '127.0.0.1', label: '仅本机（127.0.0.1）' },
              { value: '0.0.0.0', label: '所有网卡（0.0.0.0）' },
            ]} />
          </Form.Item>
          <Form.Item name="port" label="端口（留空使用默认）"
            extra={portCheck && (
              portCheck.available
                ? <span style={{ color: '#52c41a' }}>{portCheck.message}</span>
                : <span style={{ color: '#ff4d4f' }}>{portCheck.message}{portCheck.process && ` (${portCheck.process})`}</span>
            )}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }}
              onChange={(val) => val && onCheckPort(val as number)} />
          </Form.Item>
        </Form>
      </Modal>

      {/* Docker Hub tag pager (opened from the "更多版本" Select option) */}
      <Modal
        title={`${server.display_name} - 更多版本（Docker Hub）`}
        open={dockerVisible}
        onCancel={() => setDockerVisible(false)}
        footer={null}
      >
        {dockerLoading ? (
          <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div>
        ) : dockerTags.length === 0 ? (
          <Empty description="未获取到版本列表" />
        ) : (
          <>
            <List
              size="small"
              bordered
              dataSource={dockerTags}
              renderItem={(tag) => (
                <List.Item onClick={() => pickDockerTag(tag)} style={{ cursor: 'pointer' }}>
                  <Space>
                    <Tag>docker.io/{server.base_image}:{tag}</Tag>
                    <span style={{ color: '#999', fontSize: 12 }}>点击选择此版本</span>
                  </Space>
                </List.Item>
              )}
            />
            <div style={{ textAlign: 'center', marginTop: 12 }}>
              <Pagination
                align="center"
                current={dockerPage}
                pageSize={DOCKER_PAGE_SIZE}
                total={dockerTotal}
                showSizeChanger={false}
                showQuickJumper={false}
                onChange={fetchDockerPage}
              />
            </div>
          </>
        )}
      </Modal>
    </>
  );
}
