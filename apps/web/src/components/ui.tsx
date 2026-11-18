import { AlertCircle, CheckCircle2, FlaskConical, Info, TriangleAlert, X, type LucideIcon } from 'lucide-react'
import { useEffect, useId, useLayoutEffect, useRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from 'react'
import { formatBRL, type Money } from '@bank/core'
import { cx } from '../lib/format'
import { gsap, reducedMotion } from '../lib/motion'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'credit'

export function Button({ variant = 'primary', loading, className, children, disabled, icon: Icon, size = 'md', ...rest }:
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; loading?: boolean; icon?: LucideIcon; size?: 'sm' | 'md' | 'lg' }) {
  const styles: Record<Variant, string> = {
    primary: 'gloss bg-brand text-brand-fg hover:brightness-105 shadow-[inset_0_1px_0_rgb(255_255_255/0.35),0_8px_24px_-8px_var(--brand)]',
    secondary: 'glass text-fg hover:bg-glass-strong',
    ghost: 'text-brand-strong hover:bg-surface-2',
    danger: 'glass text-danger hover:bg-danger-soft',
    credit: 'gloss bg-credit text-white hover:brightness-110 shadow-[inset_0_1px_0_rgb(255_255_255/0.3),0_8px_24px_-8px_var(--credit)]',
  }
  const sizes = { sm: 'px-3.5 py-1.5 text-xs rounded-full', md: 'px-5 py-2.5 text-sm rounded-full', lg: 'px-6 py-3.5 text-[15px] rounded-full' }
  return (
    <button
      {...rest}
      disabled={disabled || loading}
      className={cx('inline-flex items-center justify-center gap-2 font-semibold transition-all duration-200 select-none',
        'active:scale-[0.97] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand disabled:pointer-events-none disabled:opacity-45',
        styles[variant], sizes[size], className)}
    >
      {loading ? <Spinner small /> : Icon && <Icon aria-hidden className="size-4" strokeWidth={2.25} />}
      {children}
    </button>
  )
}

export function IconButton({ icon: Icon, label, className, ...rest }: ButtonHTMLAttributes<HTMLButtonElement> & { icon: LucideIcon; label: string }) {
  return (
    <button aria-label={label} title={label} {...rest}
      className={cx('grid size-10 place-items-center rounded-full text-fg-2 transition hover:bg-surface-2 hover:text-fg active:scale-90', className)}>
      <Icon className="size-5" strokeWidth={2} />
    </button>
  )
}

export function Spinner({ small }: { small?: boolean }) {
  return <span aria-hidden className={cx('inline-block animate-spin rounded-full border-2 border-current border-r-transparent', small ? 'size-4' : 'size-6')} />
}

const control = 'block w-full rounded-2xl border-0 bg-surface-2 px-4 py-3.5 text-[15px] text-fg ring-1 ring-transparent placeholder:text-subtle transition focus:bg-glass-strong focus:ring-2 focus:ring-brand focus:outline-none disabled:opacity-60'

export function Field({ label, hint, error, ...rest }: InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: string; error?: string }) {
  const id = useId()
  return (
    <label htmlFor={id} className="block">
      <span className="mb-1.5 ml-1 block text-[13px] font-medium text-muted">{label}</span>
      <input id={id} {...rest} aria-invalid={error ? true : undefined} className={cx(control, error && 'ring-danger', rest.className)} />
      {(error || hint) && <span className={cx('mt-1.5 block text-xs', error ? 'text-danger' : 'text-muted')}>{error ?? hint}</span>}
    </label>
  )
}

