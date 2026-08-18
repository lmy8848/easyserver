import { useSearchParams } from 'react-router-dom';
import { useCallback, useMemo } from 'react';

export interface UseTabOptions {
  paramKey?: string;
  replace?: boolean;
  clearDefault?: boolean;
}

/**
 * useTab Hook - Binds tab state with URL query parameters (e.g. ?tab=xxx)
 *
 * @param defaultTab The default active tab key
 * @param options Query parameter key (default: 'tab') or config options
 * @returns [activeTab, setActiveTab]
 */
export function useTab<T extends string = string>(
  defaultTab: string,
  options?: string | UseTabOptions
): [T, (tab: string, overrideReplace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();

  const opts: UseTabOptions = useMemo(() => {
    if (typeof options === 'string') {
      return { paramKey: options, replace: true, clearDefault: true };
    }
    return {
      paramKey: options?.paramKey ?? 'tab',
      replace: options?.replace ?? true,
      clearDefault: options?.clearDefault ?? true,
    };
  }, [options]);

  const { paramKey = 'tab', replace = true, clearDefault = true } = opts;

  const currentTab = (searchParams.get(paramKey) || defaultTab) as T;

  const setTab = useCallback(
    (newTab: string, overrideReplace?: boolean) => {
      if (newTab === currentTab) return;
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (clearDefault && newTab === defaultTab) {
            next.delete(paramKey);
          } else {
            next.set(paramKey, newTab);
          }
          return next;
        },
        { replace: overrideReplace !== undefined ? overrideReplace : replace }
      );
    },
    [clearDefault, currentTab, defaultTab, paramKey, replace, setSearchParams]
  );

  return [currentTab, setTab];
}
