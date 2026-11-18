import { addBRL, ApiError, formatBRL, type Card as CardT, type CreditLine, type FeedItem, type Invoice } from '@bank/core'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { router } from 'expo-router'
import {
  ArrowDownLeft, ArrowLeftRight, ArrowUpRight, Barcode, Bell, ChevronRight, CreditCard, Eye, EyeOff, FileText, QrCode, ShoppingBag, Sparkles,
  Undo2, Wallet, Zap, type LucideIcon,
} from 'lucide-react-native'
import { useState } from 'react'
import { RefreshControl, ScrollView, StyleSheet, View } from 'react-native'
import Animated, { FadeInDown } from 'react-native-reanimated'
import { Glass } from '../../components/Glass'
import { Amount, Card, IconTile, Loading, Muted, Press, Row, Screen, Txt } from '../../components/ui'
import { api } from '../../lib/client'
import { useCountUp } from '../../lib/motion'
import { dateTime, typeLabels, useAccount, useBalance, useStatement } from '../../lib/queries'
import { font, useTheme } from '../../lib/theme'

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

type Href = '/pix' | '/boletos' | '/cartoes' | '/credito'
const shortcuts: [string, LucideIcon, Href][] = [
  ['Área Pix', Zap, '/pix'], ['Pagar', Barcode, '/boletos'], ['Transferir', ArrowUpRight, '/pix'],
  ['Cobrar', QrCode, '/pix'], ['Crédito', Wallet, '/credito'], ['Cartões', CreditCard, '/cartoes'],
]

function Credit({ accountId, hidden }: { accountId: string; hidden: boolean }) {
  const { c } = useTheme()
  const line = useQuery({
    queryKey: ['credit', accountId],
    queryFn: async () => {
      try { return await api<CreditLine>('GET', `/v1/accounts/${accountId}/credit`) } catch (e) { if (e instanceof ApiError && e.status === 404) return null; throw e }
    },
  })
  const invoices = useQuery({
    queryKey: ['invoices', accountId], enabled: !!line.data,
    queryFn: async () => (await api<{ invoices: Invoice[] }>('GET', `/v1/accounts/${accountId}/credit/invoices`)).invoices,
  })
  if (line.isLoading) return null
  if (!line.data) {
    return (
      <Press accessibilityLabel="Ativar cartão de crédito" onPress={() => router.push('/credito')}>
        <Glass radius={28} style={st.promo}>
          <IconTile icon={Sparkles} tone="credit" size={46} />
          <View style={{ flex: 1 }}>
            <Txt variant="label" style={{ fontFamily: font.semibold }}>Ative seu cartão de crédito</Txt>
            <Txt variant="caption" tone="muted">Limite de R$ 1.000,00 no mesmo cartão.</Txt>
          </View>
          <ChevronRight size={18} color={c.subtle} />
        </Glass>
      </Press>
    )
  }
  const current = invoices.data?.find((i) => i.status === 'OPEN')
  const l = line.data
  return (
    <Press accessibilityLabel="Cartão de crédito" onPress={() => router.push('/credito')}>
      <Glass radius={28} style={{ padding: 20, gap: 4 }}>
        <View style={st.between}><Txt variant="heading">Cartão de crédito</Txt><ChevronRight size={18} color={c.subtle} /></View>
        <Txt variant="caption" tone="muted" style={{ marginTop: 6 }}>Fatura atual</Txt>
        <Txt variant="title" tone="credit" style={{ fontSize: 28 }}>{hidden ? 'R$ ••••' : formatBRL(current?.total ?? '0.00')}</Txt>
        <Txt variant="caption" tone="muted">Limite disponível de {hidden ? '••••' : formatBRL(l.available)}</Txt>
      </Glass>
    </Press>
  )
}

