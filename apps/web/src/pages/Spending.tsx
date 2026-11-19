import { formatBRL, monthLabel, shiftMonth, spendingCategories, type SpendingSummary } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, PieChart, Store, TrendingDown, TrendingUp } from 'lucide-react'
import { useState } from 'react'
import { useAccount } from '../app/queries'
import { Empty, IconButton, Loading, PageTitle, Panel } from '../components/ui'
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
