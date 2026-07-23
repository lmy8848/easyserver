export const MODAL_TOP_OFFSET = 20;

export const MARKDOWN_STYLES = {
  table: { borderCollapse: 'collapse' as const, width: '100%', marginBottom: 16 },
  th: { border: '1px solid #d9d9d9', padding: '8px 12px', background: '#fafafa', fontWeight: 600 },
  td: { border: '1px solid #d9d9d9', padding: '8px 12px' },
  code: { background: '#f5f5f5', padding: '2px 6px', borderRadius: 4, fontSize: 13, fontFamily: 'monospace' },
  pre: { background: '#f5f5f5', padding: 16, borderRadius: 8, overflow: 'auto' as const, marginBottom: 16 },
};

export const STYLES = {
  scheduleTag: { fontFamily: 'monospace', fontSize: 12 },
  presetSelect: { width: '100%' },
  description: { color: '#8c8c8c', fontSize: 12, marginTop: 4, minHeight: 18 },
  nextRunItem: { fontFamily: 'monospace' as const, fontSize: 12 },
  modal: { top: MODAL_TOP_OFFSET },
};

export interface Preset {
  label: string;
  value: string;
  description: string;
}
