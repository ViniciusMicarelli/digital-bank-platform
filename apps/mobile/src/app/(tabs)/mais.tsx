import { useQuery } from '@tanstack/react-query'
import { router } from 'expo-router'
import { Text } from 'react-native'
import { Button, Card, Row, Screen } from '../../components/ui'
import { api, client } from '../../lib/client'
import { useAccount, useProfile } from '../../lib/queries'
import { colors } from '../../lib/theme'

export default function More() {
  const profile = useProfile().data
  const account = useAccount()
  const unread = useQuery({
    queryKey: ['notifications', 'unread'],
    queryFn: async () => (await api<{ unread_count: number }>('GET', '/v1/notifications?unread=true&limit=1')).unread_count,
  }).data ?? 0
  const chevron = <Text style={{ color: colors.muted, fontSize: 18 }}>›</Text>
  return (
    <Screen title="Mais">
      <Card>
        <Row title="Notificações" subtitle={unread ? `${unread} não lidas` : 'Tudo lido'} right={chevron} onPress={() => router.push('/notificacoes')} />
        <Row title="Boletos" subtitle="Pagar e agendar contas" right={chevron} onPress={() => router.push('/boletos')} />
        <Row title="Crédito" subtitle="Limite e faturas" right={chevron} onPress={() => router.push('/credito')} />
      </Card>
      {profile && (
        <Card title="Meus dados">
          <Row title={profile.full_name} subtitle={`CPF ${profile.cpf}`} />
          {account && <Row title={`Ag. ${account.branch} · Conta ${account.number}`} subtitle="Conta corrente" />}
        </Card>
      )}
      <Button title="Sair" variant="danger" onPress={() => client.logout()} />
    </Screen>
  )
}
