import { addBRL, errorMessage, formatBRL, type BoletoPayment, type BoletoQuote } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Text } from 'react-native'
import { Button, Card, Field, MoneyField, Muted, Notice, Row, Sandbox, Screen } from '../components/ui'
import { api } from '../lib/client'
import { date, useAccount } from '../lib/queries'

const status: Record<string, string> = { COMPLETED: 'Pago', SCHEDULED: 'Agendado', CANCELLED: 'Cancelado', FAILED: 'Recusado', REJECTED: 'Não pago' }

export default function Boletos() {
  const account = useAccount()
  const qc = useQueryClient()
  const [line, setLine] = useState('')
  const [quote, setQuote] = useState<BoletoQuote | null>(null)
  const [done, setDone] = useState('')
  const [error, setError] = useState('')
  const [amount, setAmount] = useState<string | null>(null)
  const base = `/v1/accounts/${account?.id}/boleto-payments`
  const payments = useQuery({ queryKey: ['boletos', account?.id], enabled: !!account, queryFn: async () => (await api<{ payments: BoletoPayment[] }>('GET', base)).payments })
  const lookup = useMutation({
    mutationFn: (l: string) => api<BoletoQuote>('POST', '/v1/boletos/lookup', { body: { line: l } }),
    onSuccess: (q) => { setQuote(q); setError(''); setDone('') }, onError: (e) => setError(errorMessage(e)),
  })
  const pay = useMutation({
    mutationFn: () => api<BoletoPayment>('POST', base, { body: { line: quote!.line }, idempotent: true }),
    onSuccess: (p) => { setDone(`${formatBRL(p.amount)} para ${p.payee.name}`); setQuote(null); setLine(''); qc.invalidateQueries({ queryKey: ['boletos'] }); qc.invalidateQueries({ queryKey: ['balance'] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  const issue = useMutation({
    mutationFn: () => api<{ formatted_line: string }>('POST', '/v1/sandbox/boletos', { body: { amount, due_date: new Date().toLocaleDateString('sv-SE', { timeZone: 'America/Sao_Paulo' }), payee_name: 'CIA DE SANEAMENTO' } }),
    onSuccess: (r) => { setLine(r.formatted_line); lookup.mutate(r.formatted_line) },
  })
  return (
    <Screen>
      <Card title="Pagar boleto">
        <Field label="Linha digitável" keyboardType="number-pad" multiline value={line} onChangeText={setLine} />
        {!quote && <Button title="Consultar" loading={lookup.isPending} onPress={() => lookup.mutate(line)} />}
        {quote && (
          <>
            <Row title={quote.payee.name} subtitle={`${quote.payee.bank} · vence ${date(quote.due_date)}`} />
            <Muted>Valor {formatBRL(quote.nominal)} · multa e juros {formatBRL(addBRL(quote.fine.amount, quote.interest.amount))}</Muted>
            <Text style={{ fontSize: 24, fontWeight: '700' }}>{formatBRL(quote.total)}</Text>
            <Button title="Pagar agora" loading={pay.isPending} onPress={() => pay.mutate()} />
          </>
        )}
        <Notice tone="green">{done && `Boleto pago: ${done}`}</Notice>
        <Notice>{error}</Notice>
      </Card>
      <Card title="Pagamentos">
        {!payments.data?.length ? <Muted>Nenhum boleto pago ainda.</Muted> : payments.data.map((p) => (
          <Row key={p.id} title={p.payee.name || 'Boleto'} subtitle={`${status[p.status] ?? p.status} · ${date(p.created_at)}`} right={<Text>{formatBRL(p.amount)}</Text>} />
        ))}
      </Card>
      <Sandbox>
        <MoneyField label="Gerar boleto de" onValue={setAmount} />
        <Button title="Gerar e consultar" disabled={!amount} loading={issue.isPending} onPress={() => issue.mutate()} />
      </Sandbox>
    </Screen>
  )
}
