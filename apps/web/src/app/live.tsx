import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeftRight, Bell, CreditCard, FileText, ShieldCheck, Wallet, type LucideIcon } from 'lucide-react'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { gsap, reducedMotion, useGSAP } from '../lib/motion'
import { ensureAccess } from '../lib/api'
import { openStream, type Notification } from '@bank/core'
import { LiveContext } from './contexts'

interface ToastItem extends Notification {
  key: number
}

const icons: Record<string, LucideIcon> = {
  PIX: ArrowLeftRight, TRANSFER: ArrowLeftRight, CARD: CreditCard, CREDIT: Wallet, BOLETO: FileText, ACCOUNT: ShieldCheck, PROFILE: ShieldCheck,
}

function Toast({ t }: { t: ToastItem }) {
  const el = useRef<HTMLDivElement>(null)
  useGSAP(() => {
    if (!reducedMotion()) gsap.from(el.current, { x: 40, autoAlpha: 0, scale: 0.96, duration: 0.5, ease: 'back.out(1.6)' })
  })
  const Icon = icons[t.category] ?? Bell
  return (
    <div ref={el} className="pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-2xl bg-ink/95 p-4 text-white shadow-lift ring-1 ring-white/10 backdrop-blur-xl">
      <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-[#2ed3a0]/15 text-[#2ed3a0]"><Icon className="size-[18px]" /></span>
      <div className="min-w-0">
        <p className="text-sm font-semibold">{t.title}</p>
        <p className="text-sm text-white/70">{t.body}</p>
      </div>
    </div>
  )
}

/** Streams notifications: shows them as toasts and refreshes what changed. */
export function LiveNotifications({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    let n = 0
    return openStream({
      url: '/v1/notifications/stream',
      token: ensureAccess,
      onStatus: setConnected,
      onEvent: (ev) => {
        if (ev.event !== 'notification') return
        const notif = JSON.parse(ev.data) as Notification
        const key = ++n
        setToasts((t) => [...t.slice(-2), { ...notif, key }])
        setTimeout(() => setToasts((t) => t.filter((x) => x.key !== key)), 6000)
        // Money moved somewhere: refresh balances, statements and lists.
        for (const k of ['notifications', 'balance', 'statement', 'pix', 'cards', 'credit', 'invoices', 'boletos', 'profile', 'accounts']) {
          qc.invalidateQueries({ queryKey: [k] })
        }
        // The statement is a projection built from the same events by another
        // service; it may land a moment after the notification. Look again.
        setTimeout(() => qc.invalidateQueries({ queryKey: ['statement'] }), 1500)
      },
    })
  }, [qc])

  return (
    <LiveContext.Provider value={connected}>
      {children}
      <div aria-live="polite" className="pointer-events-none fixed inset-x-0 top-4 z-50 flex flex-col items-center gap-2 px-4 sm:items-end sm:pr-6">
        {toasts.map((t) => <Toast key={t.key} t={t} />)}
      </div>
    </LiveContext.Provider>
  )
}
