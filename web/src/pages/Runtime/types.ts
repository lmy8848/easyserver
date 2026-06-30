export interface RuntimeEnvironment {
  id: number;
  name: string;
  version: string;
  path: string;
  is_default: boolean;
  status: string;
  progress: number;
  progress_step: string;
  error_message: string;
  installed_at: string;
}

export interface VersionInfo {
  version: string;
  installed: boolean;
  is_default: boolean;
}

export interface PackageInfo {
  name: string;
  version: string;
  scope: string;
  source: string;
}

export interface LogsData {
  id: number;
  name: string;
  version: string;
  status: string;
  progress: number;
  progress_step: string;
  logs: string;
  error_message: string;
}

export interface CleanupData {
  runtime: {
    name: string;
    version: string;
  };
  will_cleanup: {
    env_configs_count: number;
    path_entries_count: number;
  };
  env_configs: Array<{ id: number; name: string; value: string }>;
  path_entries: Array<{ id: number; path: string }>;
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
