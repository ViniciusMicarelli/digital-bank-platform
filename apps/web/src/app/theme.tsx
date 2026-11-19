import { useEffect, useState, type ReactNode } from 'react'
import { ThemeContext } from './contexts'

import type { ThemeMode } from './contexts'
const KEY = 'db.theme'
const media = () => window.matchMedia('(prefers-color-scheme: dark)')

function apply(mode: ThemeMode) {
  const dark = mode === 'dark' || (mode === 'system' && media().matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
  // The browser's bars take the page background (no white strips on phones).
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', dark ? '#0a0a0c' : '#f2f3f5')
}

function read(): ThemeMode {
  try {
    return (localStorage.getItem(KEY) as ThemeMode) || 'system'
  } catch {
    return 'system'
  }
}

/** Holds the chosen theme mode; 'system' follows the OS live. */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setMode] = useState<ThemeMode>(read)
  useEffect(() => {
    apply(mode)
    if (mode !== 'system') return
    const m = media()
    const onChange = () => apply('system')
    m.addEventListener('change', onChange)
    return () => m.removeEventListener('change', onChange)
  }, [mode])
  const choose = (m: ThemeMode) => {
    try {
      localStorage.setItem(KEY, m)
    } catch {
      /* per-tab only */
    }
    setMode(m)
  }
  return <ThemeContext.Provider value={[mode, choose]}>{children}</ThemeContext.Provider>
}
