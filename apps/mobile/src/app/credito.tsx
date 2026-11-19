import { ApiError, compareBRL, errorMessage, formatBRL, type CreditLine, type Invoice } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { View } from 'react-native'
import { CalendarClock, Receipt, Wallet } from 'lucide-react-native'
import { LinearGradient } from 'expo-linear-gradient'
import { Button, Card, Loading, MoneyField, Muted, Notice, Row, Sandbox, Screen, Txt } from '../components/ui'
import { useCountUp } from '../lib/motion'
import { api } from '../lib/client'
import { date, useAccount } from '../lib/queries'
import { creditGradient, font } from '../lib/theme'

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

export default function Credit() {
  const account = useAccount()
  const qc = useQueryClient()
  const base = `/v1/accounts/${account?.id}/credit`
  const [amount, setAmount] = useState<string | null>(null)
  const [error, setError] = useState('')
  const line = useQuery({
    queryKey: ['credit', account?.id], enabled: !!account,
    queryFn: async () => { try { return await api<CreditLine>('GET', base) } catch (e) { if (e instanceof ApiError && e.status === 404) return null; throw e } },
  })
  const invoices = useQuery({ queryKey: ['invoices', account?.id], enabled: !!line.data, queryFn: async () => (await api<{ invoices: Invoice[] }>('GET', `${base}/invoices`)).invoices })
  const refresh = () => { for (const k of ['credit', 'invoices', 'balance']) qc.invalidateQueries({ queryKey: [k] }) }
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
          <MoneyField label="Pagar" onValue={setAmount} />
          <Notice>{error}</Notice>
          <Button title="Pagar com saldo" variant="credit" disabled={!amount} loading={pay.isPending} onPress={() => pay.mutate()} />
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
