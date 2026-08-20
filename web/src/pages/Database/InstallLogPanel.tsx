import { Card } from 'antd';
import { FileTextOutlined } from '@ant-design/icons';
import { LogViewer } from '../../components/LogViewer';

// Inline install log for an installing/failed instance.
// Self-contained: owns the SSE stream and follow-scroll via LogViewer.
// The log area is capped so the whole panel stays within one viewport below the header card.
export default function InstallLogPanel({
  instanceId,
  version,
  onDone,
}: {
  instanceId: number;
  version: string;
  onDone?: () => void; // install finished (success / failure / cancel)
}) {
  return (
    <Card
      styles={{
        body: {
          padding: 0,
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          flex: 1,
          minHeight: 0,
        },
      }}
      style={{
        overflow: 'hidden',
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
      }}
    >
      <LogViewer
        title={
          <span>
            <FileTextOutlined style={{ marginRight: 6 }} />
            {version} 安装日志
          </span>
        }
        streamUrl={`/api/db/installs/${instanceId}/log`}
        onDone={() => onDone?.()}
        height="100%"
        downloadFileName={`db_instance_${instanceId}_install`}
        style={{ border: 'none', borderRadius: 0, flex: 1, minHeight: 0 }}
      />
    </Card>
  );
}
