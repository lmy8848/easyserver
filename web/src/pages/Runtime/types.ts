export interface RuntimeEnvironment {
  name: string;          // lang: node / python / ...
  version: string;       // exact: 20.11.0
  path: string;
  status: string;        // 恒 'installed'（目录扫描权威，ADR-0009）
}

export interface VersionInfo {
  version: string;
  installed: boolean;
}

export interface PackageInfo {
  name: string;
  version: string;
  scope: string;
  source: string;
}

export interface LogsData {
  name: string;          // lang
  version: string;       // exact
  status: string;
  logs: string;
}

export interface CleanupData {
  runtime: {
    name: string;
    version: string;
  };
}

export interface PackageSearchResult {
  name: string;
  description: string;
}

export interface CatalogEntry {
  lang: string;
  display: string;
  mise_tool: string;
  majors: string[];
  supports_global_pkgs: boolean;
  mirror_envs: string[];
  mirror_candidates: string[];
}

export const RUNTIME_ICON_MAP: Record<string, string> = {
  java: '☕',
  node: '🟢',
  go: '🔵',
  python: '🐍',
  php: '🐘',
};

export function getRuntimeIcon(name: string): string {
  return RUNTIME_ICON_MAP[name] || '📦';
}
