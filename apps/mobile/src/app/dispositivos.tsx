import { errorMessage, lastSeen, type Session } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { LogOut, MonitorSmartphone } from 'lucide-react-native'
import { useState } from 'react'
import { View } from 'react-native'
import { Button, Card, Loading, Muted, Notice, Screen, Txt } from '../components/ui'
import { api, client } from '../lib/client'
import { font, useTheme } from '../lib/theme'

/** Where the customer is signed in; ends any other session, or all of them. */
export default function Devices() {
  const { c } = useTheme()
  const qc = useQueryClient()
  const [error, setError] = useState('')
  const sessions = useQuery({ queryKey: ['sessions'], queryFn: async () => (await api<{ sessions: Session[] }>('GET', '/v1/me/sessions')).sessions })
  const done = { onSuccess: () => { setError(''); qc.invalidateQueries({ queryKey: ['sessions'] }) }, onError: (e: unknown) => setError(errorMessage(e)) }
  const end = useMutation({ mutationFn: (id: string) => api('DELETE', `/v1/me/sessions/${id}`), ...done })
  const endOthers = useMutation({ mutationFn: () => api('POST', '/v1/me/sessions/end-others'), ...done })
  if (sessions.isLoading) return <Screen><Loading /></Screen>
  const list = sessions.data ?? []
  const others = list.filter((s) => !s.current)
  return (
    <Screen>
      <Muted>Onde sua conta está aberta. Não reconhece algum? Encerre o acesso e troque sua senha.</Muted>
      {list.map((s, i) => (
        <Card key={s.id} index={i}>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
            <MonitorSmartphone size={20} color={c.muted} />
            <View style={{ flex: 1, gap: 2 }}>
              <Txt style={{ fontFamily: font.semibold }}>{s.device}{s.current ? ' · este dispositivo' : ''}</Txt>
              <Txt variant="caption" tone="muted">{s.current ? 'Em uso agora' : `Último acesso ${lastSeen(s.last_used_at)}`} · IP {s.ip || '—'}</Txt>
            </View>
          </View>
          {s.current
            ? <Button title="Sair" icon={LogOut} variant="ghost" onPress={() => client.logout()} />
            : <Button title="Encerrar" variant="ghost" loading={end.isPending && end.variables === s.id} onPress={() => end.mutate(s.id)} />}
        </Card>
      ))}
      <Notice>{error}</Notice>
      {others.length > 0 && (
        <Button title={others.length === 1 ? 'Encerrar o outro acesso' : `Encerrar os outros ${others.length} acessos`} variant="danger"
          loading={endOthers.isPending} onPress={() => endOthers.mutate()} />
      )}
    </Screen>
  )
}
