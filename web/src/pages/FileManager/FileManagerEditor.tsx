import { useState, useMemo } from 'react';
import { Modal, Button, Space } from 'antd';
import { SaveOutlined } from '@ant-design/icons';
import Editor, { loader } from '@monaco-editor/react';
import * as monaco from 'monaco-editor';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

// Use local monaco-editor bundle (no CDN)
// @ts-expect-error type incompatibility in upstream package
loader.config({ monaco });

// Monaco workers: return a no-op worker stub so Monaco doesn't crash.
(self as unknown as { MonacoEnvironment: { getWorker: () => Worker } }).MonacoEnvironment = {
  getWorker: () => new Worker('data:text/javascript;base64,', { name: 'monaco-dummy' }),
};

// detectLanguage maps file extensions to Monaco language IDs.
function detectLanguage(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() || '';
  const map: Record<string, string> = {
    js: 'javascript', jsx: 'javascript', mjs: 'javascript', cjs: 'javascript',
    ts: 'typescript', tsx: 'typescript',
    json: 'json',
    html: 'html', htm: 'html', xml: 'xml', svg: 'xml',
    css: 'css', scss: 'scss', less: 'less',
    py: 'python',
    go: 'go',
    java: 'java',
    c: 'c', h: 'c',
    cpp: 'cpp', cc: 'cpp', hpp: 'cpp',
    rs: 'rust',
    rb: 'ruby',
    php: 'php',
    sh: 'shell', bash: 'shell', zsh: 'shell',
    yml: 'yaml', yaml: 'yaml',
    toml: 'ini', ini: 'ini', conf: 'ini', cfg: 'ini',
    sql: 'sql',
    dart: 'dart',
    bat: 'bat',
    ps1: 'powershell',
    txt: 'plaintext',
    log: 'plaintext',
    env: 'plaintext',
    csv: 'plaintext',
  };
  const basename = path.split('/').pop()?.toLowerCase() || '';
  if (basename === 'dockerfile') return 'dockerfile';
  if (basename === 'makefile') return 'makefile';
  return map[ext] || 'plaintext';
}

function isMarkdown(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase() || '';
  return ext === 'md' || ext === 'markdown';
}

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
  const language = useMemo(() => detectLanguage(path), [path]);
  const isMd = useMemo(() => isMarkdown(path), [path]);
  const [view, setView] = useState<'split' | 'edit' | 'preview'>('split');

  return (
    <Modal
      title={`编辑: ${path.split('/').pop()}`}
      open={visible}
      onCancel={onClose}
      width="90%"
      footer={
        <Space>
          {isMd && (
            <>
              <Button
                type={view === 'split' ? 'primary' : 'default'}
                onClick={() => setView('split')}
              >分屏</Button>
              <Button
                type={view === 'edit' ? 'primary' : 'default'}
                onClick={() => setView('edit')}
              >仅编辑</Button>
              <Button
                type={view === 'preview' ? 'primary' : 'default'}
                onClick={() => setView('preview')}
              >仅预览</Button>
            </>
          )}
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" icon={<SaveOutlined />} onClick={onSave}>保存</Button>
        </Space>
      }
      styles={{ body: { padding: 0 } }}
    >
      {isMd ? (
        // Markdown: split / edit-only / preview-only
        <div style={{ display: 'flex', height: '70vh' }}>
          {view !== 'preview' && (
            <div style={{ flex: 1, borderRight: view === 'split' ? '1px solid #d9d9d9' : 'none', overflow: 'hidden' }}>
              <Editor
                value={content}
                language="markdown"
                theme="vs-dark"
                onChange={(val) => onContentChange(val || '')}
                options={{
                  minimap: { enabled: false },
                  fontSize: 13,
                  fontFamily: 'Consolas, Monaco, "Courier New", monospace',
                  wordWrap: 'on',
                  scrollBeyondLastLine: false,
                  automaticLayout: true,
                }}
              />
            </div>
          )}
          {view !== 'edit' && (
            <div style={{ flex: 1, overflow: 'auto', padding: 16, background: '#fff' }}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {content}
              </ReactMarkdown>
            </div>
          )}
        </div>
      ) : (
        // Non-markdown: Monaco editor with syntax highlighting
        <div style={{ height: '70vh', overflow: 'hidden' }}>
          <Editor
            value={content}
            language={language}
            theme="vs-dark"
            onChange={(val) => onContentChange(val || '')}
            options={{
              minimap: { enabled: false },
              fontSize: 13,
              fontFamily: 'Consolas, Monaco, "Courier New", monospace',
              wordWrap: 'on',
              scrollBeyondLastLine: false,
              automaticLayout: true,
            }}
          />
        </div>
      )}
    </Modal>
  );
}
