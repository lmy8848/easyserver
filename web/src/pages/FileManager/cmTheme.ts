import { EditorView } from '@codemirror/view';

/**
 * 自定义 CodeMirror 6 控件主题，包含 Ctrl+F 搜索/替换面板及正文高亮样式，
 * 与 EasyServer 深色暗夜风格保持一致。
 */
export const customEditorTheme = EditorView.theme({
  '&': {
    height: '100%',
  },
  '.cm-scroller': {
    fontFamily: 'Consolas, Monaco, "Courier New", monospace',
  },
  // 1. 搜索面板外层容器：深色背景 + 边框 + flex 布局
  '.cm-panel.cm-search': {
    backgroundColor: '#1f1f1f',
    color: '#d9d9d9',
    padding: '8px 14px',
    borderBottom: '1px solid #303030',
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    flexWrap: 'wrap',
    fontSize: '12px',
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
  },
  // 2. 搜索/替换输入框：深色背景 + 圆角 + 聚焦高亮
  '.cm-panel.cm-search .cm-textfield': {
    backgroundColor: '#141414',
    border: '1px solid #434343',
    borderRadius: '4px',
    color: '#ffffff',
    padding: '4px 8px',
    fontSize: '12px',
    outline: 'none',
    transition: 'border-color 0.2s, box-shadow 0.2s',
  },
  '.cm-panel.cm-search .cm-textfield:focus': {
    borderColor: '#1677ff',
    boxShadow: '0 0 0 2px rgba(22, 119, 255, 0.2)',
  },
  // 3. 搜索操作按钮（上一个、下一个、替换等）
  '.cm-panel.cm-search .cm-button': {
    backgroundImage: 'none',
    backgroundColor: '#262626',
    border: '1px solid #434343',
    borderRadius: '4px',
    color: '#d9d9d9',
    padding: '3px 10px',
    fontSize: '12px',
    cursor: 'pointer',
    lineHeight: '1.5',
    transition: 'all 0.2s',
  },
  '.cm-panel.cm-search .cm-button:hover': {
    backgroundColor: '#303030',
    borderColor: '#1677ff',
    color: '#1677ff',
  },
  '.cm-panel.cm-search .cm-button:active': {
    backgroundColor: '#1677ff',
    color: '#ffffff',
  },
  // 4. 选项复选框标签（大小写匹配、正则、全词等）
  '.cm-panel.cm-search label': {
    fontSize: '12px',
    color: '#8c8c8c',
    cursor: 'pointer',
    display: 'inline-flex',
    alignItems: 'center',
    gap: '4px',
    userSelect: 'none',
  },
  '.cm-panel.cm-search label:hover': {
    color: '#ffffff',
  },
  '.cm-panel.cm-search input[type="checkbox"]': {
    accentColor: '#1677ff',
    cursor: 'pointer',
  },
  // 5. 搜索栏关闭按钮
  '.cm-panel.cm-search button[name="close"]': {
    marginLeft: 'auto',
    cursor: 'pointer',
    backgroundColor: 'transparent',
    border: 'none',
    color: '#8c8c8c',
    fontSize: '16px',
    padding: '0 6px',
    lineHeight: '1',
    borderRadius: '4px',
  },
  '.cm-panel.cm-search button[name="close"]:hover': {
    backgroundColor: '#303030',
    color: '#ff4d4f',
    borderColor: 'transparent',
  },
  // 6. 正文中匹配项的高亮样式
  '.cm-searchMatch': {
    backgroundColor: 'rgba(250, 173, 20, 0.35)',
    borderRadius: '2px',
  },
  '.cm-searchMatch-selected': {
    backgroundColor: 'rgba(250, 173, 20, 0.85)',
    color: '#000000 !important',
    borderRadius: '2px',
    fontWeight: 'bold',
  },
});
