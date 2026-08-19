// ==================== Shared Types ====================

export interface ImageTemplate {
  name: string;
  image: string;
  description: string;
  ports?: string[];
  env?: Record<string, string>;
  volumes?: string[];
}

export interface ImageCategory {
  name: string;
  images: ImageTemplate[];
}

export type {
  DockerStatus,
  Container,
  Image,
  ComposeProject,
  Volume,
  Network,
  ContainerStats,
} from '../../types';

// ==================== Helpers ====================

// withEngine appends `engine=podman` to a URL when the active engine is
// podman. Docker is the backend default, so no param is needed for it.
export function withEngine(url: string, engine: string): string {
  if (engine !== 'podman') return url;
  return url + (url.includes('?') ? '&' : '?') + 'engine=podman';
}

// Re-export from shared utils (backward compatible)
export { formatBytes } from '../../utils/format';
export { getStatusColorName as getStatusColor } from '../../utils/status';
