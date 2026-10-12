import { useSyncExternalStore } from 'react';

export type Theme = 'dark' | 'light';

/** Shared with public/theme.js, which applies the theme before first paint. */
const STORAGE_KEY = 'protolab-theme';

const listeners = new Set<() => void>();

function current(): Theme {
  return document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
}

export function setTheme(theme: Theme): void {
  document.documentElement.dataset.theme = theme;
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // Storage can be blocked (private window); the choice then lasts for the session only.
  }
  listeners.forEach((notify) => notify());
}

export function useTheme(): { theme: Theme; toggle: () => void } {
  const theme = useSyncExternalStore(
    (notify) => {
      listeners.add(notify);
      return () => listeners.delete(notify);
    },
    current,
    (): Theme => 'dark',
  );
  return { theme, toggle: () => setTheme(theme === 'dark' ? 'light' : 'dark') };
}
