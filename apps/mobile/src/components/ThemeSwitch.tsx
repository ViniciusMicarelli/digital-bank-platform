import { useTheme, type ThemeMode } from '../lib/theme'
import { Segmented } from './ui'

export function ThemeSwitch() {
  const { mode, setMode } = useTheme()
  return <Segmented<ThemeMode> value={mode} onChange={setMode} options={[['system', 'Sistema'], ['light', 'Claro'], ['dark', 'Escuro']]} />
}
