import { Tooltip } from 'antd';
import type { ReactNode } from 'react';

// renderCell renders a table cell according to the column's type category. The
// direct-connection channel returns native values (numbers, null, []byte, ISO
// times); the CLI fallback returns plain strings, which render as-is. A string
// literally equal to "NULL" is treated as NULL for display.
export function renderCell(v: any, type?: string): ReactNode {
  if (v === null || v === undefined) return <span style={{ color: '#bfbfbf', fontStyle: 'italic' }}>NULL</span>;
  const cat = type || (typeof v === 'number' ? 'number' : 'string');
  if (cat === 'blob') {
    const bytes = typeof v === 'string' ? hexToBytes(v) : Array.isArray(v) ? Uint8Array.from(v) : null;
    const preview = bytes && bytes.length > 0
      ? `0x${Array.from(bytes.slice(0, 8)).map(b => b.toString(16).padStart(2, '0')).join('')}${bytes.length > 8 ? '…' : ''}`
      : 'BLOB';
    return (
      <Tooltip title={`BLOB（${bytes?.length ?? '?'} 字节）`}>
        <code style={{ fontSize: 12 }}>{preview}</code>
      </Tooltip>
    );
  }
  if (cat === 'time') {
    const t = formatCellTime(v);
    return t;
  }
  if (typeof v === 'string' && v === 'NULL') return <span style={{ color: '#bfbfbf', fontStyle: 'italic' }}>NULL</span>;
  return String(v);
}

function hexToBytes(hex: string): Uint8Array | null {
  if (!/^[0-9a-fA-F]+$/.test(hex)) return null;
  const out = new Uint8Array(Math.floor(hex.length / 2));
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

// formatCellTime renders a time cell in the browser's local timezone. The value
// is a driver time (ISO string) or a raw timestamp string from the CLI channel.
function formatCellTime(v: any): string {
  if (typeof v === 'string') {
    const t = new Date(v);
    if (!Number.isNaN(t.getTime()) && /\d/.test(v)) {
      return t.toLocaleString();
    }
  } else if (v instanceof Date) {
    return v.toLocaleString();
  }
  return String(v);
}
