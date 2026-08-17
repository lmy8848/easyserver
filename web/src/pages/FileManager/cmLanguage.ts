import { languages } from '@codemirror/language-data';
import { LanguageDescription } from '@codemirror/language';
import type { Extension } from '@codemirror/state';

/**
 * 利用 CodeMirror 官方 @codemirror/language-data 库，
 * 根据文件名自动匹配并异步动态加载对应语言包（支持 100+ 种语言）。
 * package.json 中无需单独安装任何 @codemirror/lang-* 子包，全部由 language-data 内部按需分块加载。
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
