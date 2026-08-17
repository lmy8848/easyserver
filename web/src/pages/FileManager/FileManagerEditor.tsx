import { useState, useMemo, useEffect } from 'react';
import { Modal, Button, Space, Tooltip } from 'antd';
import {
  SaveOutlined,
  FullscreenOutlined,
  FullscreenExitOutlined,
  ZoomInOutlined,
  ZoomOutOutlined,
} from '@ant-design/icons';
import CodeMirror from '@uiw/react-codemirror';
import { oneDark } from '@codemirror/theme-one-dark';
import { EditorView, keymap } from '@codemirror/view';
import type { Extension } from '@codemirror/state';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { loadLanguageExtension, customEditorTheme } from './cmConfig';

const FONT_SIZE_STORAGE_KEY = 'easyserver_editor_font_size';
const MIN_FONT_SIZE = 10;
const MAX_FONT_SIZE = 28;
const DEFAULT_FONT_SIZE = 16;

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
  const isMd = useMemo(() => isMarkdown(path), [path]);
  const [view, setView] = useState<'split' | 'edit' | 'preview'>('split');
  const [langExts, setLangExts] = useState<Extension[]>([]);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [fontSize, setFontSize] = useState<number>(() => {
    const saved = localStorage.getItem(FONT_SIZE_STORAGE_KEY);
    return saved ? parseInt(saved, 10) || DEFAULT_FONT_SIZE : DEFAULT_FONT_SIZE;
  });

  useEffect(() => {
    let active = true;
    loadLanguageExtension(path).then((exts) => {
      if (active) {
        setLangExts(exts);
      }
    });
    return () => {
      active = false;
    };
  }, [path]);

  const changeFontSize = (delta: number) => {
    const next = Math.max(MIN_FONT_SIZE, Math.min(MAX_FONT_SIZE, fontSize + delta));
    setFontSize(next);
    localStorage.setItem(FONT_SIZE_STORAGE_KEY, String(next));
  };

  const extensions = useMemo(() => {
    const saveBinding = keymap.of([
      {
        key: 'Mod-s',
        run: () => {
          onSave();
          return true;
        },
      },
    ]);

    const dynamicFontTheme = EditorView.theme({
      '&': {
        fontSize: `${fontSize}px !important`,
      },
      '.cm-scroller': {
        fontSize: `${fontSize}px !important`,
      },
      '.cm-content': {
        fontSize: `${fontSize}px !important`,
      },
      '.cm-gutters': {
        fontSize: `${fontSize}px !important`,
      },
      '.cm-line': {
        fontSize: `${fontSize}px !important`,
      },
    });

    return [
      ...langExts,
      EditorView.lineWrapping,
      saveBinding,
      customEditorTheme,
      dynamicFontTheme,
    ];
  }, [langExts, onSave, fontSize]);

  const editorHeight = isFullscreen ? 'calc(100vh - 110px)' : '70vh';

  return (
    <Modal
      title={
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingRight: 32 }}>
          <span style={{ fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            编辑: {path.split('/').pop() || path}
          </span>
          <Space size="middle" align="center">
            <Space size={6} align="center" style={{ display: 'inline-flex', alignItems: 'center' }}>
              <Tooltip title="缩小字号">
                <Button
                  icon={<ZoomOutOutlined />}
                  onClick={() => changeFontSize(-1)}
                  disabled={fontSize <= MIN_FONT_SIZE}
                />
              </Tooltip>
              <span
                style={{
                  fontSize: 14,
                  fontWeight: 600,
                  color: 'inherit',
                  minWidth: 42,
                  textAlign: 'center',
                  userSelect: 'none',
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  lineHeight: '32px',
                }}
              >
                {fontSize}px
              </span>
              <Tooltip title="放大字号">
                <Button
                  icon={<ZoomInOutlined />}
                  onClick={() => changeFontSize(1)}
                  disabled={fontSize >= MAX_FONT_SIZE}
                />
              </Tooltip>
            </Space>
            <Tooltip title={isFullscreen ? '退出全屏' : '页面全屏'}>
              <Button
                icon={isFullscreen ? <FullscreenExitOutlined /> : <FullscreenOutlined />}
                onClick={() => setIsFullscreen(!isFullscreen)}
              >
                {isFullscreen ? '退出全屏' : '全屏'}
              </Button>
            </Tooltip>
          </Space>
        </div>
      }
      open={visible}
      onCancel={onClose}
      width={isFullscreen ? '100vw' : '90%'}
      style={isFullscreen ? { top: 0, margin: 0, paddingBottom: 0, maxWidth: '100vw' } : undefined}
      styles={{
        body: {
          padding: 0,
          height: editorHeight,
          overflow: 'hidden',
        },
      }}
      footer={
        <Space>
          {isMd && (
            <>
              <Button
                type={view === 'split' ? 'primary' : 'default'}
                onClick={() => setView('split')}
              >
                分屏
              </Button>
              <Button
                type={view === 'edit' ? 'primary' : 'default'}
                onClick={() => setView('edit')}
              >
                仅编辑
              </Button>
              <Button
                type={view === 'preview' ? 'primary' : 'default'}
                onClick={() => setView('preview')}
              >
                仅预览
              </Button>
            </>
          )}
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" icon={<SaveOutlined />} onClick={onSave}>
            保存 (Ctrl+S)
          </Button>
        </Space>
      }
    >
      {isMd ? (
        // Markdown: split / edit-only / preview-only
        <div style={{ display: 'flex', height: editorHeight }}>
          {view !== 'preview' && (
            <div style={{ flex: 1, borderRight: view === 'split' ? '1px solid #303030' : 'none', overflow: 'hidden' }}>
              <CodeMirror
                value={content}
                height={editorHeight}
                theme={oneDark}
                indentWithTab={true}
                extensions={extensions}
                onChange={(val) => onContentChange(val)}
                style={{ height: '100%' }}
              />
            </div>
          )}
          {view !== 'edit' && (
            <div style={{ flex: 1, overflow: 'auto', padding: 16, background: '#1e1e1e', color: '#e0e0e0', fontSize: `${fontSize}px` }}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {content}
              </ReactMarkdown>
            </div>
          )}
        </div>
      ) : (
        // Non-markdown: CodeMirror with dark theme & on-demand dynamic syntax highlighting
        <div style={{ height: editorHeight, overflow: 'hidden' }}>
          <CodeMirror
            value={content}
            height={editorHeight}
            theme={oneDark}
            indentWithTab={true}
            extensions={extensions}
            onChange={(val) => onContentChange(val)}
            style={{ height: '100%' }}
          />
        </div>
      )}
    </Modal>
  );
}
