import { useQuery } from '@tanstack/react-query'
import { ArrowLeftRight, Bell, CreditCard, FileText, Home, LogOut, Menu, ReceiptText, Settings, Wallet, type LucideIcon } from 'lucide-react'
import { Suspense, useRef } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router'
import { useLiveConnected } from '../app/contexts'
import { useAccount } from '../app/queries'
import { api, logout } from '../lib/api'
import { cx } from '../lib/format'
import { gsap, reducedMotion, reveal, useGSAP } from '../lib/motion'
import { Brand } from './Brand'
import { ThemeSwitch } from './ThemeSwitch'

interface Item { to: string; label: string; icon: LucideIcon; mobile: boolean; desktop?: boolean }

const nav: Item[] = [
  { to: '/', label: 'Início', icon: Home, mobile: true },
  { to: '/extrato', label: 'Extrato', icon: ReceiptText, mobile: true },
  { to: '/pix', label: 'Pix', icon: ArrowLeftRight, mobile: true },
  { to: '/cartoes', label: 'Cartões', icon: CreditCard, mobile: true },
  { to: '/credito', label: 'Crédito', icon: Wallet, mobile: false },
  { to: '/boletos', label: 'Boletos', icon: FileText, mobile: false },
  { to: '/notificacoes', label: 'Notificações', icon: Bell, mobile: false },
  { to: '/ajustes', label: 'Ajustes', icon: Settings, mobile: false },
  { to: '/mais', label: 'Mais', icon: Menu, mobile: true, desktop: false },
]

function useUnread() {
  return useQuery({
    queryKey: ['notifications', 'unread'],
    queryFn: async () => (await api<{ unread_count: number }>('GET', '/v1/notifications?unread=true&limit=1')).unread_count,
  }).data ?? 0
}

function Initials({ name }: { name?: string }) {
  const parts = (name ?? '').split(' ').filter(Boolean)
  return (
    <span className="grid size-9 place-items-center rounded-full bg-brand-soft text-xs font-bold text-brand-strong">
      {(parts[0]?.[0] ?? '') + (parts.at(-1)?.[0] ?? '')}
    </span>
  )
}

