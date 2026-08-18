/** Validate a file path for safety (reject null bytes, control chars, and traversal) */
export const isValidPath = (p: string): boolean => {
  if (!p || p.includes('\x00')) return false;
  const parts = p.split('/');
  for (const part of parts) {
    if (part === '..') return false;
  }
  return true;
};
