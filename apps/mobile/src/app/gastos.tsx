import { budgetLabel, budgetState, errorMessage, formatBRL, monthLabel, shiftMonth, spendingCategories, type Budget, type BudgetCategory, type SpendingSummary } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, PieChart, Plus, Store, Target, TrendingDown, TrendingUp, X } from 'lucide-react-native'
import { useState } from 'react'
import { ScrollView, View } from 'react-native'
import { Button, Card, Loading, MoneyField, Muted, Notice, Press, Row, Screen, Txt } from '../components/ui'
import { api } from '../lib/client'
import { useAccount } from '../lib/queries'
import { font, useTheme } from '../lib/theme'

const currentMonth = () => {
  const d = new Date(Date.now() - 3 * 3600_000) // Brasília
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

const stateTone = { ok: 'positive', near: 'warning', over: 'danger' } as const
const budgetCategories: BudgetCategory[] = ['TOTAL', ...(Object.keys(spendingCategories) as BudgetCategory[])]

/** Monthly budgets with how much of each was spent in the month shown. */
function BudgetsCard({ accountId, month, current }: { accountId: string; month: string; current: boolean }) {
  const { c } = useTheme()
  const qc = useQueryClient()
  const base = `/v1/accounts/${accountId}/budgets`
  const [adding, setAdding] = useState(false)
  const [category, setCategory] = useState<BudgetCategory | null>(null)
  const [amount, setAmount] = useState<string | null>(null)
  const [error, setError] = useState('')
  const budgets = useQuery({ queryKey: ['budgets', accountId, month], queryFn: async () => (await api<{ budgets: Budget[] }>('GET', `${base}?month=${month}`)).budgets })
  const refresh = () => qc.invalidateQueries({ queryKey: ['budgets'] })
  const save = useMutation({
    mutationFn: () => api('PUT', `${base}/${category}`, { body: { amount } }),
    onSuccess: () => { setAdding(false); setCategory(null); setError(''); refresh() },
    onError: (e) => setError(errorMessage(e)),
  })
  const remove = useMutation({ mutationFn: (cat: string) => api('DELETE', `${base}/${cat}`), onSuccess: refresh })
  const list = budgets.data ?? []
  const free = budgetCategories.filter((cat) => !list.some((b) => b.category === cat))
  return (
    <Card title="Orçamentos" icon={Target} index={1}>
      {!list.length && !adding && <Muted>Defina quanto quer gastar por mês, no total ou por categoria, e receba um aviso antes de passar do ponto.</Muted>}
      {list.map((b) => {
        const st = budgetState(b)
        return (
          <View key={b.category} style={{ gap: 6 }}>
            <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
              <Txt variant="label" style={{ flex: 1, fontFamily: font.medium }}>{budgetLabel(b.category)}</Txt>
              <Txt variant="label" style={{ fontFamily: font.semibold }}>{formatBRL(b.spent)}</Txt>
              <Txt variant="caption" tone="muted">de {formatBRL(b.amount)}</Txt>
              {current && (
                <Press accessibilityLabel={`Remover orçamento de ${budgetLabel(b.category)}`} onPress={() => remove.mutate(b.category)}>
                  <X size={16} color={c.muted} />
                </Press>
              )}
            </View>
            <View style={{ height: 6, borderRadius: 3, backgroundColor: c.surface2, overflow: 'hidden' }}
              accessibilityRole="progressbar" accessibilityValue={{ min: 0, max: 100, now: Math.min(100, Math.round(st.pct)) }}>
              <View style={{ height: 6, width: `${Math.min(100, st.pct)}%`, backgroundColor: c[stateTone[st.state]] }} />
            </View>
            <Txt variant="caption" tone="muted">{Math.round(st.pct)}% · {st.label}</Txt>
          </View>
        )
      })}
      {current && adding && (
        <>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
            {free.map((cat) => (
              <Press key={cat} accessibilityLabel={budgetLabel(cat)} onPress={() => setCategory(cat)}>
                <View style={{ paddingHorizontal: 12, paddingVertical: 8, borderRadius: 999, borderWidth: 1,
                  borderColor: category === cat ? c.brand : c.line, backgroundColor: category === cat ? c.brandSoft : 'transparent' }}>
                  <Txt variant="caption" tone={category === cat ? 'brandStrong' : 'fg2'}>{budgetLabel(cat)}</Txt>
                </View>
              </Press>
            ))}
          </ScrollView>
          <MoneyField label="Quanto por mês" onValue={setAmount} />
          <Muted>Avisamos quando você chegar a 80% e a 100% do valor no mês.</Muted>
          <Notice>{error}</Notice>
          <Button title="Definir orçamento" disabled={!category || !amount} loading={save.isPending} onPress={() => save.mutate()} />
          <Button title="Cancelar" variant="ghost" onPress={() => { setAdding(false); setError('') }} />
        </>
      )}
      {current && !adding && free.length > 0 && <Button title="Novo orçamento" icon={Plus} variant="secondary" onPress={() => setAdding(true)} />}
    </Card>
  )
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
          <BudgetsCard accountId={account!.id} month={month} current={atEnd} />
          <Card title="Por categoria" icon={PieChart} index={2}>
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
          <Card title="Onde você mais gastou" icon={Store} index={3}>
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
