import { Modal, Button, Card, Space, Tag, Typography } from 'antd';
import type { DetectedRuntime } from './types';
import { getRuntimeIcon } from './types';

const { Title } = Typography;

interface DetectPanelProps {
  visible: boolean;
  detectedRuntimes: DetectedRuntime[];
  onClose: () => void;
  onImport: () => void;
}

export default function DetectPanel({
  visible,
  detectedRuntimes,
  onClose,
  onImport,
}: DetectPanelProps) {
  return (
    <Modal
      title="系统已安装的运行环境"
      open={visible}
      onCancel={onClose}
      footer={[
        <Button key="cancel" onClick={onClose}>
          关闭
        </Button>,
        <Button key="import" type="primary" onClick={onImport}>
          导入到管理列表
        </Button>,
      ]}
    >
      {detectedRuntimes.length === 0 ? (
        <p>未检测到已安装的运行环境</p>
      ) : (
        <div>
          {detectedRuntimes.map((runtime) => (
            <Card key={runtime.name} size="small" style={{ marginBottom: 16 }}>
              <Title level={5}>
                {getRuntimeIcon(runtime.name)} {runtime.name}
              </Title>
              <Space>
                {runtime.versions.map((version) => (
                  <Tag key={version}>{version}</Tag>
                ))}
              </Space>
            </Card>
          ))}
        </div>
      )}
    </Modal>
  );
}
