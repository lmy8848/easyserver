import { Card } from 'antd';
import { FileTextOutlined } from '@ant-design/icons';
import { LogViewer } from '../../components/LogViewer';

// Inline install log for an installing/failed instance.
// Self-contained: owns the SSE stream and follow-scroll via LogViewer.
// The log area is capped so the whole panel stays within one viewport below the header card.
export default function InstallLogPanel({
  containerName,
  version,
  onDone,
}: {
  containerName: string;
  version: string;
  onDone?: () => void; // install finished (success / failure / cancel)
}) {
  return (
    <Card styles={{ body: { padding: 0 } }} style={{ overflow: 'hidden' }}>
      <LogViewer
        title={
          <span>
            <FileTextOutlined style={{ marginRight: 6 }} />
            {version} 安装日志
          </span>
        }
        streamUrl={`/api/db/installs/${containerName}/log`}
        onDone={() => onDone?.()}
        maxHeight="calc(100vh - 220px)"
        height="calc(100vh - 220px)"
        downloadFileName={`db_${containerName}_install`}
        style={{ border: 'none', borderRadius: 0 }}
      />
    </Card>
  );
}