export function Select({ label, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement> & { label: string }) {
  const id = useId()
  return (
    <label htmlFor={id} className="block">
      {label && <span className="mb-1.5 ml-1 block text-[13px] font-medium text-muted">{label}</span>}
      <select id={id} {...rest} className={cx(control, 'appearance-none bg-[length:16px] bg-[right_12px_center] bg-no-repeat pr-10')}
        style={{ backgroundImage: "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%23889' stroke-width='2'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")" }}>
        {children}
      </select>
    </label>
  )
}

export function Toggle({ label, checked, onChange, description }: { label: string; checked: boolean; onChange: (v: boolean) => void; description?: string }) {
  return (
    <label className="flex cursor-pointer items-center justify-between gap-4 py-3">
      <span>
        <span className="block text-sm font-medium text-fg">{label}</span>
        {description && <span className="block text-xs text-muted">{description}</span>}
      </span>
      <button type="button" role="switch" aria-checked={checked} onClick={() => onChange(!checked)}
        className={cx('relative h-[31px] w-[51px] shrink-0 rounded-full transition-colors duration-300', checked ? 'bg-brand' : 'bg-surface-3')}>
        <span className={cx('absolute top-[2px] size-[27px] rounded-full bg-white shadow-[0_3px_8px_rgb(0_0_0/0.15),0_1px_1px_rgb(0_0_0/0.16)] transition-all duration-300 ease-[cubic-bezier(.34,1.56,.64,1)]', checked ? 'left-[22px]' : 'left-[2px]')} />
      </button>
    </label>
  )
}

export function Panel({ title, action, children, className, icon: Icon }: { title?: ReactNode; action?: ReactNode; children: ReactNode; className?: string; icon?: LucideIcon }) {
  return (
    <section data-reveal className={cx('glass rounded-[28px] p-5 sm:p-6', className)}>
      {(title || action) && (
        <header className="mb-4 flex items-center justify-between gap-3">
          {title && (
            <h2 className="flex items-center gap-2.5 text-[15px] font-semibold tracking-tight text-fg">
              {Icon && <span className="grid size-8 place-items-center rounded-full bg-surface-2 text-fg-2"><Icon className="size-4" /></span>}
              {title}
            </h2>
          )}
          {action}
        </header>
      )}
      {children}
    </section>
  )
}

export function PageTitle({ children, subtitle, action }: { children: ReactNode; subtitle?: ReactNode; action?: ReactNode }) {
  return (
    <div data-reveal className="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="text-[34px] leading-tight font-bold tracking-[-0.02em] text-fg">{children}</h1>
        {subtitle && <p className="mt-1 text-sm text-muted">{subtitle}</p>}
      </div>
      {action}
    </div>
  )
}

const tones = {
  green: 'bg-positive-soft text-positive',
  amber: 'bg-warning-soft text-warning',
  red: 'bg-danger-soft text-danger',
  slate: 'bg-surface-2 text-fg-2',
  brand: 'bg-brand-soft text-brand-strong',
  credit: 'bg-credit-soft text-credit',
}

export function Badge({ tone = 'slate', children }: { tone?: keyof typeof tones; children: ReactNode }) {
  return <span className={cx('inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold', tones[tone])}>{children}</span>
}

export function Alert({ tone = 'red', children }: { tone?: 'red' | 'green' | 'amber' | 'brand'; children: ReactNode }) {
  if (!children) return null
  const Icon = { red: AlertCircle, green: CheckCircle2, amber: TriangleAlert, brand: Info }[tone]
  return (
    <div role={tone === 'red' ? 'alert' : 'status'} className={cx('flex items-start gap-2.5 rounded-2xl px-4 py-3 text-sm', tones[tone])}>
      <Icon aria-hidden className="mt-px size-4 shrink-0" />
      <span>{children}</span>
    </div>
  )
}

export function Amount({ value, direction, className }: { value?: Money | string; direction?: 'DEBIT' | 'CREDIT'; className?: string }) {
  return (
    <span className={cx('tabular font-semibold whitespace-nowrap', direction === 'CREDIT' ? 'text-positive' : 'text-fg', className)}>
      {direction === 'DEBIT' ? '− ' : direction === 'CREDIT' ? '+ ' : ''}
      {formatBRL(value)}
    </span>
  )
}

export function Empty({ children, icon: Icon = Info }: { children: ReactNode; icon?: LucideIcon }) {
  return (
    <div className="flex flex-col items-center gap-3 py-10 text-center">
      <span className="grid size-14 place-items-center rounded-full bg-surface-2 text-subtle"><Icon className="size-6" /></span>
      <p className="max-w-xs text-sm text-muted">{children}</p>
    </div>
  )
}

export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cx('skeleton rounded-xl', className)} />
}

