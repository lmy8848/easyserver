/** Validate a file path for safety (reject null bytes, control chars, and traversal) */
export const isValidPath = (p: string): boolean => {
  if (!p || p.includes('\x00')) return false;
  const parts = p.split('/');
  for (const part of parts) {
    if (part === '..') return false;
  }
  return true;
};

/** Format byte size to human-readable string */
export const formatFileSize = (size: number): string => {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
  return `${(size / 1024 / 1024 / 1024).toFixed(1)} GB`;
};
