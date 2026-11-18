import type { Notification } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeftRight, Bell, CheckCheck, CreditCard, FileText, ShieldCheck, Wallet, type LucideIcon } from 'lucide-react-native'
import { View } from 'react-native'
import { Button, Card, IconTile, Loading, Muted, Screen, Toggle, Txt, type Tone } from '../components/ui'
import { api } from '../lib/client'
import { dateTime } from '../lib/queries'
import { font, useTheme } from '../lib/theme'

const icons: Record<string, [LucideIcon, Tone]> = {
  PIX: [ArrowLeftRight, 'brand'], TRANSFER: [ArrowLeftRight, 'brand'], CARD: [CreditCard, 'slate'], CREDIT: [Wallet, 'credit'],
  BOLETO: [FileText, 'amber'], ACCOUNT: [ShieldCheck, 'slate'], PROFILE: [ShieldCheck, 'green'],
}

export default function Notifications() {
  const { c } = useTheme()
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['notifications', 'list'], queryFn: () => api<{ notifications: Notification[]; unread_count: number }>('GET', '/v1/notifications?limit=50') })
  const prefs = useQuery({ queryKey: ['notifications', 'prefs'], queryFn: () => api<{ push: boolean; email: boolean }>('GET', '/v1/notifications/preferences') })
  const readAll = useMutation({ mutationFn: () => api('POST', '/v1/notifications/read', { body: {} }), onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications'] }) })
  const save = useMutation({ mutationFn: (p: { push: boolean; email: boolean }) => api('PUT', '/v1/notifications/preferences', { body: p }), onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications', 'prefs'] }) })
  return (
    <Screen>
      <Card title={list.data ? `${list.data.unread_count} não lidas` : 'Notificações'} icon={Bell}>
        {list.isLoading ? <Loading /> : !list.data?.notifications.length ? <Muted>Nenhuma notificação ainda.</Muted> : list.data.notifications.map((n) => {
          const [Icon, tone] = icons[n.category] ?? [Bell, 'slate']
          return (
            <View key={n.id} style={{ flexDirection: 'row', gap: 12, paddingVertical: 10 }}>
              <IconTile icon={Icon} tone={tone} />
              <View style={{ flex: 1 }}>
                <View style={{ flexDirection: 'row', alignItems: 'center', gap: 6 }}>
                  <Txt variant="label" style={{ fontFamily: n.read ? font.medium : font.bold, flex: 1 }}>{n.title}</Txt>
                  {!n.read && <View style={{ width: 8, height: 8, borderRadius: 4, backgroundColor: c.brand }} />}
                </View>
                <Txt variant="label" tone="fg2" style={{ marginTop: 2 }}>{n.body}</Txt>
                <Txt variant="caption" tone="subtle" style={{ marginTop: 2 }}>{dateTime(n.occurred_at)}</Txt>
              </View>
            </View>
          )
        })}
        {!!list.data?.unread_count && <Button title="Marcar todas como lidas" icon={CheckCheck} variant="ghost" loading={readAll.isPending} onPress={() => readAll.mutate()} />}
      </Card>
      {prefs.data && (
        <Card title="Como avisar você" index={1}>
          <Toggle label="Push no celular" value={prefs.data.push} onChange={(push) => save.mutate({ ...prefs.data!, push })} />
          <Toggle label="E-mail" description="Só o importante: falhas, faturas, segurança." value={prefs.data.email} onChange={(email) => save.mutate({ ...prefs.data!, email })} />
        </Card>
      )}
    </Screen>
  )
}
