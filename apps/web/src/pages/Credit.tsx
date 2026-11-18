import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useAccount } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import {Alert, Amount, Badge, Button, Empty, Loading, Modal, PageTitle, Panel, Sandbox, Select } from '../components/ui'
import { dateBR, dateTimeBR } from '../lib/format'
import { api, ApiError } from '../lib/api'
import { errorMessage } from '../lib/errors'
import { compareBRL, formatBRL } from '../lib/money'
import type { CreditLine, Invoice } from '../lib/types'

const invoiceStatus: Record<Invoice['status'], [string, 'brand' | 'amber' | 'green' | 'red']> = {
  OPEN: ['Aberta', 'brand'], CLOSED: ['Fechada', 'amber'], PAID: ['Paga', 'green'], OVERDUE: ['Vencida', 'red'],
}
const itemKinds: Record<string, string> = { PURCHASE: 'Compra', PAYMENT: 'Pagamento', BALANCE_CARRIED: 'Saldo anterior', FINE: 'Multa', INTEREST: 'Juros' }

function InvoiceModal({ base, invoice, onClose }: { base: string; invoice: Invoice | null; onClose: () => void }) {
  const qc = useQueryClient()
  const [amount, setAmount] = useState<string | null>(null)
  const [error, setError] = useState('')
  const detail = useQuery({
    queryKey: ['invoices', invoice?.id],
    enabled: !!invoice,
    queryFn: () => api<Invoice>('GET', `${base}/invoices/${invoice!.id}`),
  })
  const pay = useMutation({
    mutationFn: () => api('POST', `${base}/invoices/${invoice!.id}/payments`, { body: { amount }, idempotent: true }),
    onSuccess: () => { setError(''); for (const k of ['invoices', 'credit', 'balance']) qc.invalidateQueries({ queryKey: [k] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  const inv = detail.data ?? invoice
  const payable = inv && (inv.status === 'OPEN' || inv.status === 'CLOSED') && compareBRL(inv.remaining.amount, '0.00') > 0
  return (
    <Modal open={!!invoice} onClose={onClose} title={`Fatura de ${inv ? dateBR(inv.closing_date) : ''}`}>
      {inv && (
        <div className="space-y-4">
          <div className="grid grid-cols-3 gap-2 text-sm">
            <div><p className="text-slate-500">Total</p><p className="font-semibold">{formatBRL(inv.total)}</p></div>
            <div><p className="text-slate-500">Pago</p><p className="font-semibold">{formatBRL(inv.paid)}</p></div>
            <div><p className="text-slate-500">Vence</p><p className="font-semibold">{dateBR(inv.due_date)}</p></div>
          </div>
          {detail.isLoading ? <Loading /> : !inv.items?.length ? <Empty>Sem lançamentos.</Empty> : (
            <ul className="max-h-64 divide-y divide-slate-100 overflow-y-auto">
              {inv.items.map((it, i) => (
                <li key={i} className="flex justify-between gap-3 py-2 text-sm">
                  <span className="min-w-0"><span className="block truncate font-medium">{it.description || itemKinds[it.kind]}</span>
                    <span className="text-xs text-slate-500">{itemKinds[it.kind] ?? it.kind} · {dateTimeBR(it.occurred_at)}</span></span>
                  <Amount value={it.amount} direction={it.kind === 'PAYMENT' ? 'CREDIT' : undefined} />
                </li>
              ))}
            </ul>
          )}
          {payable && (
            <form onSubmit={(e) => { e.preventDefault(); pay.mutate() }} className="space-y-3 border-t border-slate-100 pt-4">
              <MoneyField label={`Pagar (falta ${formatBRL(inv.remaining)})`} onValue={setAmount} />
              <Alert>{error}</Alert>
              <Button type="submit" loading={pay.isPending} disabled={!amount}>Pagar com saldo da conta</Button>
            </form>
          )}
        </div>
      )}
    </Modal>
  )
}

export function CreditPage() {
  const account = useAccount()
  const qc = useQueryClient()
  const base = `/v1/accounts/${account?.id}/credit`
  const [closingDay, setClosingDay] = useState('10')
  const [open, setOpen] = useState<Invoice | null>(null)
  const [error, setError] = useState('')
  const line = useQuery({
    queryKey: ['credit', account?.id],
    enabled: !!account,
    queryFn: async () => {
      try {
        return await api<CreditLine>('GET', base)
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null
        throw e
      }
    },
  })
  const invoices = useQuery({
    queryKey: ['invoices', account?.id],
    enabled: !!line.data,
    queryFn: async () => (await api<{ invoices: Invoice[] }>('GET', `${base}/invoices`)).invoices,
  })
  const activate = useMutation({
    mutationFn: () => api('POST', base, { body: { closing_day: Number(closingDay) }, idempotent: true }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['credit'] }),
    onError: (e) => setError(errorMessage(e)),
  })
  const cycle = useMutation({
    mutationFn: (action: 'CLOSE' | 'EXPIRE') => api('POST', `/v1/sandbox/accounts/${account!.id}/credit/cycle`, { body: { action } }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['invoices'] }); qc.invalidateQueries({ queryKey: ['credit'] }) },
  })

  if (!account || line.isLoading) return <Loading />
  if (!line.data) {
    return (
      <div>
        <PageTitle subtitle="Use o mesmo cartão no crédito, com fatura mensal.">Crédito</PageTitle>
        <Panel title="Ativar cartão de crédito">
          <p className="mb-4 text-sm text-slate-600">Limite inicial de R$ 1.000,00. A fatura vence 10 dias depois do fechamento.</p>
          <form onSubmit={(e) => { e.preventDefault(); activate.mutate() }} className="space-y-4">
            <Select label="Dia de fechamento" value={closingDay} onChange={(e) => setClosingDay(e.target.value)}>
              {[1, 5, 10, 15, 20, 25].map((d) => <option key={d} value={d}>Dia {d}</option>)}
            </Select>
            <Alert>{error}</Alert>
            <Button type="submit" loading={activate.isPending}>Ativar crédito</Button>
          </form>
        </Panel>
      </div>
    )
  }
  const l = line.data
  const usedPct = Math.min(100, (Number(l.used.amount) / Number(l.limit.amount)) * 100)
  const list = invoices.data ?? []
  const current = list.find((i) => i.status === 'OPEN')
  const toPay = list.find((i) => i.status === 'CLOSED')
  return (
    <div className="space-y-6">
      <PageTitle subtitle={`Fecha todo dia ${l.closing_day} · juros do rotativo de ${l.monthly_interest_rate}% ao mês`}>Crédito</PageTitle>
      <Panel>
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div><p className="text-sm text-slate-500">Limite disponível</p><p className="tabular text-3xl font-bold">{formatBRL(l.available)}</p></div>
          <div className="text-right text-sm text-slate-500"><p>Usado {formatBRL(l.used)}</p><p>de {formatBRL(l.limit)}</p></div>
        </div>
        <div className="mt-4 h-2 overflow-hidden rounded-full bg-slate-100"><div className="h-full rounded-full bg-brand-600" style={{ width: `${usedPct}%` }} /></div>
      </Panel>
      <div className="grid gap-6 lg:grid-cols-2">
        {toPay && (
          <Panel title={<span className="flex items-center gap-2">Fatura a pagar <Badge tone="amber">vence {dateBR(toPay.due_date)}</Badge></span>}>
            <p className="tabular text-2xl font-bold">{formatBRL(toPay.remaining)}</p>
            <Button className="mt-4" onClick={() => setOpen(toPay)}>Ver e pagar</Button>
          </Panel>
        )}
        {current && (
          <Panel title="Fatura atual">
            <p className="tabular text-2xl font-bold">{formatBRL(current.total)}</p>
            <p className="text-sm text-slate-500">Fecha em {dateBR(current.closing_date)}</p>
            <Button className="mt-4" variant="secondary" onClick={() => setOpen(current)}>Ver lançamentos</Button>
          </Panel>
        )}
      </div>
      <Panel title="Faturas">
        {invoices.isLoading ? <Loading /> : (
          <ul className="divide-y divide-slate-100">
            {list.map((i) => (
              <li key={i.id}>
                <button onClick={() => setOpen(i)} className="flex w-full items-center gap-3 py-3 text-left hover:bg-slate-50">
                  <span className="flex-1 text-sm font-medium">Fechamento {dateBR(i.closing_date)}</span>
                  <Badge tone={invoiceStatus[i.status][1]}>{invoiceStatus[i.status][0]}</Badge>
                  <Amount value={i.total} className="w-28 text-right text-sm" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </Panel>
      <Sandbox>
        <p className="mb-3 text-sm text-slate-600">Sem esperar o calendário:</p>
        <div className="flex flex-wrap gap-2">
          <Button variant="secondary" loading={cycle.isPending} onClick={() => cycle.mutate('CLOSE')}>Fechar a fatura agora</Button>
          <Button variant="secondary" loading={cycle.isPending} onClick={() => cycle.mutate('EXPIRE')}>Vencer a fatura fechada</Button>
        </div>
      </Sandbox>
      <InvoiceModal base={base} invoice={open} onClose={() => setOpen(null)} />
    </div>
  )
}
