import type { Notification } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Text, View } from 'react-native'
import { Button, Card, Loading, Muted, Screen, Toggle } from '../components/ui'
import { api } from '../lib/client'
import { dateTime } from '../lib/queries'
import { colors } from '../lib/theme'

export default function Notifications() {
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['notifications', 'list'], queryFn: () => api<{ notifications: Notification[]; unread_count: number }>('GET', '/v1/notifications?limit=50') })
  const prefs = useQuery({ queryKey: ['notifications', 'prefs'], queryFn: () => api<{ push: boolean; email: boolean }>('GET', '/v1/notifications/preferences') })
  const readAll = useMutation({ mutationFn: () => api('POST', '/v1/notifications/read', { body: {} }), onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications'] }) })
  const save = useMutation({ mutationFn: (p: { push: boolean; email: boolean }) => api('PUT', '/v1/notifications/preferences', { body: p }), onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications', 'prefs'] }) })
  return (
    <Screen>
      {prefs.data && (
        <Card title="Como avisar você">
          <Toggle label="Push no celular" value={prefs.data.push} onChange={(push) => save.mutate({ ...prefs.data!, push })} />
          <Toggle label="E-mail (só o importante)" value={prefs.data.email} onChange={(email) => save.mutate({ ...prefs.data!, email })} />
        </Card>
      )}
      <Card title={list.data ? `${list.data.unread_count} não lidas` : 'Notificações'}>
        {list.isLoading ? <Loading /> : !list.data?.notifications.length ? <Muted>Nenhuma notificação ainda.</Muted> : list.data.notifications.map((n) => (
          <View key={n.id} style={{ flexDirection: 'row', gap: 10, paddingVertical: 8 }}>
            <View style={{ width: 8, height: 8, borderRadius: 4, marginTop: 7, backgroundColor: n.read ? 'transparent' : colors.brand }} />
            <View style={{ flex: 1 }}>
              <Text style={{ fontWeight: n.read ? '500' : '700', color: colors.text }}>{n.title}</Text>
              <Text style={{ color: colors.muted }}>{n.body}</Text>
              <Text style={{ color: '#94a3b8', fontSize: 12 }}>{dateTime(n.occurred_at)}</Text>
            </View>
          </View>
        ))}
        {!!list.data?.unread_count && <Button title="Marcar todas como lidas" variant="ghost" loading={readAll.isPending} onPress={() => readAll.mutate()} />}
      </Card>
    </Screen>
  )
}
