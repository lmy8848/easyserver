import { create } from 'zustand';

export type ThemeMode = 'auto' | 'light' | 'dark';

interface ThemeState {
  mode: ThemeMode;
  isDark: boolean;
  setMode: (mode: ThemeMode) => void;
}

const THEME_STORAGE_KEY = 'easyserver_theme_mode';

function getSystemDark(): boolean {
  if (typeof window === 'undefined') return false;
  return Boolean(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
}

function getInitialMode(): ThemeMode {
  if (typeof window === 'undefined') return 'auto';
  const saved = localStorage.getItem(THEME_STORAGE_KEY) as ThemeMode | null;
  if (saved === 'light' || saved === 'dark' || saved === 'auto') {
    return saved;
  }
  return 'auto';
}

function resolveIsDark(mode: ThemeMode): boolean {
  if (mode === 'dark') return true;
  if (mode === 'light') return false;
  return getSystemDark();
}

function applyThemeToDOM(isDark: boolean) {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', isDark ? 'dark' : 'light');
    if (isDark) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
  }
}

export const useThemeStore = create<ThemeState>((set, get) => {
  const initialMode = getInitialMode();
  const initialIsDark = resolveIsDark(initialMode);
  applyThemeToDOM(initialIsDark);

  // Listen to system theme changes in real time
  if (typeof window !== 'undefined' && window.matchMedia) {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handleChange = () => {
      if (get().mode === 'auto') {
        const isDark = getSystemDark();
        applyThemeToDOM(isDark);
        set({ isDark });
      }
    };
    if (mediaQuery.addEventListener) {
      mediaQuery.addEventListener('change', handleChange);
    } else {
      mediaQuery.addListener(handleChange);
    }
  }

  return {
    mode: initialMode,
    isDark: initialIsDark,
    setMode: (mode: ThemeMode) => {
      localStorage.setItem(THEME_STORAGE_KEY, mode);
      const isDark = resolveIsDark(mode);
      applyThemeToDOM(isDark);
      set({ mode, isDark });
    },
  };
});
