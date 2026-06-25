import { Modal, Input } from 'antd';

interface FileManagerEditorProps {
  visible: boolean;
  path: string;
  content: string;
  onClose: () => void;
  onSave: () => void;
  onContentChange: (content: string) => void;
}

export default function FileManagerEditor({
  visible,
  path,
  content,
  onClose,
  onSave,
  onContentChange,
}: FileManagerEditorProps) {
  return (
    <Modal
      title={`编辑文件: ${path}`}
      open={visible}
      onCancel={onClose}
      onOk={onSave}
      width="80%"
      okText="保存"
      cancelText="取消"
    >
      <Input.TextArea
        value={content}
        onChange={(e) => onContentChange(e.target.value)}
        rows={20}
        style={{ fontFamily: 'monospace' }}
      />
    </Modal>
  );
}
