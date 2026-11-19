import * as SecureStore from 'expo-secure-store'
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Platform, useColorScheme } from 'react-native'

// The same semantic tokens as the web app: brand (jade) is the bank and
// money in; outgoing money stays neutral; danger only for failures; warning
// for pending; credit products are violet.
const light = {
  bg: '#f2f3f5', surface: '#ffffff', surface2: 'rgba(118,128,150,0.1)', surface3: 'rgba(118,128,150,0.2)',
  glass: 'rgba(255,255,255,0.6)', glassStrong: 'rgba(255,255,255,0.8)', glassRim: 'rgba(255,255,255,0.75)',
  fg: '#0b1220', fg2: '#354052', muted: '#667085', subtle: '#98a2b3',
  line: '#e7eaef', lineStrong: '#d0d5dd',
  brand: '#00a37a', brandStrong: '#00805f', brandSoft: '#e2f6ef', brandFg: '#ffffff',
  credit: '#7a5af8', creditSoft: '#efebff',
  positive: '#079455', positiveSoft: '#e3f6ec',
  danger: '#d92d20', dangerSoft: '#fdecea',
  warning: '#b54708', warningSoft: '#fef3e2',
  shadow: '#101828',
}

const dark: typeof light = {
  bg: '#0a0a0c', surface: '#16171b', surface2: 'rgba(255,255,255,0.07)', surface3: 'rgba(255,255,255,0.13)',
  glass: 'rgba(30,34,44,0.55)', glassStrong: 'rgba(30,34,44,0.78)', glassRim: 'rgba(255,255,255,0.1)',
  fg: '#f2f4f7', fg2: '#cdd3dd', muted: '#8d96a8', subtle: '#5d6676',
  line: '#23252b', lineStrong: '#30333a',
  brand: '#2ed3a0', brandStrong: '#63e3bb', brandSoft: '#0d2b22', brandFg: '#03140e',
  credit: '#a48bff', creditSoft: '#1f1a3b',
  positive: '#3ccf8e', positiveSoft: '#0e2a1e',
  danger: '#f97066', dangerSoft: '#2d1512',
  warning: '#fdb022', warningSoft: '#2b1f0a',
  shadow: '#000000',
}

export type Colors = typeof light
export type ThemeMode = 'system' | 'light' | 'dark'

/** Hero surfaces: deep jade for the bank, deep violet for credit. */
export const heroGradient = ['#0d6b53', '#0a4d3c'] as const
export const creditGradient = ['#3b2a8f', '#251a5e'] as const
export const font = { regular: 'Inter_400Regular', medium: 'Inter_500Medium', semibold: 'Inter_600SemiBold', bold: 'Inter_700Bold' }
export const radius = { sm: 10, md: 14, lg: 22, xl: 28 }

interface Theme { c: Colors; dark: boolean; mode: ThemeMode; setMode: (m: ThemeMode) => void }
const ThemeContext = createContext<Theme>({ c: light, dark: false, mode: 'system', setMode: () => {} })
export const useTheme = () => useContext(ThemeContext)

const KEY = 'db.theme'
const store = {
  get: async () => (Platform.OS === 'web' ? globalThis.localStorage?.getItem(KEY) : await SecureStore.getItemAsync(KEY)) as ThemeMode | null,
  set: async (m: ThemeMode) => (Platform.OS === 'web' ? globalThis.localStorage?.setItem(KEY, m) : SecureStore.setItemAsync(KEY, m)),
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const system = useColorScheme()
  const [mode, setModeState] = useState<ThemeMode>('system')
  useEffect(() => {
    store.get().then((m) => m && setModeState(m)).catch(() => {})
  }, [])
  const isDark = mode === 'dark' || (mode === 'system' && system === 'dark')
  // Web build: the page and the browser bars take the app background.
  useEffect(() => {
    if (Platform.OS !== 'web' || typeof document === 'undefined') return
    const bg = (isDark ? dark : light).bg
    document.documentElement.style.background = bg
    document.body.style.background = bg
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', bg)
  }, [isDark])
  const value = useMemo<Theme>(() => ({
    c: isDark ? dark : light,
    dark: isDark,
    mode,
    setMode: (m) => {
      setModeState(m)
      store.set(m)?.catch?.(() => {})
    },
  }), [isDark, mode])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}
