import { useQuery } from '@tanstack/react-query'
import { NavLink, Outlet } from 'react-router'
import { useLiveConnected } from '../app/contexts'
import { useAccount } from '../app/queries'
import { api, logout } from '../lib/api'
import { } from './ui'
import { cx } from '../lib/format'

const nav = [
  { to: '/', label: 'Início', icon: '⌂', mobile: true },
  { to: '/extrato', label: 'Extrato', icon: '≡', mobile: true },
  { to: '/pix', label: 'Pix', icon: '◇', mobile: true },
  { to: '/cartoes', label: 'Cartões', icon: '▭', mobile: true },
  { to: '/credito', label: 'Crédito', icon: '◔', mobile: false },
  { to: '/boletos', label: 'Boletos', icon: '▥', mobile: false },
  { to: '/notificacoes', label: 'Notificações', icon: '◉', mobile: false },
  { to: '/ajustes', label: 'Ajustes', icon: '⚙', mobile: false },
  { to: '/mais', label: 'Mais', icon: '⋯', mobile: true, desktop: false },
]

export function Brand() {
  return (
    <span className="flex items-center gap-2 text-lg font-bold tracking-tight">
      <span className="grid size-8 place-items-center rounded-lg bg-brand-600 text-white">D</span>
      Digital Bank
    </span>
  )
}

function useUnread() {
  return useQuery({
    queryKey: ['notifications', 'unread'],
    queryFn: async () => (await api<{ unread_count: number }>('GET', '/v1/notifications?unread=true&limit=1')).unread_count,
  }).data ?? 0
}

export function Layout() {
  const account = useAccount()
  const unread = useUnread()
  const live = useLiveConnected()
  return (
    <div className="min-h-full lg:flex">
      <aside className="hidden w-64 shrink-0 flex-col border-r border-slate-200 bg-white p-5 lg:flex">
        <Brand />
        <nav className="mt-8 flex flex-1 flex-col gap-1">
          {nav.filter((n) => n.desktop !== false).map((n) => (
            <NavLink key={n.to} to={n.to} end={n.to === '/'}
              className={({ isActive }) => cx('flex items-center gap-3 rounded-xl px-3 py-2 text-sm font-medium transition',
                isActive ? 'bg-brand-50 text-brand-700' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900')}>
              <span aria-hidden className="w-5 text-center">{n.icon}</span>
              {n.label}
              {n.to === '/notificacoes' && unread > 0 && (
                <span className="ml-auto rounded-full bg-brand-600 px-2 text-xs text-white">{unread}</span>
              )}
            </NavLink>
          ))}
        </nav>
        {account && (
          <div className="rounded-xl bg-slate-50 p-3 text-xs text-slate-600">
            <p className="font-semibold text-slate-900">{account.holder_name}</p>
            <p>Ag. {account.branch} · Conta {account.number}</p>
          </div>
        )}
        <button onClick={() => logout()} className="mt-3 rounded-xl px-3 py-2 text-left text-sm text-slate-500 hover:bg-slate-50">
          Sair
        </button>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3 lg:hidden">
          <Brand />
          <NavLink to="/notificacoes" className="relative rounded-lg p-2 text-slate-600" aria-label="Notificações">
            ◉{unread > 0 && <span className="absolute -top-0.5 -right-0.5 rounded-full bg-brand-600 px-1.5 text-[10px] text-white">{unread}</span>}
          </NavLink>
        </header>
        <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-6 pb-24 lg:px-8 lg:py-10">
          <Outlet />
        </main>
        <p className="hidden px-8 pb-4 text-xs text-slate-400 lg:block">
          <span className={cx('mr-1 inline-block size-2 rounded-full', live ? 'bg-emerald-500' : 'bg-slate-300')} />
          {live ? 'Atualizações em tempo real' : 'Reconectando…'}
        </p>
      </div>

      <nav className="fixed inset-x-0 bottom-0 z-40 grid grid-cols-5 border-t border-slate-200 bg-white lg:hidden">
        {nav.filter((n) => n.mobile).map((n) => (
          <NavLink key={n.to} to={n.to} end={n.to === '/'}
            className={({ isActive }) => cx('flex flex-col items-center gap-0.5 py-2 text-[11px] font-medium', isActive ? 'text-brand-700' : 'text-slate-500')}>
            <span aria-hidden className="text-lg leading-none">{n.icon}</span>
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
      <h1 className="mb-6 text-2xl font-bold tracking-tight">Mais</h1>
      <ul className="divide-y divide-slate-100 rounded-2xl bg-white shadow-sm ring-1 ring-slate-100">
        {nav.filter((n) => !n.mobile).map((n) => (
          <li key={n.to}>
            <NavLink to={n.to} className="flex items-center gap-3 px-5 py-4 text-sm font-medium">
              <span aria-hidden className="w-5 text-center text-brand-700">{n.icon}</span>{n.label}
              <span aria-hidden className="ml-auto text-slate-400">›</span>
            </NavLink>
          </li>
        ))}
        <li><button onClick={() => logout()} className="w-full px-5 py-4 text-left text-sm font-medium text-red-700">Sair</button></li>
      </ul>
    </div>
  )
}
