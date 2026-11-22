import { useState } from 'react';
import { Alert, Button, Card, Descriptions, Modal, Space, Typography, Upload, message } from 'antd';
import { CloudDownloadOutlined, DownloadOutlined, UploadOutlined } from '@ant-design/icons';
import api from '../../services/client';

interface InspectInfo {
  manifest?: { version?: string; created_at?: string; includes?: string[] };
  entries?: { name: string; size: number }[];
  total_size?: number;
  warnings?: string[];
}

// 面板数据备份：导出（配置+数据库，可选站点/插件）与备份包预检。
export default function BackupCard() {
  const [inspecting, setInspecting] = useState(false);
  const [downloading, setDownloading] = useState(false);
  const [info, setInfo] = useState<InspectInfo | null>(null);

  const handleInspect = async (file: File) => {
    setInspecting(true);
    try {
      const form = new FormData();
      form.append('file', file);
      const res = await api.post('/backup/inspect', form);
      setInfo((res.data?.data as InspectInfo) || null);
    } catch (error: unknown) {
      message.error(error instanceof Error ? error.message : '备份包预检失败');
    } finally {
      setInspecting(false);
    }
  };

  // 走 axios 下载而不是 window.location：401/超时能拿到错误信息，
  // 不会把整个页面跳到一个裸 JSON 错误页。
  const handleExport = async (include: string, label: string) => {
    Modal.confirm({
      title: `导出${label}？`,
      content: '导出包含面板配置、密钥与数据库，属于敏感数据，请确认保存位置与访问权限。',
      okText: '开始导出',
      onOk: async () => {
        setDownloading(true);
        try {
          const res = await api.get('/backup/export', {
            params: { include },
            responseType: 'blob',
            timeout: 600000,
          });
          const warnings = (res.headers['x-backup-warnings'] as string | undefined) || '';
          const disposition = (res.headers['content-disposition'] as string | undefined) || '';
          const match = /filename\*=UTF-8''([^;]+)/.exec(disposition);
          const filename = match?.[1] ? decodeURIComponent(match[1]) : 'easyserver-backup.tar.gz';
          const url = URL.createObjectURL(res.data as Blob);
          const a = document.createElement('a');
          a.href = url;
          a.download = filename;
          a.click();
          URL.revokeObjectURL(url);
          if (warnings) {
            message.warning(`备份已下载，但有缺项：${warnings}`);
          } else {
            message.success('备份已开始下载');
          }
        } catch (error: unknown) {
          message.error(error instanceof Error ? error.message : '导出失败');
        } finally {
          setDownloading(false);
        }
      },
    });
  };

  return (
    <Card title="数据备份" style={{ marginTop: 16 }}>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
        导出面板自身数据用于换机迁移/灾难恢复：配置（含密钥）+ SQLite 一致性快照，
        可选包含站点文件与插件目录。导出包含敏感凭据，请妥善保管。
      </Typography.Paragraph>
      <Space wrap>
        <Button
          icon={<DownloadOutlined />}
          loading={downloading}
          onClick={() => handleExport('config,db', '配置与数据库')}
        >
          导出（配置 + 数据库）
        </Button>
        <Button
          icon={<DownloadOutlined />}
          loading={downloading}
          onClick={() => handleExport('config,db,sites,plugins', '完整备份（含站点/插件）')}
        >
          导出完整备份（含站点/插件）
        </Button>
        <Upload
          accept=".gz,.tar.gz"
          showUploadList={false}
          beforeUpload={(file) => {
            void handleInspect(file as File);
            return false;
          }}
        >
          <Button icon={<UploadOutlined />} loading={inspecting}>
            预检备份包
          </Button>
        </Upload>
      </Space>

      <Modal
        title="备份包预检"
        open={!!info}
        onCancel={() => setInfo(null)}
        footer={<Button onClick={() => setInfo(null)}>关闭</Button>}
      >
        {info && (
          <Space direction="vertical" style={{ width: '100%' }}>
            <Descriptions size="small" column={1} bordered>
              <Descriptions.Item label="版本">{info.manifest?.version || '-'}</Descriptions.Item>
              <Descriptions.Item label="创建时间">{info.manifest?.created_at || '-'}</Descriptions.Item>
              <Descriptions.Item label="包含项">{(info.manifest?.includes || []).join(', ')}</Descriptions.Item>
              <Descriptions.Item label="总大小">{Math.round((info.total_size || 0) / 1024)} KB</Descriptions.Item>
            </Descriptions>
            {(info.warnings || []).map((w) => (
              <Alert key={w} type="warning" showIcon message={w} />
            ))}
            <div style={{ maxHeight: 240, overflow: 'auto', fontSize: 12, fontFamily: 'monospace' }}>
              {(info.entries || []).map((e) => (
                <div key={e.name}>{e.name}  {Math.round(e.size / 1024)} KB</div>
              ))}
            </div>
          </Space>
        )}
      </Modal>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginTop: 8, marginBottom: 0 }}>
        <CloudDownloadOutlined /> 导入/恢复功能将先做预检与二次确认，再替换数据并自动回滚。
      </Typography.Paragraph>
    </Card>
  );
}
