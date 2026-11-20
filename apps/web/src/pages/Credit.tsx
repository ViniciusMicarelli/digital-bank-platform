import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarClock, Gauge, Layers, Receipt, Wallet } from 'lucide-react'
import { useState } from 'react'
import { BankCard } from '../components/BankCard'
import { useCountUp } from '../lib/motion'
import { useAccount } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import { Alert, Amount, Badge, Button, Empty, Loading, Modal, PageTitle, Panel, Sandbox, Select } from '../components/ui'
import { dateBR, dateTimeBR } from '../lib/format'
import { api, ApiError } from '../lib/api'
import { errorMessage, compareBRL, formatBRL, type CreditLine, type InstallmentPlan, type Invoice, type LimitRequest, type PlanQuote } from '@bank/core'

const invoiceStatus: Record<Invoice['status'], [string, 'credit' | 'amber' | 'green' | 'red']> = {
  OPEN: ['Aberta', 'credit'], CLOSED: ['Fechada', 'amber'], PAID: ['Paga', 'green'], OVERDUE: ['Vencida', 'red'],
}
const itemKinds: Record<string, string> = {
  PURCHASE: 'Compra', PAYMENT: 'Pagamento', BALANCE_CARRIED: 'Saldo anterior', FINE: 'Multa', INTEREST: 'Juros',
  REFUND: 'Estorno', CREDIT_CARRIED_IN: 'Crédito anterior', CREDIT_CARRIED_OUT: 'Crédito para a próxima fatura',
  INVOICE_PLAN: 'Parcelamento',
}
const creditKinds = new Set(['PAYMENT', 'REFUND', 'CREDIT_CARRIED_IN', 'INVOICE_PLAN'])
const pct = (n: number) => `${n.toLocaleString('pt-BR', { maximumFractionDigits: 2 })}%`

function PlanPicker({ base, invoice, onDone }: { base: string; invoice: Invoice; onDone: () => void }) {
  const [chosen, setChosen] = useState<number | null>(null)
  const [error, setError] = useState('')
  const quotes = useQuery({
    queryKey: ['plan-quotes', invoice.id],
    queryFn: async () => (await api<{ quotes: PlanQuote[] }>('GET', `${base}/invoices/${invoice.id}/plans`)).quotes,
  })
  const plan = useMutation({
    mutationFn: () => api('POST', `${base}/invoices/${invoice.id}/plans`, { body: { installments: chosen }, idempotent: true }),
    onSuccess: onDone,
    onError: (e) => setError(errorMessage(e)),
  })
  if (quotes.isLoading) return <Loading />
  if (quotes.isError) return <Alert>{errorMessage(quotes.error)}</Alert>
  const pick = quotes.data!.find((q) => q.installments === chosen)
  return (
    <div className="space-y-3">
      <p className="text-sm text-fg-2">Divida {formatBRL(invoice.remaining)} em parcelas fixas nas próximas faturas, com juros menores que os do rotativo.</p>
      <ul role="radiogroup" aria-label="Parcelas" className="max-h-56 space-y-1.5 overflow-y-auto">
        {quotes.data!.map((q) => (
          <li key={q.installments}>
            <button type="button" role="radio" aria-checked={chosen === q.installments} onClick={() => setChosen(q.installments)}
              className={`flex w-full items-center justify-between rounded-2xl px-4 py-3 text-left text-sm transition ${chosen === q.installments ? 'bg-credit-soft ring-2 ring-credit' : 'ring-1 ring-line hover:bg-surface-2'}`}>
              <span className="font-semibold text-fg">{q.installments}x de {formatBRL(q.installment)}</span>
              <span className="text-xs text-muted">total {formatBRL(q.total)}</span>
            </button>
          </li>
        ))}
      </ul>
      {pick && (
        <p className="text-xs text-muted">
          Juros de {pct(pick.monthly_rate)} ao mês ({formatBRL(pick.interest)} no total) · CET de {pct(pick.annual_cost)} ao ano.
          A primeira parcela entra na fatura atual e esta fatura fica quitada.
        </p>
      )}
      <Alert>{error}</Alert>
      <Button variant="credit" disabled={!pick} loading={plan.isPending} onClick={() => plan.mutate()}>
        {pick ? `Parcelar em ${pick.installments}x` : 'Escolha as parcelas'}
      </Button>
    </div>
  )
}

