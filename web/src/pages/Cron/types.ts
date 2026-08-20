export const MODAL_TOP_OFFSET = 20;

export const STYLES = {
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
