'use client';

import {useCallback, useSyncExternalStore} from 'react';

/** localStorage key holding the user's explicit theme choice. */
const STORAGE_KEY = 'tw-theme';
/** Window event broadcast when the theme changes, to sync all subscribers. */
const EVENT = 'tw-theme-change';

/**
 * Inline script (stringified) that applies the persisted theme before first
 * paint, preventing a light→dark flash. Injected in `<head>` so it runs
 * synchronously ahead of hydration. The `.dark` class on `<html>` is the single
 * source of truth that {@link useTheme} reads.
 */
export const themeNoFlashScript = `(function(){try{var t=localStorage.getItem('${STORAGE_KEY}');var d=t?t==='dark':false;document.documentElement.classList.toggle('dark',d);}catch(e){}})();`;

function subscribe(callback: () => void): () => void {
  window.addEventListener(EVENT, callback);
  window.addEventListener('storage', callback);
  return () => {
    window.removeEventListener(EVENT, callback);
    window.removeEventListener('storage', callback);
  };
}

function getSnapshot(): boolean {
  return document.documentElement.classList.contains('dark');
}

function getServerSnapshot(): boolean {
  return false; // SSR renders light; the no-flash script reconciles on the client.
}

/**
 * Pass-through provider. State lives on the `<html>` `.dark` class (set before
 * paint by {@link themeNoFlashScript}); hooks read it via useSyncExternalStore.
 */
export function ThemeProvider({children}: {children: React.ReactNode}) {
  return <>{children}</>;
}

/** Theme state + controls. Hydration-safe (reads the DOM, not React state). */
export function useTheme(): {
  dark: boolean;
  toggle: () => void;
  setDark: (dark: boolean) => void;
} {
  const dark = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const setDark = useCallback((next: boolean) => {
    document.documentElement.classList.toggle('dark', next);
    try {
      localStorage.setItem(STORAGE_KEY, next ? 'dark' : 'light');
    } catch {
      // Private mode / storage disabled: theme still applies for this session.
    }
    window.dispatchEvent(new Event(EVENT));
  }, []);

  const toggle = useCallback(() => setDark(!getSnapshot()), [setDark]);

  return {dark, toggle, setDark};
}

/** Convenience hook returning just whether the dark variant is active. */
export function useDark(): boolean {
  return useTheme().dark;
}
