export interface ProjectType {
  name: string;
  label: string;
  description: string;
  default_port: number;
  proxy: boolean;
}

export interface DirEntry {
  name: string;
  path: string;
  is_dir: boolean;
  has_items: boolean;
  project: string;
}

export interface PathValidation {
  valid: boolean;
  message: string;
  exists?: boolean;
  writable?: boolean;
  project?: string;
}

export interface ConfigTestResult {
  valid: boolean;
  message: string;
}
