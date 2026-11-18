import { addBRL, formatBRL, type FeedItem } from '@bank/core'
import { useQueryClient } from '@tanstack/react-query'
import { LinearGradient } from 'expo-linear-gradient'
import { router } from 'expo-router'
import {
  ArrowDownLeft, ArrowLeftRight, ArrowUpRight, Barcode, Bell, Eye, EyeOff, FileText, QrCode, ShoppingBag, Undo2, Wallet, type LucideIcon,
} from 'lucide-react-native'
import { useState } from 'react'
import { RefreshControl, StyleSheet, View } from 'react-native'
import Animated, { FadeInDown } from 'react-native-reanimated'
import { Amount, Card, IconTile, Loading, Muted, Press, Row, Screen, Txt } from '../../components/ui'
import { useCountUp } from '../../lib/motion'
import { dateTime, typeLabels, useAccount, useBalance, useStatement } from '../../lib/queries'
import { font, heroGradient, useTheme } from '../../lib/theme'

const typeIcons: Record<string, LucideIcon> = {
  PIX: ArrowLeftRight, PIX_OUT: ArrowUpRight, PIX_IN: ArrowDownLeft, PIX_REFUND: Undo2, INTERNAL_TRANSFER: ArrowLeftRight,
  DEBIT_CARD_PURCHASE: ShoppingBag, CREDIT_INVOICE_PAYMENT: Wallet, BOLETO_PAYMENT: FileText,
}

function FeedRow({ item }: { item: FeedItem }) {
  const credit = item.direction === 'CREDIT'
  return (
    <Row icon={credit ? ArrowDownLeft : (typeIcons[item.type] ?? ArrowUpRight)} tone={credit ? 'green' : 'slate'}
      title={item.counterparty_name || item.description || typeLabels[item.type] || item.type}
      subtitle={`${typeLabels[item.type] ?? item.type} · ${dateTime(item.occurred_at)}`}
      right={<Amount value={item.amount} direction={item.direction} />} />
  )
}

const actions: [string, LucideIcon, '/pix' | '/boletos' | '/cartoes' | '/credito'][] = [
  ['Pix', ArrowLeftRight, '/pix'], ['Cobrar', QrCode, '/pix'], ['Boleto', Barcode, '/boletos'], ['Crédito', Wallet, '/credito'],
]