function InvoiceModal({ base, invoice, onClose }: { base: string; invoice: Invoice | null; onClose: () => void }) {
  const qc = useQueryClient()
  const [amount, setAmount] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [planning, setPlanning] = useState(false)
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
    <Modal open={!!invoice} onClose={() => { setPlanning(false); onClose() }} title={`Fatura de ${inv ? dateBR(inv.closing_date) : ''}`}>
      {inv && (
        <div className="space-y-4">
          <div className="grid grid-cols-3 gap-2 text-sm">
            <div><p className="text-muted">Total</p><p className="font-semibold">{formatBRL(inv.total)}</p></div>
            <div><p className="text-muted">Pago</p><p className="font-semibold">{formatBRL(inv.paid)}</p></div>
            <div><p className="text-muted">Vence</p><p className="font-semibold">{dateBR(inv.due_date)}</p></div>
          </div>
          {detail.isLoading ? <Loading /> : !inv.items?.length ? <Empty>Sem lançamentos.</Empty> : (
            <ul className="max-h-64 divide-y divide-line overflow-y-auto">
              {inv.items.map((it, i) => (
                <li key={i} className="flex justify-between gap-3 py-2 text-sm">
                  <span className="min-w-0"><span className="block truncate font-medium">{it.description || itemKinds[it.kind]}</span>
                    <span className="text-xs text-muted">{itemKinds[it.kind] ?? it.kind} · {dateTimeBR(it.occurred_at)}</span></span>
                  <Amount value={it.amount} direction={creditKinds.has(it.kind) ? 'CREDIT' : undefined} />
                </li>
              ))}
            </ul>
          )}
          {payable && planning && <PlanPicker base={base} invoice={inv} onDone={() => { setPlanning(false); for (const k of ['invoices', 'credit', 'installments']) qc.invalidateQueries({ queryKey: [k] }); onClose() }} />}
          {payable && !planning && (
            <form onSubmit={(e) => { e.preventDefault(); pay.mutate() }} className="space-y-3 border-t border-line pt-4">
              {inv.minimum_payment && compareBRL(inv.minimum_payment.amount, inv.remaining.amount) < 0 && (
                <p className="text-xs text-muted">Pagamento mínimo: {formatBRL(inv.minimum_payment)}. O que faltar vai para a próxima fatura com juros do rotativo.</p>
              )}
              <MoneyField label={`Pagar (falta ${formatBRL(inv.remaining)})`} onValue={setAmount} />
              <Alert>{error}</Alert>
              <div className="flex flex-wrap gap-2">
                <Button type="submit" loading={pay.isPending} disabled={!amount}>Pagar com saldo da conta</Button>
                {inv.status === 'CLOSED' && compareBRL(inv.remaining.amount, '100.00') >= 0 && (
                  <Button type="button" variant="secondary" onClick={() => setPlanning(true)}>Parcelar fatura</Button>
                )}
              </div>
            </form>
          )}
        </div>
      )}
    </Modal>
  )
}

const limitStatus = { APPROVED: ['green', 'Aprovado'], REJECTED: ['red', 'Recusado'], IN_REVIEW: ['amber', 'Em análise'] } as const
const limitReasons: Record<string, string> = { HAS_OVERDUE_INVOICE: 'há uma fatura em atraso', CREDIT_LINE_BLOCKED: 'o crédito está bloqueado' }

