import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useAccount } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import {Alert, Amount, Badge, Button, Empty, Field, Loading, PageTitle, Panel, Sandbox, Select } from '../components/ui'
import { dateBR } from '../lib/format'
import { api, newIdempotencyKey } from '../lib/api'
import { errorMessage } from '../lib/errors'
import { addBRL, formatBRL } from '../lib/money'
import type { BoletoPayment, BoletoQuote } from '../lib/types'

const statusLabel: Record<string, [string, 'green' | 'amber' | 'red' | 'slate' | 'brand']> = {
  COMPLETED: ['Pago', 'green'], SCHEDULED: ['Agendado', 'brand'], CANCELLED: ['Cancelado', 'slate'],
  FAILED: ['Recusado', 'red'], REJECTED: ['Não pago', 'red'],
}

const todaySP = () => new Date().toLocaleDateString('sv-SE', { timeZone: 'America/Sao_Paulo' })

export function BoletosPage() {
  const account = useAccount()
  const qc = useQueryClient()
  const [line, setLine] = useState('')
  const [quote, setQuote] = useState<BoletoQuote | null>(null)
  const [amount, setAmount] = useState<string | null>(null)
  const [date, setDate] = useState(todaySP())
  const [error, setError] = useState('')
  const [done, setDone] = useState<BoletoPayment | null>(null)
  const [idem, setIdem] = useState(newIdempotencyKey)
  const base = `/v1/accounts/${account?.id}/boleto-payments`
  const payments = useQuery({
    queryKey: ['boletos', account?.id],
    enabled: !!account,
    queryFn: async () => (await api<{ payments: BoletoPayment[] }>('GET', base)).payments,
  })
  const lookup = useMutation({
    mutationFn: (l: string) => api<BoletoQuote>('POST', '/v1/boletos/lookup', { body: { line: l } }),
    onSuccess: (q) => { setQuote(q); setError(''); setDate(todaySP()) },
    onError: (e) => { setQuote(null); setError(errorMessage(e)) },
  })
  const pay = useMutation({
    mutationFn: () => api<BoletoPayment>('POST', base, {
      body: { line: quote!.line, amount: quote!.nominal.amount === '0.00' ? amount : undefined, scheduled_for: date === todaySP() ? undefined : date },
      idempotent: idem }),
    onSuccess: (p) => { setDone(p); for (const k of ['boletos', 'balance', 'statement']) qc.invalidateQueries({ queryKey: [k] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  const cancel = useMutation({ mutationFn: (id: string) => api('DELETE', `${base}/${id}`), onSuccess: () => qc.invalidateQueries({ queryKey: ['boletos'] }) })
  const reset = () => { setQuote(null); setLine(''); setDone(null); setError(''); setIdem(newIdempotencyKey()) }

  const [sb, setSb] = useState({ amount: null as string | null, due: todaySP(), behavior: '' })
  const issue = useMutation({
    mutationFn: () => api<{ formatted_line: string }>('POST', '/v1/sandbox/boletos', { body: { amount: sb.amount, due_date: sb.due, payee_name: 'CIA PAULISTA DE ENERGIA', behavior: sb.behavior } }),
    onSuccess: (r) => { reset(); setLine(r.formatted_line); lookup.mutate(r.formatted_line) },
  })

  if (!account) return <Loading />
  return (
    <div className="space-y-6">
      <PageTitle subtitle="Pague contas pela linha digitável, na hora ou agendado.">Boletos</PageTitle>
      <Panel title="Pagar boleto">
        {done ? (
          <div className="space-y-3 text-center">
            <div className="mx-auto grid size-12 place-items-center rounded-full bg-emerald-50 text-xl text-emerald-700">✓</div>
            <p className="font-semibold">{done.status === 'SCHEDULED' ? `Agendado para ${dateBR(done.scheduled_for)}` : 'Boleto pago'}</p>
            <p className="tabular text-2xl font-bold">{formatBRL(done.amount)}</p>
            <p className="text-sm text-slate-600">{done.payee.name}</p>
            <Button variant="secondary" onClick={reset}>Pagar outro</Button>
          </div>
        ) : !quote ? (
          <form onSubmit={(e) => { e.preventDefault(); lookup.mutate(line) }} className="space-y-4">
            <Field label="Linha digitável ou código de barras" required inputMode="numeric" value={line} onChange={(e) => setLine(e.target.value)}
              placeholder="00000.00000 00000.000000 00000.000000 0 00000000000000" />
            <Alert>{error}</Alert>
            <Button type="submit" loading={lookup.isPending}>Consultar</Button>
          </form>
        ) : (
          <div className="space-y-4">
            <div className="rounded-xl bg-slate-50 p-4 text-sm">
              <p className="font-semibold">{quote.payee.name}</p>
              <p className="text-slate-600">{quote.payee.document} · {quote.payee.bank}</p>
              <p className="mt-2 font-mono text-xs break-all text-slate-500">{quote.formatted_line}</p>
            </div>
            <dl className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
              <div><dt className="text-slate-500">Vencimento</dt><dd className="font-medium">{dateBR(quote.due_date)}</dd></div>
              <div><dt className="text-slate-500">Valor</dt><dd className="font-medium">{formatBRL(quote.nominal)}</dd></div>
              <div><dt className="text-slate-500">Multa + juros</dt><dd className="font-medium">{formatBRL(addBRL(quote.fine.amount, quote.interest.amount))}</dd></div>
              <div><dt className="text-slate-500">Total hoje</dt><dd className="text-lg font-bold">{formatBRL(quote.total)}</dd></div>
            </dl>
            {quote.nominal.amount === '0.00' && <MoneyField label="Valor a pagar" onValue={setAmount} />}
            <Field label="Data do pagamento" type="date" min={todaySP()} max={quote.due_date || undefined} value={date} onChange={(e) => setDate(e.target.value)}
              hint={date === todaySP() ? 'Pago agora.' : 'Agendado: pago nesse dia, com o saldo de então.'} />
            <Alert>{error}</Alert>
            <div className="flex gap-2">
              <Button loading={pay.isPending} onClick={() => pay.mutate()}>{date === todaySP() ? 'Pagar agora' : 'Agendar'}</Button>
              <Button variant="ghost" onClick={reset}>Voltar</Button>
            </div>
          </div>
        )}
      </Panel>

      <Panel title="Pagamentos">
        {payments.isLoading ? <Loading /> : !payments.data?.length ? <Empty>Nenhum boleto pago ainda.</Empty> : (
          <ul className="divide-y divide-slate-100">
            {payments.data.map((p) => (
              <li key={p.id} className="flex items-center gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{p.payee.name || 'Boleto'}</p>
                  <p className="text-xs text-slate-500">{p.status === 'SCHEDULED' ? `para ${dateBR(p.scheduled_for)}` : dateBR(p.created_at)}{p.failure_reason && ` · ${p.failure_reason}`}</p>
                </div>
                <Badge tone={(statusLabel[p.status] ?? [p.status, 'slate'])[1]}>{(statusLabel[p.status] ?? [p.status])[0]}</Badge>
                <Amount value={p.amount} className="w-24 text-right text-sm" />
                {p.status === 'SCHEDULED' && <Button variant="ghost" onClick={() => cancel.mutate(p.id)}>Cancelar</Button>}
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <Sandbox>
        <form onSubmit={(e) => { e.preventDefault(); issue.mutate() }} className="grid gap-3 sm:grid-cols-4 sm:items-end">
          <MoneyField label="Gerar boleto de" onValue={(v) => setSb({ ...sb, amount: v })} />
          <Field label="Vencimento" type="date" value={sb.due} onChange={(e) => setSb({ ...sb, due: e.target.value })} />
          <Select label="Comportamento" value={sb.behavior} onChange={(e) => setSb({ ...sb, behavior: e.target.value })}>
            <option value="">Normal</option><option value="PAID">Já pago</option><option value="REJECT">Banco recusa</option>
          </Select>
          <Button type="submit" loading={issue.isPending} disabled={!sb.amount}>Gerar e consultar</Button>
        </form>
      </Sandbox>
    </div>
  )
}
