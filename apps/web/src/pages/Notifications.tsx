import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLiveConnected } from '../app/contexts'
import {Badge, Button, Empty, Loading, PageTitle, Panel, Toggle } from '../components/ui'
import { cx, dateTimeBR } from '../lib/format'
import { api } from '../lib/api'
import type { Notification } from '@bank/core'

const categories: Record<string, string> = {
  PIX: 'Pix', TRANSFER: 'Transferência', CARD: 'Cartão', CREDIT: 'Crédito', BOLETO: 'Boleto', ACCOUNT: 'Conta', PROFILE: 'Cadastro',
}

export function NotificationsPage() {
  const qc = useQueryClient()
  const live = useLiveConnected()
  const list = useQuery({
    queryKey: ['notifications', 'list'],
    queryFn: () => api<{ notifications: Notification[]; unread_count: number }>('GET', '/v1/notifications?limit=50'),
  })
  const prefs = useQuery({ queryKey: ['notifications', 'prefs'], queryFn: () => api<{ push: boolean; email: boolean }>('GET', '/v1/notifications/preferences') })
  const readAll = useMutation({ mutationFn: () => api('POST', '/v1/notifications/read', { body: {} }), onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications'] }) })
  const save = useMutation({
    mutationFn: (p: { push: boolean; email: boolean }) => api('PUT', '/v1/notifications/preferences', { body: p }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications', 'prefs'] }),
  })
  return (
    <div className="grid gap-6 lg:grid-cols-3">
      <div className="lg:col-span-2">
        <PageTitle subtitle={live ? 'Chegam aqui na hora em que acontecem.' : 'Reconectando…'}>Notificações</PageTitle>
        <Panel title={list.data ? `${list.data.unread_count} não lidas` : undefined}
          action={list.data?.unread_count ? <Button variant="ghost" loading={readAll.isPending} onClick={() => readAll.mutate()}>Marcar todas como lidas</Button> : undefined}>
          {list.isLoading ? <Loading /> : !list.data?.notifications.length ? <Empty>Nenhuma notificação ainda.</Empty> : (
            <ul className="divide-y divide-slate-100">
              {list.data.notifications.map((n) => (
                <li key={n.id} className="flex gap-3 py-3">
                  <span className={cx('mt-1.5 size-2 shrink-0 rounded-full', n.read ? 'bg-transparent' : 'bg-brand-600')} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className={cx('text-sm', n.read ? 'font-medium' : 'font-semibold')}>{n.title}</p>
                      <Badge>{categories[n.category] ?? n.category}</Badge>
                    </div>
                    <p className="text-sm text-slate-600">{n.body}</p>
                    <p className="mt-0.5 text-xs text-slate-400">{dateTimeBR(n.occurred_at)}</p>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>
      <div className="lg:pt-16">
        <Panel title="Como avisar você">
          {prefs.data && (
            <div className="divide-y divide-slate-100">
              <Toggle label="Push no celular" checked={prefs.data.push} onChange={(push) => save.mutate({ ...prefs.data!, push })} />
              <Toggle label="E-mail" description="Só o importante: falhas, faturas, segurança." checked={prefs.data.email}
                onChange={(email) => save.mutate({ ...prefs.data!, email })} />
            </div>
          )}
          <p className="mt-3 text-xs text-slate-500">No app você sempre recebe tudo.</p>
        </Panel>
      </div>
    </div>
  )
}