function LimitPanel({ base, line }: { base: string; line: CreditLine }) {
  const qc = useQueryClient()
  const [amount, setAmount] = useState<string | null>(null)
  const [message, setMessage] = useState<{ tone: 'green' | 'red' | 'amber'; text: string } | null>(null)
  const requests = useQuery({
    queryKey: ['limit-requests', base],
    queryFn: async () => (await api<{ requests: LimitRequest[] }>('GET', `${base}/limit-requests`)).requests,
  })
  const ask = useMutation({
    mutationFn: () => api<LimitRequest>('POST', `${base}/limit`, { body: { amount }, idempotent: true }),
    onSuccess: (r) => {
      setMessage(r.status === 'APPROVED' ? { tone: 'green', text: `Pronto: seu limite agora é ${formatBRL(r.granted)}.` }
        : r.status === 'IN_REVIEW' ? { tone: 'amber', text: 'Pedido enviado para análise. Avisamos quando terminar.' }
          : { tone: 'red', text: `Não foi possível agora: ${limitReasons[r.reason] ?? r.reason}.` })
      for (const k of ['credit', 'limit-requests']) qc.invalidateQueries({ queryKey: [k] })
    },
    onError: (e) => setMessage({ tone: 'red', text: errorMessage(e) }),
  })
  const inReview = requests.data?.find((r) => r.status === 'IN_REVIEW')
  return (
    <Panel title="Ajustar limite" icon={Gauge}>
      {inReview ? (
        <Alert tone="amber">Pedido de {formatBRL(inReview.requested)} em análise desde {dateTimeBR(inReview.created_at)}.</Alert>
      ) : (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); setMessage(null); ask.mutate() }}>
          <p className="text-sm text-fg-2">Limite atual de {formatBRL(line.limit)}. Para diminuir é na hora; aumentos de até 50% também. Acima disso, um analista avalia.</p>
          <MoneyField label="Novo limite" onValue={setAmount} />
          <Button type="submit" variant="credit" loading={ask.isPending} disabled={!amount}>Pedir novo limite</Button>
        </form>
      )}
      {message && <div className="mt-3"><Alert tone={message.tone}>{message.text}</Alert></div>}
      {!!requests.data?.length && (
        <ul className="mt-4 divide-y divide-line border-t border-line">
          {requests.data.slice(0, 4).map((r) => (
            <li key={r.id} className="flex items-center justify-between gap-3 py-2.5 text-sm">
              <span className="text-fg-2">{formatBRL(r.current_limit)} → {formatBRL(r.requested)}</span>
              <span className="flex items-center gap-2">
                <span className="text-xs text-muted">{dateTimeBR(r.created_at)}</span>
                <Badge tone={limitStatus[r.status][0]}>{limitStatus[r.status][1]}</Badge>
              </span>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}

function CreditHero({ line, usedPct }: { line: CreditLine; usedPct: number }) {
  const ref = useCountUp(Number(line.available.amount), (n) => formatBRL(n.toFixed(2)))
  return (
    <section data-reveal className="relative overflow-hidden rounded-[28px] bg-[linear-gradient(135deg,#4a37a8_0%,#2d2075_55%,#1c1450_100%)] p-6 text-white shadow-lift sm:p-8">
      <p className="text-sm text-white/70">Limite disponível</p>
      <p className="tabular mt-1 text-4xl font-bold tracking-tight"><span ref={ref}>{formatBRL(line.available)}</span></p>
      <div className="mt-6 h-2.5 overflow-hidden rounded-full bg-white/15">
        <div className="h-full rounded-full bg-gradient-to-r from-[#a48bff] to-[#2ed3a0] transition-all duration-1000" style={{ width: `${usedPct}%` }} />
      </div>
      <div className="mt-3 flex justify-between text-sm text-white/70"><span>Usado {formatBRL(line.used)}</span><span>Limite {formatBRL(line.limit)}</span></div>
    </section>
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
  const plans = useQuery({
    queryKey: ['installments', account?.id],
    enabled: !!line.data,
    queryFn: async () => (await api<{ plans: InstallmentPlan[] }>('GET', `${base}/installments`)).plans,
  })
  const activate = useMutation({
    mutationFn: () => api('POST', base, { body: { closing_day: Number(closingDay) }, idempotent: true }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['credit'] }),
    onError: (e) => setError(errorMessage(e)),
  })
  const cycle = useMutation({
    mutationFn: (action: 'CLOSE' | 'EXPIRE') => api('POST', `/v1/sandbox/accounts/${account!.id}/credit/cycle`, { body: { action } }),
    onSuccess: () => { for (const k of ['invoices', 'credit', 'installments']) qc.invalidateQueries({ queryKey: [k] }) },
  })

  if (!account || line.isLoading) return <Loading />
  if (!line.data) {
    return (
      <div>
        <PageTitle subtitle="Use o mesmo cartão no crédito, com fatura mensal.">Crédito</PageTitle>
        <Panel title="Ativar cartão de crédito" icon={Wallet}>
          <div className="mb-6 grid items-center gap-6 sm:grid-cols-2">
            <BankCard holder={account.holder_name.toUpperCase()} last4="••••" expiry="—" skin="credit" label="Crédito" />
            <ul className="space-y-2 text-sm text-fg-2">
              <li>• Limite inicial de <b className="text-fg">R$ 1.000,00</b></li>
              <li>• O mesmo cartão: escolha débito ou crédito na compra</li>
              <li>• Fatura vence 10 dias depois do fechamento</li>
              <li>• Pague quando quiser, até antes de fechar</li>
            </ul>
          </div>
          <form onSubmit={(e) => { e.preventDefault(); activate.mutate() }} className="space-y-4">
            <Select label="Dia de fechamento" value={closingDay} onChange={(e) => setClosingDay(e.target.value)}>
              {[1, 5, 10, 15, 20, 25].map((d) => <option key={d} value={d}>Dia {d}</option>)}
            </Select>
            <Alert>{error}</Alert>
            <Button type="submit" variant="credit" loading={activate.isPending}>Ativar crédito</Button>
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
      <CreditHero line={l} usedPct={usedPct} />
      <div className="grid gap-6 lg:grid-cols-2">
        {toPay && (
          <Panel icon={CalendarClock} title={<span className="flex items-center gap-2">Fatura a pagar <Badge tone="amber">vence {dateBR(toPay.due_date)}</Badge></span>}>
            <p className="tabular text-2xl font-bold">{formatBRL(toPay.remaining)}</p>
            <Button className="mt-4" variant="credit" onClick={() => setOpen(toPay)}>Ver e pagar</Button>
          </Panel>
        )}
        {current && (
          <Panel title="Fatura atual" icon={Receipt}>
            <p className="tabular text-2xl font-bold">{formatBRL(current.total)}</p>
            <p className="text-sm text-muted">Fecha em {dateBR(current.closing_date)}</p>
            <Button className="mt-4" variant="secondary" onClick={() => setOpen(current)}>Ver lançamentos</Button>
          </Panel>
        )}
      </div>
      <LimitPanel base={base} line={l} />
      {!!plans.data?.length && (
        <Panel title="Compras parceladas" icon={Layers}
          action={l.upcoming_installments && <span className="text-sm text-muted">a vencer {formatBRL(l.upcoming_installments)}</span>}>
          <ul className="-my-2 divide-y divide-line">
            {plans.data.map((p) => (
              <li key={p.description + p.purchased_at} className="flex items-center gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-semibold text-fg">{p.description}</p>
                  <p className="text-xs text-muted">{p.installments}x de {formatBRL(p.installment_amount)} · {p.billed} de {p.installments} na fatura · {dateTimeBR(p.purchased_at)}</p>
                  <div className="mt-2 h-1 overflow-hidden rounded-full bg-surface-2">
                    <div className="h-full rounded-full bg-credit" style={{ width: `${(p.billed / p.installments) * 100}%` }} />
                  </div>
                </div>
                <div className="text-right">
                  <p className="tabular text-sm font-semibold text-fg">{formatBRL(p.remaining)}</p>
                  <p className="text-xs text-muted">a vencer</p>
                </div>
              </li>
            ))}
          </ul>
        </Panel>
      )}
      <Panel title="Faturas" icon={Receipt}>
        {invoices.isLoading ? <Loading /> : (
          <ul className="divide-y divide-line">
            {list.map((i) => (
              <li key={i.id}>
                <button onClick={() => setOpen(i)} className="-mx-2 flex w-[calc(100%+1rem)] items-center gap-3 rounded-xl px-2 py-3 text-left transition hover:bg-surface-2">
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
        <p className="mb-3 text-sm text-fg-2">Sem esperar o calendário:</p>
        <div className="flex flex-wrap gap-2">
          <Button variant="secondary" loading={cycle.isPending} onClick={() => cycle.mutate('CLOSE')}>Fechar a fatura agora</Button>
          <Button variant="secondary" loading={cycle.isPending} onClick={() => cycle.mutate('EXPIRE')}>Vencer a fatura fechada</Button>
        </div>
      </Sandbox>
      <InvoiceModal base={base} invoice={open} onClose={() => setOpen(null)} />
    </div>
  )
}
