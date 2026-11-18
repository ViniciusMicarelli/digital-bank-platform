import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { ensureAccess } from '../lib/api'
import { openStream } from '@bank/core'
import type { Notification } from '@bank/core'
import { LiveContext } from './contexts'

interface Toast extends Notification {
  key: number
}

/** Streams notifications: shows them as toasts and refreshes what changed. */
export function LiveNotifications({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const [toasts, setToasts] = useState<Toast[]>([])
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
        {toasts.map((t) => (
          <div key={t.key} className="pointer-events-auto w-full max-w-sm rounded-2xl bg-slate-900 px-4 py-3 text-white shadow-lg">
            <p className="text-sm font-semibold">{t.title}</p>
            <p className="text-sm text-slate-300">{t.body}</p>
          </div>
        ))}
      </div>
    </LiveContext.Provider>
  )
}
