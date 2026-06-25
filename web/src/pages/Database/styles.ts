const STYLES = {
  cardActions: {
    marginTop: 12,
    borderTop: '1px solid #f0f0f0',
    paddingTop: 12,
    display: 'flex' as const,
    justifyContent: 'space-between' as const,
    flexWrap: 'wrap' as const,
    gap: 8,
  },
  versionInfo: {
    color: '#999',
    fontSize: 12,
    marginTop: 4,
  },
  emptyHint: {
    margin: '4px 0',
    color: '#52c41a',
  },
  logContainer: {
    background: '#fafafa',
    border: '1px solid #e8e8e8',
    fontFamily: "'Cascadia Code', 'Fira Code', 'Consolas', monospace",
    fontSize: 13,
    lineHeight: 1.8,
    padding: '8px 0',
    borderRadius: 6,
    maxHeight: '60vh',
    overflowY: 'auto' as const,
    overflowX: 'auto' as const,
  },
  logLine: {
    display: 'flex' as const,
    alignItems: 'baseline' as const,
    padding: '0 12px',
    minHeight: 22,
  },
  logLineNumber: {
    color: '#bfbfbf',
    minWidth: 36,
    width: 36,
    textAlign: 'right' as const,
    marginRight: 16,
    userSelect: 'none' as const,
    fontSize: 11,
    flexShrink: 0,
  },
  logLineText: {
    whiteSpace: 'nowrap' as const,
    color: '#262626',
  },
  skeletonLine: {
    display: 'flex' as const,
    gap: 8,
    marginBottom: 6,
  },
  skeletonBar: (width: number | string) => ({
    width,
    height: 14,
    background: '#f5f5f5',
    borderRadius: 2,
  }),
  portHint: {
    color: '#52c41a',
  },
  portHintError: {
    color: '#ff4d4f',
  },
} as const;

export default STYLES;
