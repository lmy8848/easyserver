import { languages } from '@codemirror/language-data';
import { LanguageDescription } from '@codemirror/language';
import { EditorView } from '@codemirror/view';
import type { Extension } from '@codemirror/state';

/**
 * 利用 CodeMirror 官方 @codemirror/language-data 库，
 * 根据文件名自动匹配并异步动态加载对应语言包（支持 100+ 种语言）。
 */
export async function loadLanguageExtension(path: string): Promise<Extension[]> {
  const filename = path.split('/').pop() || path;
  const desc = LanguageDescription.matchFilename(languages, filename);
  if (!desc) {
    return [];
  }
  try {
    const langSupport = await desc.load();
    return [langSupport.extension];
  } catch (err) {
    console.warn(`Failed to dynamically load language for ${path}:`, err);
    return [];
  }
}

/**
 * 自定义 CodeMirror 6 控件主题，包含 Ctrl+F 搜索/替换面板及正文高亮样式，
 * 采用精确 CSS Grid 双行布局：第 1 行搜索，第 2 行替换（严格置于搜索框正下方）。
 */
export const customEditorTheme = EditorView.theme({
  '&': {
    height: '100%',
  },
  '.cm-scroller': {
    fontFamily: 'Consolas, Monaco, "Courier New", monospace',
  },
  // 1. 搜索面板外层容器：CSS Grid 双行结构
  '.cm-search, .cm-panel.cm-search': {
    position: 'relative',
    display: 'grid !important',
    gridTemplateColumns: '240px auto auto auto auto auto auto 1fr',
    gridTemplateRows: 'auto auto',
    rowGap: '8px !important',
    columnGap: '6px !important',
    alignItems: 'center !important',
    padding: '10px 48px 10px 14px !important',
    backgroundColor: '#1f1f1f !important',
    color: '#d9d9d9 !important',
    borderBottom: '1px solid #303030 !important',
    fontSize: '12px',
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
  },
  // 隐藏原生 <br>
  '.cm-search br, .cm-panel.cm-search br': {
    display: 'none !important',
  },
  // --- 第一行：搜索行 ---
  '.cm-search input[name="search"], .cm-panel.cm-search input[name="search"]': {
    gridRow: '1 !important',
    gridColumn: '1 !important',
    width: '240px !important',
  },
  '.cm-search button[name="next"], .cm-panel.cm-search button[name="next"]': {
    gridRow: '1 !important',
    gridColumn: '2 !important',
  },
  '.cm-search button[name="prev"], .cm-panel.cm-search button[name="prev"]': {
    gridRow: '1 !important',
    gridColumn: '3 !important',
  },
  '.cm-search button[name="select"], .cm-panel.cm-search button[name="select"]': {
    gridRow: '1 !important',
    gridColumn: '4 !important',
  },
  '.cm-search label:nth-of-type(1), .cm-panel.cm-search label:nth-of-type(1)': {
    gridRow: '1 !important',
    gridColumn: '5 !important',
  },
  '.cm-search label:nth-of-type(2), .cm-panel.cm-search label:nth-of-type(2)': {
    gridRow: '1 !important',
    gridColumn: '6 !important',
  },
  '.cm-search label:nth-of-type(3), .cm-panel.cm-search label:nth-of-type(3)': {
    gridRow: '1 !important',
    gridColumn: '7 !important',
  },

  // --- 第二行：替换行（严格位于搜索框正下方） ---
  '.cm-search input[name="replace"], .cm-panel.cm-search input[name="replace"]': {
    gridRow: '2 !important',
    gridColumn: '1 !important',
    width: '240px !important',
  },
  '.cm-search button[name="replace"], .cm-panel.cm-search button[name="replace"]': {
    gridRow: '2 !important',
    gridColumn: '2 !important',
  },
  '.cm-search button[name="replaceAll"], .cm-panel.cm-search button[name="replaceAll"]': {
    gridRow: '2 !important',
    gridColumn: '3 !important',
  },

  // 输入框通用样式
  '.cm-search .cm-textfield, .cm-panel.cm-search .cm-textfield': {
    backgroundColor: '#141414 !important',
    border: '1px solid #434343 !important',
    borderRadius: '4px !important',
    color: '#ffffff !important',
    padding: '4px 8px !important',
    fontSize: '12px !important',
    outline: 'none !important',
    boxSizing: 'border-box !important',
    transition: 'border-color 0.2s, box-shadow 0.2s',
  },
  '.cm-search .cm-textfield:focus, .cm-panel.cm-search .cm-textfield:focus': {
    borderColor: '#1677ff !important',
    boxShadow: '0 0 0 2px rgba(22, 119, 255, 0.2) !important',
  },

  // 按钮通用样式
  '.cm-search .cm-button, .cm-panel.cm-search .cm-button': {
    backgroundImage: 'none !important',
    backgroundColor: '#262626 !important',
    border: '1px solid #434343 !important',
    borderRadius: '4px !important',
    color: '#d9d9d9 !important',
    padding: '3px 10px !important',
    fontSize: '12px !important',
    cursor: 'pointer !important',
    lineHeight: '1.5 !important',
    margin: '0 !important',
    whiteSpace: 'nowrap !important',
    transition: 'all 0.2s',
  },
  '.cm-search .cm-button:hover, .cm-panel.cm-search .cm-button:hover': {
    backgroundColor: '#303030 !important',
    borderColor: '#1677ff !important',
    color: '#1677ff !important',
  },
  '.cm-search .cm-button:active, .cm-panel.cm-search .cm-button:active': {
    backgroundColor: '#1677ff !important',
    color: '#ffffff !important',
  },

  // 选项复选框标签
  '.cm-search label, .cm-panel.cm-search label': {
    fontSize: '12px !important',
    color: '#8c8c8c !important',
    cursor: 'pointer',
    display: 'inline-flex !important',
    alignItems: 'center !important',
    gap: '4px !important',
    margin: '0 !important',
    whiteSpace: 'nowrap !important',
    userSelect: 'none',
  },
  '.cm-search label:hover, .cm-panel.cm-search label:hover': {
    color: '#ffffff !important',
  },
  '.cm-search input[type="checkbox"], .cm-panel.cm-search input[type="checkbox"]': {
    accentColor: '#1677ff',
    cursor: 'pointer',
    margin: '0 !important',
  },

  // 关闭按钮
  '.cm-search button[name="close"], .cm-panel.cm-search button[name="close"]': {
    position: 'absolute !important',
    right: '12px !important',
    top: '10px !important',
    cursor: 'pointer',
    backgroundColor: 'transparent !important',
    border: 'none !important',
    color: '#8c8c8c !important',
    fontSize: '18px !important',
    padding: '2px 6px !important',
    lineHeight: '1 !important',
    borderRadius: '4px !important',
  },
  '.cm-search button[name="close"]:hover, .cm-panel.cm-search button[name="close"]:hover': {
    backgroundColor: '#303030 !important',
    color: '#ff4d4f !important',
    borderColor: 'transparent !important',
  },
});
