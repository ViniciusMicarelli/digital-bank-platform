import { ApiError, compareBRL, errorMessage, formatBRL, type CreditLine, type InstallmentPlan, type Invoice, type PlanQuote } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { View } from 'react-native'
import { CalendarClock, Layers, Receipt, Wallet } from 'lucide-react-native'
import { LinearGradient } from 'expo-linear-gradient'
import { Button, Card, Loading, MoneyField, Muted, Notice, Press, Row, Sandbox, Screen, Txt } from '../components/ui'
import { useCountUp } from '../lib/motion'
import { api } from '../lib/client'
import { date, useAccount } from '../lib/queries'
import { creditGradient, font, useTheme } from '../lib/theme'

const label: Record<Invoice['status'], string> = { OPEN: 'Aberta', CLOSED: 'Fechada', PAID: 'Paga', OVERDUE: 'Vencida' }

function CreditHero({ available, used, limit, pct }: { available: string; used: string; limit: string; pct: number }) {
  const shown = useCountUp(Number(available))
  return (
    <LinearGradient colors={[...creditGradient]} start={{ x: 1, y: 0 }} end={{ x: 0, y: 1 }} style={{ borderRadius: 28, padding: 22, gap: 12, overflow: 'hidden' }}>
      <Txt variant="label" style={{ color: '#ffffffb3' }}>Limite disponível</Txt>
      <Txt variant="display" tone="white">{Math.abs(shown - Number(available)) < 0.005 ? formatBRL(available) : formatBRL(shown.toFixed(2))}</Txt>
      <View style={{ height: 10, backgroundColor: '#ffffff26', borderRadius: 5, overflow: 'hidden' }}>
        <LinearGradient colors={['#a48bff', '#2ed3a0']} start={{ x: 0, y: 0 }} end={{ x: 1, y: 0 }} style={{ height: 10, width: `${pct}%`, borderRadius: 5 }} />
      </View>
      <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
        <Txt variant="caption" style={{ color: '#ffffffb3' }}>Usado {formatBRL(used)}</Txt>
        <Txt variant="caption" style={{ color: '#ffffffb3' }}>Limite {formatBRL(limit)}</Txt>
      </View>
    </LinearGradient>
  )
}

function PlanPicker({ base, invoice, onDone }: { base: string; invoice: Invoice; onDone: () => void }) {
  const { c } = useTheme()
  const [chosen, setChosen] = useState<number | null>(null)
  const [error, setError] = useState('')
  const quotes = useQuery({ queryKey: ['plan-quotes', invoice.id], queryFn: async () => (await api<{ quotes: PlanQuote[] }>('GET', `${base}/invoices/${invoice.id}/plans`)).quotes })
  const plan = useMutation({ mutationFn: () => api('POST', `${base}/invoices/${invoice.id}/plans`, { body: { installments: chosen }, idempotent: true }), onSuccess: onDone, onError: (e) => setError(errorMessage(e)) })
  if (!quotes.data) return <Loading />
  const pick = quotes.data.find((q) => q.installments === chosen)
  return (
    <View style={{ gap: 8 }}>
      {quotes.data.filter((q) => [2, 3, 4, 6, 10, 12].includes(q.installments)).map((q) => (
        <Press key={q.installments} accessibilityLabel={`${q.installments}x`} onPress={() => setChosen(q.installments)}>
          <View style={{ flexDirection: 'row', justifyContent: 'space-between', padding: 12, borderRadius: 16, borderWidth: chosen === q.installments ? 2 : 1,
            borderColor: chosen === q.installments ? c.credit : c.line, backgroundColor: chosen === q.installments ? c.creditSoft : 'transparent' }}>
            <Txt variant="label" style={{ fontFamily: font.semibold }}>{q.installments}x de {formatBRL(q.installment)}</Txt>
            <Txt variant="caption" tone="muted">total {formatBRL(q.total)}</Txt>
          </View>
        </Press>
      ))}
      {pick && <Txt variant="caption" tone="muted">Juros de {pick.monthly_rate.toLocaleString('pt-BR')}% ao mês · CET de {pick.annual_cost.toLocaleString('pt-BR', { maximumFractionDigits: 1 })}% ao ano. A primeira parcela entra na fatura atual.</Txt>}
      <Notice>{error}</Notice>
      <Button title={pick ? `Parcelar em ${pick.installments}x` : 'Escolha as parcelas'} variant="credit" disabled={!pick} loading={plan.isPending} onPress={() => plan.mutate()} />
    </View>
  )
}