export function Layout() {
  const account = useAccount()
  const unread = useUnread()
  const live = useLiveConnected()
  const { pathname } = useLocation()
  const main = useRef<HTMLElement>(null)

  // Page transition, then the page's own [data-reveal] cascade.
  useGSAP(() => {
    if (reducedMotion()) return
    gsap.from('[data-reveal]', reveal)
  }, { scope: main, dependencies: [pathname] })

  return (
    <div className="min-h-full lg:flex">
      <aside className="glass sticky top-3 m-3 hidden h-[calc(100vh-1.5rem)] w-[260px] shrink-0 flex-col self-start rounded-[30px] px-3.5 py-6 lg:flex">
        <div className="px-2"><Brand /></div>
        <nav className="mt-9 flex flex-1 flex-col gap-1">
          {nav.filter((n) => n.desktop !== false).map((n) => (
            <NavLink key={n.to} to={n.to} end={n.to === '/'}
              className={({ isActive }) => cx('group relative flex items-center gap-3 rounded-full px-3.5 py-2.5 text-[14px] font-medium transition',
                isActive ? 'glass-strong text-fg' : 'text-fg-2 hover:bg-surface-2 hover:text-fg')}>
              {({ isActive }) => (
                <>
                  <n.icon className={cx('size-[18px]', isActive && 'text-brand-strong')} strokeWidth={isActive ? 2.4 : 2} />
                  {n.label}
                  {n.to === '/notificacoes' && unread > 0 && (
                    <span className="ml-auto min-w-6 rounded-full bg-brand px-1.5 text-center text-[11px] leading-5 font-bold text-brand-fg">{unread}</span>
                  )}
                </>
              )}
            </NavLink>
          ))}
        </nav>
        <div className="mb-3 px-1"><ThemeSwitch compact /></div>
        {account && (
          <div className="flex items-center gap-3 rounded-[22px] bg-surface-2 p-3">
            <Initials name={account.holder_name} />
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-semibold text-fg">{account.holder_name}</p>
              <p className="truncate text-xs text-muted">Ag {account.branch} · {account.number}</p>
            </div>
            <button onClick={() => logout()} aria-label="Sair" title="Sair" className="rounded-lg p-1.5 text-muted transition hover:bg-surface-3 hover:text-fg">
              <LogOut className="size-4" />
            </button>
          </div>
        )}
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="bar safe-top sticky top-0 z-30 flex items-center justify-between border-b px-4 pb-3 lg:hidden">
          <Brand />
          <NavLink to="/notificacoes" className="relative grid size-10 place-items-center rounded-xl text-fg-2" aria-label="Notificações">
            <Bell className="size-5" />
            {unread > 0 && <span className="absolute top-1.5 right-1.5 grid min-w-4 place-items-center rounded-full bg-brand px-1 text-[10px] font-bold text-brand-fg">{unread}</span>}
          </NavLink>
        </header>
        <main ref={main} data-layout-main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6 pb-[calc(8rem+env(safe-area-inset-bottom))] sm:px-6 lg:px-8 lg:py-9">
          <Suspense fallback={<div className="space-y-4"><div className="skeleton h-10 w-48 rounded-xl" /><div className="skeleton h-64 rounded-3xl" /></div>}>
            <Outlet />
          </Suspense>
        </main>
        <p className="hidden items-center gap-2 px-10 pb-5 text-xs text-subtle lg:flex">
          <span className={cx('relative inline-flex size-2 rounded-full', live ? 'bg-positive' : 'bg-subtle')}>
            {live && <span className="absolute inset-0 animate-ping rounded-full bg-positive opacity-60" />}
          </span>
          {live ? 'Atualizações em tempo real' : 'Reconectando…'}
        </p>
      </div>

      <nav className="bar safe-bottom fixed inset-x-3 z-40 grid grid-cols-5 rounded-full border p-1.5 shadow-lift lg:hidden">
        {nav.filter((n) => n.mobile).map((n) => (
          <NavLink key={n.to} to={n.to} end={n.to === '/'}
            className={({ isActive }) => cx('flex flex-col items-center gap-0.5 rounded-full py-2 text-[10px] font-semibold transition',
              isActive ? 'bg-surface-2 text-brand-strong' : 'text-fg-2')}>
            <n.icon aria-hidden className="size-5" />
            {n.label}
          </NavLink>
        ))}
      </nav>
    </div>
  )
}

/** Mobile: everything that does not fit the bottom bar. */
export function MorePage() {
  return (
    <div>
      <h1 data-reveal className="mb-6 text-[28px] font-bold tracking-tight">Mais</h1>
      <ul data-reveal className="glass divide-y divide-line overflow-hidden rounded-[28px]">
        {nav.filter((n) => !n.mobile).map((n) => (
          <li key={n.to}>
            <NavLink to={n.to} className="flex items-center gap-3 px-5 py-4 text-sm font-medium text-fg transition hover:bg-surface-2">
              <span className="grid size-9 place-items-center rounded-xl bg-surface-2 text-fg-2"><n.icon aria-hidden className="size-[18px]" /></span>
              {n.label}
              <span aria-hidden className="ml-auto text-subtle">›</span>
            </NavLink>
          </li>
        ))}
        <li className="px-5 py-4"><ThemeSwitch /></li>
        <li>
          <button onClick={() => logout()} className="flex w-full items-center gap-3 px-5 py-4 text-left text-sm font-medium text-danger hover:bg-danger-soft">
            <span className="grid size-9 place-items-center rounded-xl bg-danger-soft"><LogOut className="size-[18px]" /></span> Sair
          </button>
        </li>
      </ul>
    </div>
  )
}
