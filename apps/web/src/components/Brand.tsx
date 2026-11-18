import { cx } from '../lib/format'

/** The logo: a jade "D" on ink. */
export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" aria-hidden className={cx('size-9', className)}>
      <defs>
        <linearGradient id="logo-g" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#2ed3a0" />
          <stop offset="1" stopColor="#00805f" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="9" fill="#0b1220" />
      <path d="M10 9h6.5a7 7 0 0 1 0 14H10z" fill="none" stroke="url(#logo-g)" strokeWidth="3.2" strokeLinejoin="round" />
    </svg>
  )
}

export function Brand({ light }: { light?: boolean }) {
  return (
    <span className={cx('flex items-center gap-2.5 text-[17px] font-bold tracking-tight', light ? 'text-white' : 'text-fg')}>
      <Logo /> Digital Bank
    </span>
  )
}