export default function Credit() {
  const account = useAccount()
  const qc = useQueryClient()
  const base = `/v1/accounts/${account?.id}/credit`
  const [amount, setAmount] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [planning, setPlanning] = useState(false)
  const line = useQuery({
    queryKey: ['credit', account?.id], enabled: !!account,
    queryFn: async () => { try { return await api<CreditLine>('GET', base) } catch (e) { if (e instanceof ApiError && e.status === 404) return null; throw e } },
  })
  const invoices = useQuery({ queryKey: ['invoices', account?.id], enabled: !!line.data, queryFn: async () => (await api<{ invoices: Invoice[] }>('GET', `${base}/invoices`)).invoices })
  const plans = useQuery({ queryKey: ['installments', account?.id], enabled: !!line.data, queryFn: async () => (await api<{ plans: InstallmentPlan[] }>('GET', `${base}/installments`)).plans })
  const refresh = () => { for (const k of ['credit', 'invoices', 'balance', 'installments', 'plan-quotes']) qc.invalidateQueries({ queryKey: [k] }) }
  const activate = useMutation({ mutationFn: () => api('POST', base, { body: { closing_day: 10 }, idempotent: true }), onSuccess: refresh, onError: (e) => setError(errorMessage(e)) })
  const toPay = invoices.data?.find((i) => i.status === 'CLOSED')
  const pay = useMutation({ mutationFn: () => api('POST', `${base}/invoices/${toPay!.id}/payments`, { body: { amount }, idempotent: true }), onSuccess: () => { setError(''); refresh() }, onError: (e) => setError(errorMessage(e)) })
  const cycle = useMutation({ mutationFn: (action: string) => api('POST', `/v1/sandbox/accounts/${account!.id}/credit/cycle`, { body: { action } }), onSuccess: refresh })
  if (!account || line.isLoading) return <Loading />
  if (!line.data) {
    return (
      <Screen>
        <Card title="Ativar crédito" icon={Wallet}><Muted>Limite inicial de R$ 1.000,00, fatura fechando todo dia 10.</Muted><Notice>{error}</Notice>
          <Button title="Ativar crédito" variant="credit" loading={activate.isPending} onPress={() => activate.mutate()} /></Card>
      </Screen>
    )
  }
  const l = line.data
  const pct = Math.min(100, (Number(l.used.amount) / Number(l.limit.amount)) * 100)
  return (
    <Screen>
      <CreditHero available={l.available.amount} used={l.used.amount} limit={l.limit.amount} pct={pct} />
      {toPay && compareBRL(toPay.remaining.amount, '0.00') > 0 && (
        <Card title={`Fatura a pagar · vence ${date(toPay.due_date)}`} icon={CalendarClock} index={1}>
          <Txt variant="heading" style={{ fontSize: 22 }}>{formatBRL(toPay.remaining)}</Txt>
          {toPay.minimum_payment && <Txt variant="caption" tone="muted">Pagamento mínimo {formatBRL(toPay.minimum_payment)}; o que faltar vai para a próxima fatura com juros do rotativo.</Txt>}
          {planning ? <PlanPicker base={base} invoice={toPay} onDone={() => { setPlanning(false); refresh() }} /> : (
            <>
              <MoneyField label="Pagar" onValue={setAmount} />
              <Notice>{error}</Notice>
              <Button title="Pagar com saldo" variant="credit" disabled={!amount} loading={pay.isPending} onPress={() => pay.mutate()} />
              {compareBRL(toPay.remaining.amount, '100.00') >= 0 && <Button title="Parcelar fatura" variant="secondary" onPress={() => setPlanning(true)} />}
            </>
          )}
        </Card>
      )}
      {!!plans.data?.length && (
        <Card title="Compras parceladas" icon={Layers} index={2}>
          {plans.data.map((p) => (
            <Row key={p.description + p.purchased_at} icon={Layers} tone="credit" title={p.description}
              subtitle={`${p.installments}x de ${formatBRL(p.installment_amount)} · ${p.billed} de ${p.installments} na fatura`}
              right={<Txt variant="label" style={{ fontFamily: font.semibold }}>{formatBRL(p.remaining)}</Txt>} />
          ))}
        </Card>
      )}
      <Card title="Faturas" icon={Receipt} index={2}>
        {invoices.data?.map((i) => <Row key={i.id} icon={Receipt} tone={i.status === 'PAID' ? 'green' : i.status === 'OVERDUE' ? 'red' : i.status === 'CLOSED' ? 'amber' : 'credit'} title={`Fechamento ${date(i.closing_date)}`} subtitle={label[i.status]} right={<Txt variant="label" style={{ fontFamily: font.semibold }}>{formatBRL(i.total)}</Txt>} />)}
      </Card>
      <Sandbox>
        <Button title="Fechar a fatura agora" variant="secondary" loading={cycle.isPending} onPress={() => cycle.mutate('CLOSE')} />
        <Button title="Vencer a fatura fechada" variant="secondary" loading={cycle.isPending} onPress={() => cycle.mutate('EXPIRE')} />
      </Sandbox>
    </Screen>
  )
}