export default function Home() {
  const { c } = useTheme()
  const qc = useQueryClient()
  const account = useAccount()
  const balance = useBalance(account?.id)
  const feed = useStatement(account?.id, 30)
  const [hidden, setHidden] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const shown = useCountUp(Number(balance.data?.available.amount ?? 0))
  const [month] = useState(() => new Date().toISOString().slice(0, 7))
  const items = feed.data ?? []
  const monthItems = items.filter((i) => i.occurred_at.startsWith(month))
  const sum = (d: FeedItem['direction']) => addBRL('0.00', ...monthItems.filter((i) => i.direction === d).map((i) => i.amount.amount))
  const [hour] = useState(() => new Date().getHours())
  const onRefresh = async () => {
    setRefreshing(true)
    await qc.invalidateQueries()
    setRefreshing(false)
  }
  const exact = balance.data?.available.amount
  const display = exact && Math.abs(shown - Number(exact)) < 0.005 ? formatBRL(exact) : formatBRL(shown.toFixed(2))
  return (
    <Screen refreshing={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={c.brand} />}>
      <Animated.View entering={FadeInDown.duration(400)} style={[st.between]}>
        <View>
          <Muted>{hour < 12 ? 'Bom dia' : hour < 18 ? 'Boa tarde' : 'Boa noite'},</Muted>
          <Txt variant="title">{account?.holder_name.split(' ')[0]}</Txt>
        </View>
        <Press accessibilityLabel="Notificações" onPress={() => router.push('/notificacoes')} style={[st.round, { backgroundColor: c.surface, borderColor: c.line }]}>
          <Bell size={20} color={c.fg2} />
        </Press>
      </Animated.View>

      <Animated.View entering={FadeInDown.delay(80).duration(500)}>
        <LinearGradient colors={[...heroGradient]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={st.hero}>
          <View style={[st.glow, { backgroundColor: '#2ed3a0' }]} />
          <View style={[st.glow2, { backgroundColor: '#7a5af8' }]} />
          <View style={st.between}>
            <Txt variant="label" style={{ color: '#ffffffb3' }}>Saldo disponível</Txt>
            <Press accessibilityLabel={hidden ? 'Mostrar saldo' : 'Ocultar saldo'} onPress={() => setHidden(!hidden)} style={st.eye}>
              {hidden ? <EyeOff size={16} color="#fff" /> : <Eye size={16} color="#fff" />}
            </Press>
          </View>
          <Txt variant="display" tone="white" accessibilityLabel="Saldo" style={{ fontVariant: ['tabular-nums'] }}>
            {balance.isLoading ? '…' : hidden ? 'R$ ••••••' : display}
          </Txt>
          <View style={{ flexDirection: 'row', gap: 10 }}>
            <View style={st.stat}>
              <View style={st.row}><ArrowDownLeft size={13} color="#2ed3a0" /><Txt variant="caption" style={{ color: '#ffffff99' }}>Entradas no mês</Txt></View>
              <Txt tone="white" style={{ fontFamily: font.semibold }}>{hidden ? '••••' : formatBRL(sum('CREDIT'))}</Txt>
            </View>
            <View style={st.stat}>
              <View style={st.row}><ArrowUpRight size={13} color="#ffffffb3" /><Txt variant="caption" style={{ color: '#ffffff99' }}>Saídas no mês</Txt></View>
              <Txt tone="white" style={{ fontFamily: font.semibold }}>{hidden ? '••••' : formatBRL(sum('DEBIT'))}</Txt>
            </View>
          </View>
          <Txt variant="caption" style={{ color: '#ffffff73' }}>Agência {account?.branch} · Conta {account?.number}</Txt>
        </LinearGradient>
      </Animated.View>

      <Animated.View entering={FadeInDown.delay(160).duration(500)} style={{ flexDirection: 'row', gap: 10 }}>
        {actions.map(([label, Icon, href]) => (
          <Press key={label} accessibilityLabel={label} onPress={() => router.push(href)} style={[st.action, { backgroundColor: c.surface, borderColor: c.line }]}>
            <IconTile icon={Icon} tone="brand" size={44} />
            <Txt variant="caption" style={{ fontFamily: font.semibold, color: c.fg }}>{label}</Txt>
          </Press>
        ))}
      </Animated.View>

      <Card title="Movimentações" index={3}>
        {feed.isLoading ? <Loading /> : !items.length ? <Muted>Nenhuma movimentação ainda. Cadastre uma chave Pix e receba.</Muted> :
          items.slice(0, 12).map((i) => <FeedRow key={i.id} item={i} />)}
      </Card>
    </Screen>
  )
}

const st = StyleSheet.create({
  between: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  row: { flexDirection: 'row', alignItems: 'center', gap: 5 },
  round: { width: 44, height: 44, borderRadius: 22, alignItems: 'center', justifyContent: 'center', borderWidth: 1 },
  hero: { borderRadius: 28, padding: 22, gap: 14, overflow: 'hidden' },
  glow: { position: 'absolute', width: 260, height: 260, borderRadius: 130, top: -130, right: -90, opacity: 0.3 },
  glow2: { position: 'absolute', width: 200, height: 200, borderRadius: 100, bottom: -120, left: -60, opacity: 0.25 },
  eye: { width: 34, height: 34, borderRadius: 11, backgroundColor: '#ffffff1a', alignItems: 'center', justifyContent: 'center' },
  stat: { flex: 1, borderRadius: 16, padding: 12, gap: 4, backgroundColor: '#ffffff12', borderWidth: 1, borderColor: '#ffffff1a' },
  action: { flex: 1, alignItems: 'center', gap: 8, paddingVertical: 14, borderRadius: 20, borderWidth: 1 },
})
