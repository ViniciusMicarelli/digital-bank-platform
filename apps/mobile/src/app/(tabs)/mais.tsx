import { useQuery } from '@tanstack/react-query'
import { router } from 'expo-router'
import { Barcode, Bell, ChevronRight, LogOut, Palette, ShieldCheck, Wallet } from 'lucide-react-native'
import { View } from 'react-native'
import { ThemeSwitch } from '../../components/ThemeSwitch'
import { Button, Card, Row, Screen, Txt } from '../../components/ui'
import { api, client } from '../../lib/client'
import { useAccount, useProfile } from '../../lib/queries'
import { font, useTheme } from '../../lib/theme'

export default function More() {
  const { c } = useTheme()
  const profile = useProfile().data
  const account = useAccount()
  const unread = useQuery({
    queryKey: ['notifications', 'unread'],
    queryFn: async () => (await api<{ unread_count: number }>('GET', '/v1/notifications?unread=true&limit=1')).unread_count,
  }).data ?? 0
  const chevron = <ChevronRight size={18} color={c.subtle} />
  const initials = (profile?.full_name ?? '').split(' ').filter(Boolean)
  return (
    <Screen title="Mais">
      {profile && (
        <Card>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 14 }}>
            <View style={{ width: 56, height: 56, borderRadius: 28, backgroundColor: c.brand, alignItems: 'center', justifyContent: 'center' }}>
              <Txt style={{ color: c.brandFg, fontFamily: font.bold, fontSize: 18 }}>{(initials[0]?.[0] ?? '') + (initials.at(-1)?.[0] ?? '')}</Txt>
            </View>
            <View style={{ flex: 1 }}>
              <Txt variant="heading">{profile.full_name}</Txt>
              <Txt variant="caption" tone="muted">CPF {profile.cpf}{account ? ` · Conta ${account.number}` : ''}</Txt>
            </View>
            <ShieldCheck size={20} color={c.positive} />
          </View>
        </Card>
      )}
      <Card index={1}>
        <Row icon={Bell} tone="brand" title="Notificações" subtitle={unread ? `${unread} não lidas` : 'Tudo lido'} right={chevron} onPress={() => router.push('/notificacoes')} />
        <Row icon={Barcode} tone="amber" title="Boletos" subtitle="Pagar e agendar contas" right={chevron} onPress={() => router.push('/boletos')} />
        <Row icon={Wallet} tone="credit" title="Crédito" subtitle="Limite e faturas" right={chevron} onPress={() => router.push('/credito')} />
      </Card>
      <Card title="Aparência" icon={Palette} index={2}>
        <ThemeSwitch />
      </Card>
      <Button title="Sair" icon={LogOut} variant="danger" onPress={() => client.logout()} />
    </Screen>
  )
}
