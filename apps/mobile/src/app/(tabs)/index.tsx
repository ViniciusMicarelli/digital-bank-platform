import { formatBRL } from '@bank/core'
import { useQueryClient } from '@tanstack/react-query'
import { router } from 'expo-router'
import { useState } from 'react'
import { Pressable, RefreshControl, StyleSheet, Text, View } from 'react-native'
import { Amount, Card, Loading, Muted, Row, Screen } from '../../components/ui'
import { dateTime, typeLabels, useAccount, useBalance, useStatement } from '../../lib/queries'
import { colors } from '../../lib/theme'

export default function Home() {
  const qc = useQueryClient()
  const account = useAccount()
  const balance = useBalance(account?.id)
  const feed = useStatement(account?.id, 20)
  const [hidden, setHidden] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const onRefresh = async () => {
    setRefreshing(true)
    await qc.invalidateQueries()
    setRefreshing(false)
  }
  return (
    <Screen refreshing={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} />}>
      <Muted>Olá,</Muted>
      <Text style={st.hello}>{account?.holder_name.split(' ')[0]}</Text>
      <View style={st.balance}>
        <View style={st.balanceTop}>
          <Text style={st.balanceLabel}>Saldo disponível</Text>
          <Pressable onPress={() => setHidden(!hidden)} hitSlop={10}><Text style={st.balanceLabel}>{hidden ? 'Mostrar' : 'Ocultar'}</Text></Pressable>
        </View>
        <Text style={st.balanceValue} accessibilityLabel="Saldo">{balance.isLoading ? '…' : hidden ? 'R$ •••••' : formatBRL(balance.data?.available)}</Text>
        <Text style={st.balanceLabel}>Ag. {account?.branch} · Conta {account?.number}</Text>
      </View>
      <View style={st.actions}>
        {([['Pix', '/pix'], ['Boleto', '/boletos'], ['Cartões', '/cartoes'], ['Crédito', '/credito']] as const).map(([label, href]) => (
          <Pressable key={label} onPress={() => router.push(href)} style={st.action} accessibilityRole="button">
            <Text style={st.actionText}>{label}</Text>
          </Pressable>
        ))}
      </View>
      <Card title="Movimentações">
        {feed.isLoading ? <Loading /> : !feed.data?.length ? <Muted>Nenhuma movimentação ainda. Cadastre uma chave Pix e receba.</Muted> :
          feed.data.map((i) => (
            <Row key={i.id} title={i.counterparty_name || i.description || typeLabels[i.type] || i.type}
              subtitle={`${typeLabels[i.type] ?? i.type} · ${dateTime(i.occurred_at)}`}
              right={<Amount value={i.amount} direction={i.direction} />} />
          ))}
      </Card>
    </Screen>
  )
}

const st = StyleSheet.create({
  hello: { fontSize: 26, fontWeight: '700', color: colors.text, marginTop: -10 },
  balance: { backgroundColor: colors.brandDark, borderRadius: 22, padding: 20, gap: 8 },
  balanceTop: { flexDirection: 'row', justifyContent: 'space-between' },
  balanceLabel: { color: '#c7d2fe', fontSize: 13 },
  balanceValue: { color: '#fff', fontSize: 34, fontWeight: '700', fontVariant: ['tabular-nums'] },
  actions: { flexDirection: 'row', gap: 10 },
  action: { flex: 1, backgroundColor: colors.card, borderRadius: 16, paddingVertical: 16, alignItems: 'center', borderWidth: 1, borderColor: '#f1f5f9' },
  actionText: { fontWeight: '600', color: colors.brand },
})
