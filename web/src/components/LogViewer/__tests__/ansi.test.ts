import { describe, it, expect } from 'vitest';
import { stripAnsi, parseAnsi, splitLinesWithCr } from '../ansi';

describe('ANSI Parser & Utilities', () => {
  describe('stripAnsi', () => {
    it('should return plain text unchanged', () => {
      expect(stripAnsi('Hello world')).toBe('Hello world');
    });

    it('should strip basic 16-color ANSI codes', () => {
      const colored = '\x1b[31mError:\x1b[0m \x1b[32mSuccess\x1b[0m';
      expect(stripAnsi(colored)).toBe('Error: Success');
    });

    it('should strip 256-color and 24-bit truecolor codes', () => {
      const complex = '\x1b[38;5;196mRed\x1b[0m \x1b[38;2;255;100;50mRGB\x1b[0m';
      expect(stripAnsi(complex)).toBe('Red RGB');
    });

    it('should strip bold, dim, and reset codes', () => {
      const styled = '\x1b[1mBold\x1b[22m \x1b[2mDim\x1b[0m';
      expect(stripAnsi(styled)).toBe('Bold Dim');
    });

    it('should strip OSC title sequences with BEL and ST terminators', () => {
      const oscBel = '\x1b]0;npm install\x07Running build';
      expect(stripAnsi(oscBel)).toBe('Running build');

      const oscSt = '\x1b]2;node process\x1b\\Output ready';
      expect(stripAnsi(oscSt)).toBe('Output ready');
    });

    it('should strip CSI private modes like cursor hide/show and screen modes', () => {
      const privateCsi = '\x1b[?25lLoading...\x1b[?25h Done\x1b[?7h';
      expect(stripAnsi(privateCsi)).toBe('Loading... Done');
    });
  });

  describe('splitLinesWithCr', () => {
    it('should split standard newline strings', () => {
      const input = 'line 1\nline 2\r\nline 3';
      expect(splitLinesWithCr(input)).toEqual(['line 1', 'line 2', 'line 3']);
    });

    it('should handle carriage return \\r progress overwrites by taking the last segment', () => {
      const input = 'Downloading 10%\rDownloading 50%\rDownloading 100%\nDone';
      expect(splitLinesWithCr(input)).toEqual(['Downloading 100%', 'Done']);
    });
  });

  describe('parseAnsi', () => {
    it('should parse plain text into a single span without styles', () => {
      const spans = parseAnsi('Simple text');
      expect(spans).toEqual([{ text: 'Simple text' }]);
    });

    it('should parse standard colors correctly', () => {
      const spans = parseAnsi('\x1b[31mRed text\x1b[0m Normal');
      expect(spans).toHaveLength(2);
      expect(spans[0]?.text).toBe('Red text');
      expect(spans[0]?.color).toBe('#ff4d4f');
      expect(spans[1]?.text).toBe(' Normal');
      expect(spans[1]?.color).toBeUndefined();
    });

    it('should parse compound styles (bold + colored)', () => {
      const spans = parseAnsi('\x1b[1;32mBold Green\x1b[0m');
      expect(spans).toHaveLength(1);
      expect(spans[0]?.text).toBe('Bold Green');
      expect(spans[0]?.bold).toBe(true);
      expect(spans[0]?.color).toBe('#52c41a');
    });

    it('should handle reset codes (0 or 22/24/39)', () => {
      const spans = parseAnsi('\x1b[1mBold\x1b[22mNot bold \x1b[34mBlue\x1b[39mDefault');
      expect(spans).toEqual([
        { text: 'Bold', bold: true },
        { text: 'Not bold ' },
        { text: 'Blue', color: '#1677ff' },
        { text: 'Default' },
      ]);
    });

    it('should parse 256-color palette foregrounds', () => {
      const spans = parseAnsi('\x1b[38;5;1m256-Red\x1b[0m');
      expect(spans[0]?.text).toBe('256-Red');
      expect(spans[0]?.color).toBeDefined();
    });

    it('should parse 24-bit truecolor RGB foregrounds', () => {
      const spans = parseAnsi('\x1b[38;2;100;150;200mRGB Text\x1b[0m');
      expect(spans[0]?.text).toBe('RGB Text');
      expect(spans[0]?.color).toBe('rgb(100,150,200)');
    });

    it('should ignore OSC sequences and private CSI sequences without displaying garbage', () => {
      const spans = parseAnsi('\x1b]0;Title\x07\x1b[?25l\x1b[32mSuccess\x1b[0m\x1b[?25h');
      expect(spans).toEqual([
        { text: 'Success', color: '#52c41a' },
      ]);
    });
  });
});
