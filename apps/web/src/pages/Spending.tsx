import { budgetLabel, budgetState, errorMessage, formatBRL, monthLabel, shiftMonth, spendingCategories, type Budget, type BudgetCategory, type SpendingSummary } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, PieChart, Plus, Store, Target, TrendingDown, TrendingUp, X } from 'lucide-react'
import { useState } from 'react'
import { useAccount } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import { Alert, Button, Empty, IconButton, Loading, Modal, PageTitle, Panel, Select } from '../components/ui'
import { cx } from '../lib/format'
import { api } from '../lib/api'

const currentMonth = () => {
  const d = new Date(Date.now() - 3 * 3600_000) // Brasília
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

/** A donut of the categories; the total sits in the middle. */
function Donut({ data, total }: { data: SpendingSummary['categories']; total: number }) {
  const r = 70
  const c = 2 * Math.PI * r
  const lengths = data.map((d) => (Number(d.amount.amount) / total) * c)
  const offsets = lengths.map((_, i) => lengths.slice(0, i).reduce((a, b) => a + b, 0))
  return (
    <svg viewBox="0 0 180 180" className="size-44 shrink-0 -rotate-90" role="img" aria-label="Gastos por categoria">
      <circle cx="90" cy="90" r={r} fill="none" stroke="var(--surface-2)" strokeWidth="22" />
      {data.map((d, i) => (
        <circle key={d.category} cx="90" cy="90" r={r} fill="none" stroke={spendingCategories[d.category].color} strokeWidth="22"
          strokeDasharray={`${Math.max(lengths[i] - 2, 0)} ${c}`} strokeDashoffset={-offsets[i]} />
      ))}
    </svg>
  )
}

const stateFill = { ok: 'bg-positive', near: 'bg-warning', over: 'bg-danger' } as const
const budgetCategories: BudgetCategory[] = ['TOTAL', ...(Object.keys(spendingCategories) as BudgetCategory[])]

function BudgetForm({ base, taken, onDone }: { base: string; taken: Set<string>; onDone: () => void }) {
  const free = budgetCategories.filter((c) => !taken.has(c))
  const [category, setCategory] = useState<BudgetCategory>(free[0] ?? 'TOTAL')
  const [amount, setAmount] = useState<string | null>(null)
  const [error, setError] = useState('')
  const save = useMutation({
    mutationFn: () => api('PUT', `${base}/${category}`, { body: { amount } }),
    onSuccess: onDone,
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
      <Select label="Categoria" value={category} onChange={(e) => setCategory(e.target.value as BudgetCategory)}>
        {free.map((c) => <option key={c} value={c}>{budgetLabel(c)}</option>)}
      </Select>
      <MoneyField label="Quanto por mês" onValue={setAmount} />
      <p className="text-xs text-muted">Avisamos quando você chegar a 80% e a 100% do valor no mês.</p>
      <Alert>{error}</Alert>
      <Button type="submit" loading={save.isPending} disabled={!amount}>Definir orçamento</Button>
    </form>
  )
}

/** Monthly budgets with how much of each was spent in the month shown. */
function BudgetsPanel({ accountId, month, current }: { accountId: string; month: string; current: boolean }) {
  const qc = useQueryClient()
  const [adding, setAdding] = useState(false)
  const base = `/v1/accounts/${accountId}/budgets`
  const budgets = useQuery({
    queryKey: ['budgets', accountId, month],
    queryFn: async () => (await api<{ budgets: Budget[] }>('GET', `${base}?month=${month}`)).budgets,
  })
  const refresh = () => qc.invalidateQueries({ queryKey: ['budgets'] })
  const remove = useMutation({ mutationFn: (c: string) => api('DELETE', `${base}/${c}`), onSuccess: refresh })
  const list = budgets.data ?? []
  return (
    <Panel title="Orçamentos" icon={Target}
      action={current && list.length < budgetCategories.length && <Button variant="ghost" size="sm" icon={Plus} onClick={() => setAdding(true)}>Novo</Button>}>
      {budgets.isLoading ? <Loading rows={2} /> : !list.length ? (
        <div className="space-y-3">
          <Empty icon={Target}>Defina quanto quer gastar por mês, no total ou por categoria, e receba um aviso antes de passar do ponto.</Empty>
          {current && <div className="flex justify-center"><Button icon={Plus} onClick={() => setAdding(true)}>Criar orçamento</Button></div>}
        </div>
      ) : (
        <ul className="-my-2 divide-y divide-line">
          {list.map((b) => {
            const st = budgetState(b)
            return (
              <li key={b.category} className="py-3 text-sm">
                <div className="flex items-center gap-2">
                  <span className="flex-1 font-medium text-fg">{budgetLabel(b.category)}</span>
                  <span className="tabular text-fg-2"><span className="font-semibold text-fg">{formatBRL(b.spent)}</span> de {formatBRL(b.amount)}</span>
                  {current && <IconButton icon={X} label={`Remover orçamento de ${budgetLabel(b.category)}`} onClick={() => remove.mutate(b.category)} />}
                </div>
                <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-surface-2" role="meter" aria-label={budgetLabel(b.category)}
                  aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.min(100, Math.round(st.pct))}>
                  <div className={cx('h-full rounded-full', stateFill[st.state])} style={{ width: `${Math.min(100, st.pct)}%` }} />
                </div>
                <p className="mt-1 text-xs text-muted">{Math.round(st.pct)}% · {st.label}</p>
              </li>
            )
          })}
        </ul>
      )}
      <Modal open={adding} onClose={() => setAdding(false)} title="Novo orçamento">
        <BudgetForm base={base} taken={new Set(list.map((b) => b.category))} onDone={() => { setAdding(false); refresh() }} />
      </Modal>
    </Panel>
  )
}

export function SpendingPage() {
  const account = useAccount()
  const [month, setMonth] = useState(currentMonth)
  const summary = useQuery({
    queryKey: ['spending', account?.id, month],
    enabled: !!account,
    queryFn: () => api<SpendingSummary>('GET', `/v1/accounts/${account!.id}/spending?month=${month}`),
  })
  const s = summary.data
  const total = Number(s?.total.amount ?? 0)
  const previous = Number(s?.previous_total.amount ?? 0)
  const change = previous > 0 ? ((total - previous) / previous) * 100 : null
  return (
    <div className="space-y-6">
      <PageTitle subtitle="Para onde foi o seu dinheiro: compras no cartão por categoria, Pix e contas pagas.">Gastos</PageTitle>
      <div data-reveal className="flex items-center gap-2">
        <IconButton icon={ChevronLeft} label="Mês anterior" onClick={() => setMonth(shiftMonth(month, -1))} />
        <span className="min-w-44 text-center text-[15px] font-semibold text-fg">{monthLabel(month)}</span>
        <IconButton icon={ChevronRight} label="Próximo mês" disabled={month >= currentMonth()} onClick={() => setMonth(shiftMonth(month, 1))} />
      </div>
      {summary.isLoading || !s ? <Loading rows={4} /> : (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_380px]">
          <Panel title="Por categoria" icon={PieChart}>
            {total === 0 ? <Empty icon={PieChart}>Nenhum gasto neste mês.</Empty> : (
              <div className="flex flex-col items-center gap-6 sm:flex-row sm:items-start">
                <div className="relative">
                  <Donut data={s.categories} total={total} />
                  <div className="absolute inset-0 grid place-items-center text-center">
                    <div>
                      <p className="text-xs text-muted">Total</p>
                      <p className="tabular text-lg font-bold text-fg">{formatBRL(s.total)}</p>
                    </div>
                  </div>
                </div>
                <ul className="w-full space-y-3">
                  {s.categories.map((c) => {
                    const pct = (Number(c.amount.amount) / total) * 100
                    const meta = spendingCategories[c.category]
                    return (
                      <li key={c.category}>
                        <div className="flex items-center gap-2 text-sm">
                          <span className="size-2.5 shrink-0 rounded-full" style={{ background: meta.color }} />
                          <span className="flex-1 font-medium text-fg">{meta.label}</span>
                          <span className="text-xs text-muted">{Math.round(pct)}%</span>
                          <span className="tabular w-24 text-right font-semibold text-fg">{formatBRL(c.amount)}</span>
                        </div>
                        <div className="mt-1.5 ml-4.5 h-1.5 overflow-hidden rounded-full bg-surface-2">
                          <div className="h-full rounded-full" style={{ width: `${pct}%`, background: meta.color }} />
                        </div>
                      </li>
                    )
                  })}
                </ul>
              </div>
            )}
          </Panel>
          <div className="space-y-6">
            <BudgetsPanel accountId={account!.id} month={month} current={month === currentMonth()} />
            <Panel>
              <p className="text-sm text-muted">Gasto em {monthLabel(month).split(' ')[0].toLowerCase()}</p>
              <p className="tabular text-[32px] font-bold tracking-tight text-fg">{formatBRL(s.total)}</p>
              {change !== null && (
                <p className={`mt-1 flex items-center gap-1.5 text-sm font-medium ${change > 0 ? 'text-danger' : 'text-positive'}`}>
                  {change > 0 ? <TrendingUp className="size-4" /> : <TrendingDown className="size-4" />}
                  {Math.abs(Math.round(change))}% {change > 0 ? 'a mais' : 'a menos'} que no mês anterior ({formatBRL(s.previous_total)})
                </p>
              )}
            </Panel>
            <Panel title="Onde você mais gastou" icon={Store}>
              {!s.merchants.length ? <Empty icon={Store}>Sem compras neste mês.</Empty> : (
                <ol className="-my-2 divide-y divide-line">
                  {s.merchants.map((m, i) => (
                    <li key={m.name} className="flex items-center gap-3 py-3 text-sm">
                      <span className="w-4 text-xs font-semibold text-subtle">{i + 1}</span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-semibold text-fg">{m.name}</p>
                        <p className="text-xs text-muted">{spendingCategories[m.category].label} · {m.count} {m.count === 1 ? 'compra' : 'compras'}</p>
                      </div>
                      <span className="tabular font-semibold text-fg">{formatBRL(m.amount)}</span>
                    </li>
                  ))}
                </ol>
              )}
            </Panel>
          </div>
        </div>
      )}
    </div>
  )
}
