import { Modal, type ModalProps } from 'antd';
import { LogViewer, type LogViewerProps } from './LogViewer';

export interface LogModalProps extends Omit<ModalProps, 'children'> {
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
  viewerHeight?: number | string;
  viewerMaxHeight?: number | string;
}

export function LogModal({
  open,
  title,
  onCancel,
  width = 860,
  footer = null,
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
  viewerHeight = 500,
  viewerMaxHeight = '70vh',
  ...modalProps
}: LogModalProps) {
  return (
    <Modal
      open={open}
      title={title}
      onCancel={onCancel}
      width={width}
      footer={footer}
      destroyOnHidden={destroyOnHidden}
      styles={{ body: { padding: 0 } }}
      {...modalProps}
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
        height={viewerHeight}
        maxHeight={viewerMaxHeight}
        style={{ border: 'none', borderRadius: 0 }}
        {...viewerProps}
      />
    </Modal>
  );
}
