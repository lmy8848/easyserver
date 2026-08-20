/**
 * Zero-dependency ANSI escape sequence parser and utility for terminal output.
 */

export interface AnsiSpan {
  text: string;
  color?: string;
  backgroundColor?: string;
  bold?: boolean;
  dim?: boolean;
  italic?: boolean;
  underline?: boolean;
}

// Terminal 16 colors (optimized for dark/terminal backgrounds)
const STANDARD_COLORS: Record<number, string> = {
  30: '#4d4d4d', // Black
  31: '#ff4d4f', // Red
  32: '#52c41a', // Green
  33: '#faad14', // Yellow
  34: '#1677ff', // Blue
  35: '#eb2f96', // Magenta
  36: '#13c2c2', // Cyan
  37: '#e6e6e6', // White
  90: '#8c8c8c', // Bright Black (Gray)
  91: '#ff7875', // Bright Red
  92: '#73d13d', // Bright Green
  93: '#ffc53d', // Bright Yellow
  94: '#4096ff', // Bright Blue
  95: '#f759ab', // Bright Magenta
  96: '#36cfc9', // Bright Cyan
  97: '#ffffff', // Bright White
};

const STANDARD_BG_COLORS: Record<number, string> = {
  40: '#000000',
  41: '#a8071a',
  42: '#237804',
  43: '#ad6800',
  44: '#0958d9',
  45: '#9e1068',
  46: '#08979c',
  47: '#d9d9d9',
  100: '#595959',
  101: '#cf1322',
  102: '#389e0d',
  103: '#d48806',
  104: '#1d39c4',
  105: '#c41d7f',
  106: '#13a8a8',
  107: '#ffffff',
};

// Standard 256 color lookup
function get256Color(n: number): string {
  if (n < 0 || n > 255) return '#ffffff';
  if (n < 8) return STANDARD_COLORS[30 + n] || '#ffffff';
  if (n < 16) return STANDARD_COLORS[90 + (n - 8)] || '#ffffff';
  if (n >= 232) {
    // Grayscale ramp from 232 to 255
    const gray = Math.round(((n - 232) / 23) * 255);
    return `rgb(${gray},${gray},${gray})`;
  }
  // 6x6x6 color cube: 16 + 36*r + 6*g + b
  const c = n - 16;
  const r = Math.floor(c / 36);
  const g = Math.floor((c % 36) / 6);
  const b = c % 6;
  const toRgb = (v: number) => (v === 0 ? 0 : v * 40 + 55);
  return `rgb(${toRgb(r)},${toRgb(g)},${toRgb(b)})`;
}

/**
 * Comprehensive ANSI escape sequence regex:
 * 1. OSC sequences: \x1b\] ... (\x07|\x1b\\)
 * 2. CSI sequences (including private modes with ?/=/</>): \x1b\[ ... [a-zA-Z]
 * 3. 2-byte escape sequences: \x1b[@-Z\\-_]
 */
// eslint-disable-next-line no-control-regex
export const ANSI_REGEX = /\x1b(?:\][^\x07\x1b]*(?:\x07|\x1b\\)|\[([?>=<]?)([0-9;]*)([a-zA-Z])|[@-Z\\-_])/g;

/**
 * Strips all ANSI, OSC, and terminal control codes from string for clean copy/download.
 */
export function stripAnsi(str: string): string {
  if (!str) return '';
  return str.replace(ANSI_REGEX, '');
}

/**
 * Splits multiline string and handles carriage return \r progress updates.
 */
export function splitLinesWithCr(text: string): string[] {
  if (!text) return [];
  const rawLines = text.replace(/\r\n/g, '\n').split('\n');
  return rawLines.map((line): string => {
    if (line.includes('\r')) {
      const parts = line.split('\r').filter(Boolean);
      return parts.length > 0 ? (parts[parts.length - 1] ?? '') : '';
    }
    return line;
  });
}

