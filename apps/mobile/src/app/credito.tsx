import { ApiError, compareBRL, errorMessage, formatBRL, type CreditLine, type Invoice } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Text, View } from 'react-native'
import { Button, Card, Loading, MoneyField, Muted, Notice, Row, Sandbox, Screen } from '../components/ui'
import { api } from '../lib/client'
import { date, useAccount } from '../lib/queries'
import { colors } from '../lib/theme'

const label: Record<Invoice['status'], string> = { OPEN: 'Aberta', CLOSED: 'Fechada', PAID: 'Paga', OVERDUE: 'Vencida' }

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
        <Card title="Ativar crédito"><Muted>Limite inicial de R$ 1.000,00, fatura fechando todo dia 10.</Muted><Notice>{error}</Notice>
          <Button title="Ativar crédito" loading={activate.isPending} onPress={() => activate.mutate()} /></Card>
      </Screen>
    )
  }
  const l = line.data
  const pct = Math.min(100, (Number(l.used.amount) / Number(l.limit.amount)) * 100)
  return (
    <Screen>
      <Card>
        <Muted>Limite disponível</Muted>
        <Text style={{ fontSize: 30, fontWeight: '700' }}>{formatBRL(l.available)}</Text>
        <View style={{ height: 8, backgroundColor: '#f1f5f9', borderRadius: 4 }}><View style={{ height: 8, width: `${pct}%`, backgroundColor: colors.brand, borderRadius: 4 }} /></View>
        <Muted>Usado {formatBRL(l.used)} de {formatBRL(l.limit)}</Muted>
      </Card>
      {toPay && compareBRL(toPay.remaining.amount, '0.00') > 0 && (
        <Card title={`Fatura a pagar · vence ${date(toPay.due_date)}`}>
          <Text style={{ fontSize: 22, fontWeight: '700' }}>{formatBRL(toPay.remaining)}</Text>
          <MoneyField label="Pagar" onValue={setAmount} />
          <Notice>{error}</Notice>
          <Button title="Pagar com saldo" disabled={!amount} loading={pay.isPending} onPress={() => pay.mutate()} />
        </Card>
      )}
      <Card title="Faturas">
        {invoices.data?.map((i) => <Row key={i.id} title={`Fechamento ${date(i.closing_date)}`} subtitle={label[i.status]} right={<Text>{formatBRL(i.total)}</Text>} />)}
      </Card>
      <Sandbox>
        <Button title="Fechar a fatura agora" variant="secondary" loading={cycle.isPending} onPress={() => cycle.mutate('CLOSE')} />
        <Button title="Vencer a fatura fechada" variant="secondary" loading={cycle.isPending} onPress={() => cycle.mutate('EXPIRE')} />
      </Sandbox>
    </Screen>
  )
}
