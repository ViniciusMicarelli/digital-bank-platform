import { formatBRL, monthLabel, shiftMonth, spendingCategories, type SpendingSummary } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, PieChart, Store, TrendingDown, TrendingUp } from 'lucide-react-native'
import { useState } from 'react'
import { View } from 'react-native'
import { Card, Loading, Muted, Press, Row, Screen, Txt } from '../components/ui'
import { api } from '../lib/client'
import { useAccount } from '../lib/queries'
import { font, useTheme } from '../lib/theme'

const currentMonth = () => {
  const d = new Date(Date.now() - 3 * 3600_000) // Brasília
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

export default function Spending() {
  const { c } = useTheme()
  const account = useAccount()
  const [month, setMonth] = useState(currentMonth)
  const summary = useQuery({
    queryKey: ['spending', account?.id, month], enabled: !!account,
    queryFn: () => api<SpendingSummary>('GET', `/v1/accounts/${account!.id}/spending?month=${month}`),
  })
  const s = summary.data
  const total = Number(s?.total.amount ?? 0)
  const previous = Number(s?.previous_total.amount ?? 0)
  const change = previous > 0 ? ((total - previous) / previous) * 100 : null
  const atEnd = month >= currentMonth()
  return (
    <Screen>
      <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' }}>
        <Press accessibilityLabel="Mês anterior" onPress={() => setMonth(shiftMonth(month, -1))}><ChevronLeft size={24} color={c.fg2} /></Press>
        <Txt variant="heading">{monthLabel(month)}</Txt>
        <Press accessibilityLabel="Próximo mês" disabled={atEnd} onPress={() => setMonth(shiftMonth(month, 1))}><ChevronRight size={24} color={atEnd ? c.subtle : c.fg2} /></Press>
      </View>
      {!s ? <Loading /> : (
        <>
          <Card>
            <Txt variant="caption" tone="muted">Gasto no mês</Txt>
            <Txt variant="title" style={{ fontSize: 32 }}>{formatBRL(s.total)}</Txt>
            {change !== null && (
              <View style={{ flexDirection: 'row', alignItems: 'center', gap: 6 }}>
                {change > 0 ? <TrendingUp size={16} color={c.danger} /> : <TrendingDown size={16} color={c.positive} />}
                <Txt variant="caption" tone={change > 0 ? 'danger' : 'positive'}>{Math.abs(Math.round(change))}% {change > 0 ? 'a mais' : 'a menos'} que no mês anterior</Txt>
              </View>
            )}
            {total > 0 && (
              <View style={{ flexDirection: 'row', height: 12, borderRadius: 6, overflow: 'hidden', gap: 2 }}>
                {s.categories.map((cat) => <View key={cat.category} style={{ flex: Number(cat.amount.amount), backgroundColor: spendingCategories[cat.category].color }} />)}
              </View>
            )}
          </Card>
          <Card title="Por categoria" icon={PieChart} index={1}>
            {total === 0 ? <Muted>Nenhum gasto neste mês.</Muted> : s.categories.map((cat) => {
              const meta = spendingCategories[cat.category]
              const pct = (Number(cat.amount.amount) / total) * 100
              return (
                <View key={cat.category} style={{ gap: 6 }}>
                  <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
                    <View style={{ width: 10, height: 10, borderRadius: 5, backgroundColor: meta.color }} />
                    <Txt variant="label" style={{ flex: 1, fontFamily: font.medium }}>{meta.label}</Txt>
                    <Txt variant="caption" tone="muted">{Math.round(pct)}%</Txt>
                    <Txt variant="label" style={{ fontFamily: font.semibold, minWidth: 90, textAlign: 'right' }}>{formatBRL(cat.amount)}</Txt>
                  </View>
                  <View style={{ height: 6, borderRadius: 3, backgroundColor: c.surface2, overflow: 'hidden', marginLeft: 18 }}>
                    <View style={{ height: 6, width: `${pct}%`, backgroundColor: meta.color }} />
                  </View>
                </View>
              )
            })}
          </Card>
          <Card title="Onde você mais gastou" icon={Store} index={2}>
            {!s.merchants.length ? <Muted>Sem compras neste mês.</Muted> : s.merchants.map((m) => (
              <Row key={m.name} title={m.name} subtitle={`${spendingCategories[m.category].label} · ${m.count} ${m.count === 1 ? 'compra' : 'compras'}`}
                right={<Txt variant="label" style={{ fontFamily: font.semibold }}>{formatBRL(m.amount)}</Txt>} />
            ))}
          </Card>
        </>
      )}
    </Screen>
  )
}
