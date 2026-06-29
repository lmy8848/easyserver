/**
 * Theme color constants.
 * Replace hardcoded color strings with these tokens.
 */
export const COLORS = {
  // Primary / brand
  PRIMARY: '#1890ff',

  // Semantic
  SUCCESS: '#52c41a',
  WARNING: '#faad14',
  ERROR: '#ff4d4f',
  ERROR_DARK: '#cf1322',
  SUCCESS_DARK: '#3f8600',

  // Text
  TEXT_SECONDARY: '#666',
  TEXT_DISABLED: '#999',
  TEXT_WHITE: '#fff',
  TEXT_SIDER: 'rgba(255,255,255,0.65)',

  // Background
  BG_LAYOUT: '#f0f2f5',
  BG_HEADER: '#fff',

  // Terminal
  TERMINAL_BG: '#1e1e1e',
  TERMINAL_FG: '#d4d4d4',

  // Login
  LOGIN_BG: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
} as const;
