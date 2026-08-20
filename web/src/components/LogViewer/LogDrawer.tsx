import { Drawer, type DrawerProps } from 'antd';
import { LogViewer, type LogViewerProps } from './LogViewer';

export interface LogDrawerProps extends Omit<DrawerProps, 'children'> {
  viewerProps?: LogViewerProps;
  streamUrl?: string;
  streamEnabled?: boolean;
  rawLogs?: string;
  entries?: LogViewerProps['entries'];
  buffer?: LogViewerProps['buffer'];
  status?: LogViewerProps['status'];
  error?: LogViewerProps['error'];
  exitCode?: LogViewerProps['exitCode'];
  elapsedMs?: LogViewerProps['elapsedMs'];
  onDone?: LogViewerProps['onDone'];
  onStreamMessage?: LogViewerProps['onStreamMessage'];
  downloadFileName?: string;
}

export function LogDrawer({
  open,
  title,
  onClose,
  width = 800,
  destroyOnHidden = true,
  viewerProps,
  streamUrl,
  streamEnabled,
  rawLogs,
  entries,
  buffer,
  status,
  error,
  exitCode,
  elapsedMs,
  onDone,
  onStreamMessage,
  downloadFileName,
  ...drawerProps
}: LogDrawerProps) {
  return (
    <Drawer
      open={open}
      title={title}
      onClose={onClose}
      width={width}
      destroyOnHidden={destroyOnHidden}
      styles={{ body: { padding: 0, display: 'flex', flexDirection: 'column' } }}
      {...drawerProps}
    >
      <LogViewer
        streamUrl={streamUrl}
        streamEnabled={streamEnabled ?? open}
        rawLogs={rawLogs}
        entries={entries}
        buffer={buffer}
        status={status}
        error={error}
        exitCode={exitCode}
        elapsedMs={elapsedMs}
        onDone={onDone}
        onStreamMessage={onStreamMessage}
        downloadFileName={downloadFileName}
        style={{ flex: 1, border: 'none', borderRadius: 0, height: '100%' }}
        {...viewerProps}
      />
    </Drawer>
  );
}