function createSpan(
  text: string,
  color?: string,
  bgColor?: string,
  bold?: boolean,
  dim?: boolean,
  italic?: boolean,
  underline?: boolean
): AnsiSpan {
  const span: AnsiSpan = { text };
  if (color) span.color = color;
  if (bgColor) span.backgroundColor = bgColor;
  if (bold) span.bold = true;
  if (dim) span.dim = true;
  if (italic) span.italic = true;
  if (underline) span.underline = true;
  return span;
}

/**
 * Parses ANSI-formatted string into an array of styled text spans.
 */
export function parseAnsi(input: string): AnsiSpan[] {
  if (!input) return [];
  if (!input.includes('\x1b')) {
    return [{ text: input }];
  }

  const spans: AnsiSpan[] = [];
  let currentColor: string | undefined;
  let currentBgColor: string | undefined;
  let currentBold: boolean | undefined;
  let currentDim: boolean | undefined;
  let currentItalic: boolean | undefined;
  let currentUnderline: boolean | undefined;

  let lastIndex = 0;
  ANSI_REGEX.lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = ANSI_REGEX.exec(input)) !== null) {
    const textChunk = input.slice(lastIndex, match.index);
    if (textChunk) {
      spans.push(
        createSpan(
          textChunk,
          currentColor,
          currentBgColor,
          currentBold,
          currentDim,
          currentItalic,
          currentUnderline
        )
      );
    }

    const prefix = match[1];
    const paramsStr = match[2] ?? '';
    const command = match[3];
    lastIndex = ANSI_REGEX.lastIndex;

    // We only process standard SGR (Select Graphic Rendition) 'm' commands without private prefix
    if (command === 'm' && !prefix) {
      const codes = paramsStr.length === 0 ? [0] : paramsStr.split(';').map(Number);
      let i = 0;
      while (i < codes.length) {
        const code = codes[i];
        if (code === undefined) break;

        if (code === 0) {
          currentColor = undefined;
          currentBgColor = undefined;
          currentBold = undefined;
          currentDim = undefined;
          currentItalic = undefined;
          currentUnderline = undefined;
        } else if (code === 1) {
          currentBold = true;
        } else if (code === 2) {
          currentDim = true;
        } else if (code === 3) {
          currentItalic = true;
        } else if (code === 4) {
          currentUnderline = true;
        } else if (code === 22) {
          currentBold = undefined;
          currentDim = undefined;
        } else if (code === 23) {
          currentItalic = undefined;
        } else if (code === 24) {
          currentUnderline = undefined;
        } else if (code === 39) {
          currentColor = undefined;
        } else if (code === 49) {
          currentBgColor = undefined;
        } else if (STANDARD_COLORS[code]) {
          currentColor = STANDARD_COLORS[code];
        } else if (STANDARD_BG_COLORS[code]) {
          currentBgColor = STANDARD_BG_COLORS[code];
        } else if (code === 38) {
          // Foreground: 38;5;n or 38;2;r;g;b
          const p1 = codes[i + 1];
          const p2 = codes[i + 2];
          const p3 = codes[i + 3];
          const p4 = codes[i + 4];
          if (p1 === 5 && p2 !== undefined) {
            currentColor = get256Color(p2);
            i += 2;
          } else if (p1 === 2 && p2 !== undefined && p3 !== undefined && p4 !== undefined) {
            currentColor = `rgb(${p2},${p3},${p4})`;
            i += 4;
          }
        } else if (code === 48) {
          // Background: 48;5;n or 48;2;r;g;b
          const p1 = codes[i + 1];
          const p2 = codes[i + 2];
          const p3 = codes[i + 3];
          const p4 = codes[i + 4];
          if (p1 === 5 && p2 !== undefined) {
            currentBgColor = get256Color(p2);
            i += 2;
          } else if (p1 === 2 && p2 !== undefined && p3 !== undefined && p4 !== undefined) {
            currentBgColor = `rgb(${p2},${p3},${p4})`;
            i += 4;
          }
        }
        i++;
      }
    }
  }

  // Trailing text after last ANSI sequence
  const remainingText = input.slice(lastIndex);
  if (remainingText) {
    spans.push(
      createSpan(
        remainingText,
        currentColor,
        currentBgColor,
        currentBold,
        currentDim,
        currentItalic,
        currentUnderline
      )
    );
  }

  return spans;
}