export function Loading({ rows = 3 }: { rows?: number }) {
  return (
    <div role="status" aria-label="Carregando" className="space-y-3 py-2">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="size-10 shrink-0 rounded-full" />
          <div className="flex-1 space-y-2"><Skeleton className="h-3 w-2/3" /><Skeleton className="h-3 w-1/3" /></div>
          <Skeleton className="h-4 w-20" />
        </div>
      ))}
    </div>
  )
}

export function Modal({ open, onClose, title, children }: { open: boolean; onClose: () => void; title: string; children: ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const d = ref.current
    if (!d) return
    if (open && !d.open) {
      d.showModal()
      if (!reducedMotion()) gsap.fromTo(d, { y: 24, scale: 0.97, autoAlpha: 0 }, { y: 0, scale: 1, autoAlpha: 1, duration: 0.35, ease: 'power3.out' })
    }
    if (!open && d.open) d.close()
  }, [open])
  return (
    <dialog ref={ref} onClose={onClose}
      className="glass-strong mx-auto mt-auto mb-0 w-full max-w-none rounded-t-[32px] rounded-b-none p-0 text-fg backdrop:bg-black/40 backdrop:backdrop-blur-sm sm:m-auto sm:w-[min(30rem,calc(100%-2rem))] sm:rounded-[32px]">
      <div className="p-6 pb-8">
        <span aria-hidden className="mx-auto -mt-2 mb-4 block h-[5px] w-10 rounded-full bg-fg/20 sm:hidden" />
        <div className="mb-5 flex items-start justify-between gap-4">
          <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
          <IconButton icon={X} label="Fechar" onClick={onClose} className="-mt-2 -mr-2" />
        </div>
        {open && children}
      </div>
    </dialog>
  )
}

/** Segmented tabs with a sliding indicator. */
export function Tabs<T extends string>({ value, onChange, tabs }: { value: T; onChange: (t: T) => void; tabs: [T, string][] }) {
  const list = useRef<HTMLDivElement>(null)
  const pill = useRef<HTMLSpanElement>(null)
  const first = useRef(true)
  useLayoutEffect(() => {
    const el = list.current?.querySelector<HTMLElement>('[aria-selected="true"]')
    if (!el || !pill.current) return
    const to = { x: el.offsetLeft, width: el.offsetWidth }
    if (first.current || reducedMotion()) gsap.set(pill.current, to)
    else gsap.to(pill.current, { ...to, duration: 0.45, ease: 'power3.out' })
    first.current = false
  }, [value])
  return (
    <div ref={list} role="tablist" data-reveal className="no-scrollbar relative mb-6 flex gap-1 overflow-x-auto rounded-full bg-surface-2 p-1">
      <span ref={pill} aria-hidden className="glass-strong absolute top-1 bottom-1 left-0 rounded-full" />
      {tabs.map(([id, label]) => (
        <button key={id} role="tab" aria-selected={value === id} onClick={() => onChange(id)}
          className={cx('relative z-10 flex-1 rounded-full px-4 py-2 text-[13px] font-semibold whitespace-nowrap transition-colors',
            value === id ? 'text-fg' : 'text-muted hover:text-fg-2')}>
          {label}
        </button>
      ))}
    </div>
  )
}

/** Test-only helpers, set apart from real features. */
export function Sandbox({ children }: { children: ReactNode }) {
  return (
    <div data-reveal className="rounded-[28px] border border-dashed border-warning/40 bg-warning-soft/40 p-5 backdrop-blur-xl">
      <p className="mb-3 flex items-center gap-2 text-xs font-bold tracking-wider text-warning uppercase">
        <FlaskConical className="size-4" /> Ambiente de testes
      </p>
      {children}
    </div>
  )
}

/** A rounded icon for list rows and tiles. */
export function IconTile({ icon: Icon, tone = 'slate', className }: { icon: LucideIcon; tone?: keyof typeof tones; className?: string }) {
  return <span className={cx('grid size-10 shrink-0 place-items-center rounded-full', tones[tone], className)}><Icon className="size-[18px]" strokeWidth={2.2} /></span>
}

/** A label over a value, for summaries. */
export function Stat({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <div className={className}>
      <p className="text-xs font-medium text-muted">{label}</p>
      <div className="mt-0.5 text-sm font-semibold text-fg">{children}</div>
    </div>
  )
}
