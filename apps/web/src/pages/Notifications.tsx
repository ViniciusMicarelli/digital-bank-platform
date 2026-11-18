import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useLiveConnected } from '../app/contexts'
import { ArrowLeftRight, Bell, CheckCheck, CreditCard, FileText, ShieldCheck, Wallet, type LucideIcon } from 'lucide-react'
import { Badge, Button, Empty, IconTile, Loading, PageTitle, Panel, Toggle } from '../components/ui'
import { cx, dateTimeBR } from '../lib/format'
import { api } from '../lib/api'
import type { Notification } from '@bank/core'

const categoryIcons: Record<string, [LucideIcon, 'brand' | 'credit' | 'amber' | 'slate' | 'green']> = {
  PIX: [ArrowLeftRight, 'brand'], TRANSFER: [ArrowLeftRight, 'brand'], CARD: [CreditCard, 'slate'], CREDIT: [Wallet, 'credit'],
  BOLETO: [FileText, 'amber'], ACCOUNT: [ShieldCheck, 'slate'], PROFILE: [ShieldCheck, 'green'],
}

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
          action={list.data?.unread_count ? <Button variant="ghost" size="sm" icon={CheckCheck} loading={readAll.isPending} onClick={() => readAll.mutate()}>Marcar todas como lidas</Button> : undefined}>
          {list.isLoading ? <Loading /> : !list.data?.notifications.length ? <Empty icon={Bell}>Nenhuma notificação ainda.</Empty> : (
            <ul className="-my-1 divide-y divide-line">
              {list.data.notifications.map((n) => {
                const [Icon, tone] = categoryIcons[n.category] ?? [Bell, 'slate']
                return (
                  <li key={n.id} className="flex gap-3.5 py-3.5">
                    <IconTile icon={Icon} tone={tone} />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <p className={cx('text-sm text-fg', n.read ? 'font-medium' : 'font-semibold')}>{n.title}</p>
                        <Badge>{categories[n.category] ?? n.category}</Badge>
                        {!n.read && <span aria-label="não lida" className="ml-auto size-2 shrink-0 rounded-full bg-brand" />}
                      </div>
                      <p className="mt-0.5 text-sm text-fg-2">{n.body}</p>
                      <p className="mt-1 text-xs text-subtle">{dateTimeBR(n.occurred_at)}</p>
                    </div>
                  </li>
                )
              })}
            </ul>
          )}
        </Panel>
      </div>
      <div className="lg:pt-16">
        <Panel title="Como avisar você" icon={Bell}>
          {prefs.data && (
            <div className="divide-y divide-line">
              <Toggle label="Push no celular" checked={prefs.data.push} onChange={(push) => save.mutate({ ...prefs.data!, push })} />
              <Toggle label="E-mail" description="Só o importante: falhas, faturas, segurança." checked={prefs.data.email}
                onChange={(email) => save.mutate({ ...prefs.data!, email })} />
            </div>
          )}
          <p className="mt-3 text-xs text-muted">No app você sempre recebe tudo.</p>
        </Panel>
      </div>
    </div>
  )
}
