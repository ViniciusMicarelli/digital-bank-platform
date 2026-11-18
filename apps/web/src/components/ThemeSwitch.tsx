import { Monitor, Moon, Sun } from 'lucide-react'
import { useThemeMode, type ThemeMode } from '../app/contexts'
import { cx } from '../lib/format'

const modes: [ThemeMode, string, typeof Sun][] = [['system', 'Sistema', Monitor], ['light', 'Claro', Sun], ['dark', 'Escuro', Moon]]

/** Sistema / Claro / Escuro. compact: icons only. */
export function ThemeSwitch({ compact }: { compact?: boolean }) {
  const [mode, setMode] = useThemeMode()
  return (
    <div role="radiogroup" aria-label="Aparência" className="inline-flex gap-1 rounded-xl bg-surface-2 p-1">
      {modes.map(([m, label, Icon]) => (
        <button key={m} role="radio" aria-checked={mode === m} aria-label={label} title={label} onClick={() => setMode(m)}
          className={cx('flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-semibold transition',
            mode === m ? 'bg-surface text-fg shadow-card ring-1 ring-line' : 'text-muted hover:text-fg')}>
          <Icon className="size-4" />{!compact && label}
        </button>
      ))}
    </div>
  )
}