export default function Home() {
  const { c } = useTheme()
  const qc = useQueryClient()
  const account = useAccount()
  const balance = useBalance(account?.id)
  const feed = useStatement(account?.id, 30)
  const cards = useQuery({ queryKey: ['cards', account?.id], enabled: !!account, queryFn: async () => (await api<{ cards: CardT[] }>('GET', `/v1/accounts/${account!.id}/cards`)).cards })
  const [hidden, setHidden] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const shown = useCountUp(Number(balance.data?.available.amount ?? 0))
  const [month] = useState(() => new Date().toISOString().slice(0, 7))
  const [hour] = useState(() => new Date().getHours())
  const items = feed.data ?? []
  const monthItems = items.filter((i) => i.occurred_at.startsWith(month))
  const sum = (d: FeedItem['direction']) => addBRL('0.00', ...monthItems.filter((i) => i.direction === d).map((i) => i.amount.amount))
  const exact = balance.data?.available.amount
  const display = exact && Math.abs(shown - Number(exact)) < 0.005 ? formatBRL(exact) : formatBRL(shown.toFixed(2))
  const names = (account?.holder_name ?? '').split(' ')
  const onRefresh = async () => {
    setRefreshing(true)
    await qc.invalidateQueries()
    setRefreshing(false)
  }
  return (
    <Screen refreshing={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={c.brand} />}>
      <Animated.View entering={FadeInDown.duration(400)} style={st.between}>
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
          <View style={[st.avatar, { backgroundColor: c.brand }]}>
            <Txt style={{ color: c.brandFg, fontFamily: font.bold, fontSize: 16 }}>{(names[0]?.[0] ?? '') + (names.at(-1)?.[0] ?? '')}</Txt>
          </View>
          <View>
            <Txt variant="caption" tone="muted">{hour < 12 ? 'Bom dia' : hour < 18 ? 'Boa tarde' : 'Boa noite'},</Txt>
            <Txt variant="heading" style={{ fontSize: 22, fontFamily: font.bold }}>{names[0]}</Txt>
          </View>
        </View>
        <View style={{ flexDirection: 'row', gap: 10 }}>
          <Press accessibilityLabel={hidden ? 'Mostrar saldo' : 'Ocultar saldo'} onPress={() => setHidden(!hidden)}>
            <Glass radius={22} interactive style={st.round}>{hidden ? <EyeOff size={20} color={c.fg2} /> : <Eye size={20} color={c.fg2} />}</Glass>
          </Press>
          <Press accessibilityLabel="Notificações" onPress={() => router.push('/notificacoes')}>
            <Glass radius={22} interactive style={st.round}><Bell size={20} color={c.fg2} /></Glass>
          </Press>
        </View>
      </Animated.View>

      <Animated.View entering={FadeInDown.delay(80).duration(500)}>
        <Glass radius={30} style={{ padding: 22, gap: 4 }}>
          <View style={st.between}><Txt variant="heading">Conta</Txt><ChevronRight size={18} color={c.subtle} /></View>
          <Txt variant="caption" tone="muted" style={{ marginTop: 6 }}>Saldo disponível</Txt>
          <Txt variant="display" accessibilityLabel="Saldo" style={{ fontVariant: ['tabular-nums'] }}>
            {balance.isLoading ? '…' : hidden ? 'R$ ••••••' : display}
          </Txt>
          <View style={{ flexDirection: 'row', gap: 8, marginTop: 10, flexWrap: 'wrap' }}>
            <View style={[st.chip, { backgroundColor: c.positiveSoft }]}>
              <ArrowDownLeft size={13} color={c.positive} />
              <Txt variant="caption" tone="positive" style={{ fontFamily: font.semibold }}>Entradas {hidden ? '••••' : formatBRL(sum('CREDIT'))}</Txt>
            </View>
            <View style={[st.chip, { backgroundColor: c.surface2 }]}>
              <ArrowUpRight size={13} color={c.fg2} />
              <Txt variant="caption" tone="fg2" style={{ fontFamily: font.semibold }}>Saídas {hidden ? '••••' : formatBRL(sum('DEBIT'))}</Txt>
            </View>
          </View>
        </Glass>
      </Animated.View>

      <Animated.View entering={FadeInDown.delay(150).duration(500)}>
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 14, paddingVertical: 4, paddingHorizontal: 2 }}>
          {shortcuts.map(([label, Icon, href]) => (
            <Press key={label} accessibilityLabel={label} onPress={() => router.push(href)} style={{ alignItems: 'center', gap: 8, width: 72 }}>
              <Glass radius={34} interactive style={st.shortcut}><Icon size={24} color={c.fg} strokeWidth={2} /></Glass>
              <Txt variant="caption" style={{ fontFamily: font.medium, fontSize: 13, textAlign: 'center' }}>{label}</Txt>
            </Press>
          ))}
        </ScrollView>
      </Animated.View>

      <Animated.View entering={FadeInDown.delay(210).duration(500)}>
        <Press accessibilityLabel="Meus cartões" onPress={() => router.push('/cartoes')}>
          <Glass radius={999} style={st.pillRow}>
            <CreditCard size={20} color={c.fg2} />
            <Txt variant="label" style={{ fontFamily: font.semibold, flex: 1 }}>Meus cartões{cards.data?.length ? ` · ${cards.data.length}` : ''}</Txt>
            <ChevronRight size={18} color={c.subtle} />
          </Glass>
        </Press>
      </Animated.View>

      {account && <Animated.View entering={FadeInDown.delay(260).duration(500)}><Credit accountId={account.id} hidden={hidden} /></Animated.View>}

      <Card title="Movimentações" index={4}>
        {feed.isLoading ? <Loading /> : !items.length ? <Muted>Nenhuma movimentação ainda. Cadastre uma chave Pix e receba.</Muted> :
          items.slice(0, 12).map((i) => <FeedRow key={i.id} item={i} />)}
      </Card>
    </Screen>
  )
}

const st = StyleSheet.create({
  between: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  avatar: { width: 48, height: 48, borderRadius: 24, alignItems: 'center', justifyContent: 'center' },
  round: { width: 44, height: 44, alignItems: 'center', justifyContent: 'center' },
  chip: { flexDirection: 'row', alignItems: 'center', gap: 5, borderRadius: 999, paddingHorizontal: 11, paddingVertical: 6 },
  shortcut: { width: 68, height: 68, alignItems: 'center', justifyContent: 'center' },
  pillRow: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingHorizontal: 20, paddingVertical: 17 },
  promo: { flexDirection: 'row', alignItems: 'center', gap: 14, padding: 16 },
})
