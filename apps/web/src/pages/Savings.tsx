import { errorMessage, formatBRL, type SavingsBox, type SavingsMovement, type SavingsSummary } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDownLeft, ArrowUpRight, CalendarClock, Pencil, PiggyBank, Plus, Sparkles, Target, TrendingUp, X } from 'lucide-react'
import { useState } from 'react'
import { useAccount } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import { Alert, Button, Empty, Field, IconTile, Loading, Modal, PageTitle, Panel, Sandbox, Select, Stat, Tabs } from '../components/ui'
import { api } from '../lib/api'
import { cx, dateBR, dateTimeBR } from '../lib/format'

const rate = (bp?: number) => (bp ? `${(bp / 100).toLocaleString('pt-BR', { maximumFractionDigits: 2 })}%` : '')
const movementLabel = { DEPOSIT: 'Guardado', WITHDRAWAL: 'Resgate', YIELD: 'Rendimento' } as const

/** Share of the goal reached, 0-100. */
function progress(b: Pick<SavingsBox, 'balance' | 'goal'>): number | null {
  if (!b.goal) return null
  const goal = Number(b.goal.amount)
  return goal > 0 ? Math.min(100, (Number(b.balance.amount) / goal) * 100) : null
}

function BoxForm({ base, box, onDone }: { base: string; box?: SavingsBox; onDone: (b: SavingsBox) => void }) {
  const [name, setName] = useState(box?.name ?? '')
  const [goal, setGoal] = useState<string | null>(box?.goal?.amount ?? null)
  const [error, setError] = useState('')
  const save = useMutation({
    mutationFn: () => box
      ? api<SavingsBox>('PUT', `${base}/${box.id}`, { body: { name, goal: goal ?? '' } })
      : api<SavingsBox>('POST', base, { body: { name, goal: goal ?? '' }, idempotent: true }),
    onSuccess: onDone,
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
      <Field label="Nome" placeholder="Viagem, reserva de emergência…" maxLength={40} required value={name} onChange={(e) => setName(e.target.value)} />
      <MoneyField label="Meta" optional hint="Opcional: quanto você quer juntar." onValue={setGoal} />
      <Alert>{error}</Alert>
      <Button type="submit" loading={save.isPending} disabled={!name.trim()}>{box ? 'Salvar' : 'Criar caixinha'}</Button>
    </form>
  )
}

const days = Array.from({ length: 28 }, (_, i) => i + 1)

/** The monthly deposit: set, change or remove it. */
function SchedulePanel({ base, box, onChange }: { base: string; box: SavingsBox; onChange: () => void }) {
  const sc = box.schedule
  const [editing, setEditing] = useState(false)
  const [amount, setAmount] = useState<string | null>(null)
  const [day, setDay] = useState(sc?.day ?? 5)
  const [error, setError] = useState('')
  const save = useMutation({
    mutationFn: () => api<SavingsBox>('PUT', `${base}/${box.id}/schedule`, { body: { amount, day } }),
    onSuccess: () => { setEditing(false); setError(''); onChange() },
    onError: (e) => setError(errorMessage(e)),
  })
  const remove = useMutation({
    mutationFn: () => api<SavingsBox>('DELETE', `${base}/${box.id}/schedule`),
    onSuccess: () => { setEditing(false); onChange() },
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <Panel title="Depósito programado" icon={CalendarClock}
      action={sc && !editing && <Button variant="ghost" size="sm" icon={Pencil} onClick={() => setEditing(true)}>Alterar</Button>}>
      {sc && !editing ? (
        <div className="space-y-3">
          <p className="text-sm text-fg-2">
            <span className="tabular font-semibold text-fg">{formatBRL(sc.amount)}</span> todo dia {sc.day}, da conta para a caixinha.
          </p>
          <p className="text-xs text-muted">Próximo depósito em {dateBR(sc.next_run)}.</p>
          {sc.last_failure && (
            <Alert tone="amber">
              O último depósito não foi feito{sc.last_failure === 'INSUFFICIENT_FUNDS' ? ': faltou saldo na conta' : ''}. O próximo segue programado.
            </Alert>
          )}
        </div>
      ) : !editing ? (
        <div className="space-y-3">
          <p className="text-sm text-fg-2">Guarde todo mês sem precisar lembrar: escolha o valor e o dia.</p>
          <Button variant="secondary" icon={CalendarClock} onClick={() => setEditing(true)}>Programar depósito mensal</Button>
        </div>
      ) : (
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); save.mutate() }}>
          <MoneyField label="Quanto por mês" onValue={setAmount} />
          <Select label="Todo dia" value={day} onChange={(e) => setDay(Number(e.target.value))}>
            {days.map((d) => <option key={d} value={d}>{d}</option>)}
          </Select>
          <p className="text-xs text-muted">O primeiro depósito é no próximo dia {day}. Se faltar saldo nesse dia, o mês é pulado.</p>
          <Alert>{error}</Alert>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" loading={save.isPending} disabled={!amount}>{sc ? 'Salvar' : 'Programar'}</Button>
            <Button type="button" variant="ghost" onClick={() => { setEditing(false); setError('') }}>Cancelar</Button>
            {sc && <Button type="button" variant="ghost" loading={remove.isPending} onClick={() => remove.mutate()}>Remover</Button>}
          </div>
        </form>
      )}
    </Panel>
  )
}

function BoxCard({ box, selected, onSelect }: { box: SavingsBox; selected: boolean; onSelect: () => void }) {
  const pct = progress(box)
  return (
    <button onClick={onSelect} aria-pressed={selected}
      className={cx('glass w-full rounded-[28px] p-5 text-left transition hover:-translate-y-0.5', selected && 'ring-2 ring-brand')}>
      <div className="flex items-center gap-3">
        <IconTile icon={PiggyBank} tone="brand" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-[15px] font-semibold text-fg">{box.name}</p>
          <p className="text-xs text-muted">Rendeu {formatBRL(box.yielded)}</p>
        </div>
      </div>
      <p className="tabular mt-4 text-[26px] font-bold tracking-tight text-fg">{formatBRL(box.balance)}</p>
      {box.schedule && (
        <p className="mt-1 flex items-center gap-1 text-xs text-muted">
          <CalendarClock className="size-3.5" /> {formatBRL(box.schedule.amount)} todo dia {box.schedule.day}
        </p>
      )}
      {pct !== null && (
        <>
          <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-surface-2">
            <div className="h-full rounded-full bg-brand transition-all duration-700" style={{ width: `${pct}%` }} />
          </div>
          <p className="mt-2 text-xs text-muted">{Math.floor(pct)}% de {formatBRL(box.goal)}</p>
        </>
      )}
    </button>
  )
}

function BoxDetail({ base, accountId, id, onClosed }: { base: string; accountId: string; id: string; onClosed: () => void }) {
  const qc = useQueryClient()
  const [tab, setTab] = useState<'in' | 'out'>('in')
  const [amount, setAmount] = useState<string | null>(null)
  const [message, setMessage] = useState<{ tone: 'green' | 'red'; text: string } | null>(null)
  const [editing, setEditing] = useState(false)
  const [closing, setClosing] = useState(false)
  const detail = useQuery({ queryKey: ['savings', accountId, id], queryFn: () => api<SavingsBox>('GET', `${base}/${id}`) })
  const refresh = () => { for (const k of ['savings', 'balance', 'statement']) qc.invalidateQueries({ queryKey: [k] }) }
  const move = useMutation({
    mutationFn: () => api<{ movement: SavingsMovement }>('POST', `${base}/${id}/${tab === 'in' ? 'deposits' : 'withdrawals'}`, { body: { amount }, idempotent: true }),
    onSuccess: () => {
      setMessage({ tone: 'green', text: tab === 'in' ? `${formatBRL(amount!)} guardados.` : `${formatBRL(amount!)} voltaram para a conta.` })
      refresh()
    },
    onError: (e) => setMessage({ tone: 'red', text: errorMessage(e) }),
  })
  const close = useMutation({
    mutationFn: () => api('DELETE', `${base}/${id}`, { idempotent: true }),
    onSuccess: () => { refresh(); onClosed() },
    onError: (e) => setMessage({ tone: 'red', text: errorMessage(e) }),
  })
  const runSchedule = useMutation({
    mutationFn: () => api('POST', `/v1/sandbox/accounts/${accountId}/savings/${id}/schedule/run`),
    onSuccess: refresh,
    onError: (e) => { setMessage({ tone: 'red', text: errorMessage(e) }); refresh() },
  })
  const simulate = useMutation({
    mutationFn: () => api('POST', `/v1/sandbox/accounts/${accountId}/savings/${id}/yield`, { body: { days: 30 } }),
    onSuccess: refresh,
    onError: (e) => setMessage({ tone: 'red', text: errorMessage(e) }),
  })
  if (detail.isLoading) return <Panel><Loading rows={4} /></Panel>
  const b = detail.data
  if (!b) return null
  return (
    <div className="space-y-5">
      <Panel title={b.name} icon={PiggyBank}
        action={<Button variant="ghost" size="sm" icon={Pencil} onClick={() => setEditing(true)}>Editar</Button>}>
        <div className="grid grid-cols-2 gap-4">
          <Stat label="Saldo"><span className="tabular">{formatBRL(b.balance)}</span></Stat>
          <Stat label="Rendeu"><span className="tabular text-positive">{formatBRL(b.yielded)}</span></Stat>
        </div>
        <p className="mt-3 flex items-center gap-1.5 text-xs text-muted"><TrendingUp className="size-3.5" /> Rende {rate(b.annual_rate_bp)} ao ano, todo dia. Resgate quando quiser.</p>
        <form className="mt-5 space-y-3" onSubmit={(e) => { e.preventDefault(); setMessage(null); move.mutate() }}>
          <Tabs value={tab} onChange={(t) => { setTab(t); setMessage(null) }} tabs={[['in', 'Guardar'], ['out', 'Resgatar']]} />
          <MoneyField key={tab} label={tab === 'in' ? 'Quanto guardar' : 'Quanto resgatar'} onValue={setAmount} />
          <Button type="submit" icon={tab === 'in' ? ArrowDownLeft : ArrowUpRight} loading={move.isPending} disabled={!amount}>
            {tab === 'in' ? 'Guardar' : 'Resgatar'}
          </Button>
          {message && <Alert tone={message.tone}>{message.text}</Alert>}
        </form>
      </Panel>
      <SchedulePanel base={base} box={b} onChange={refresh} />
      <Panel title="Histórico" icon={Target}>
        {!b.movements?.length ? <Empty icon={PiggyBank}>Nada guardado ainda.</Empty> : (
          <ul className="-my-2 divide-y divide-line">
            {b.movements.map((m) => (
              <li key={m.id} className="flex items-center justify-between gap-3 py-3 text-sm">
                <div>
                  <p className="font-medium text-fg">{movementLabel[m.kind]}{m.status === 'PENDING' && ' · processando'}</p>
                  <p className="text-xs text-muted">{dateTimeBR(m.created_at)}</p>
                </div>
                <span className={cx('tabular font-semibold', m.kind === 'WITHDRAWAL' ? 'text-fg' : 'text-positive')}>
                  {m.kind === 'WITHDRAWAL' ? '− ' : '+ '}{formatBRL(m.amount)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Panel>
      <Sandbox>
        <p className="mb-3 text-sm text-fg-2">Veja quanto a caixinha renderia em 30 dias, no saldo de hoje.</p>
        <div className="flex flex-wrap gap-2">
          <Button variant="secondary" icon={Sparkles} loading={simulate.isPending} onClick={() => simulate.mutate()}>Simular 30 dias de rendimento</Button>
          {b.schedule && (
            <Button variant="secondary" icon={CalendarClock} loading={runSchedule.isPending} onClick={() => { setMessage(null); runSchedule.mutate() }}>
              Fazer o depósito programado agora
            </Button>
          )}
        </div>
      </Sandbox>
      <Button variant="ghost" icon={X} onClick={() => setClosing(true)}>Encerrar caixinha</Button>
      <Modal open={editing} onClose={() => setEditing(false)} title="Editar caixinha">
        <BoxForm base={base} box={b} onDone={() => { setEditing(false); refresh() }} />
      </Modal>
      <Modal open={closing} onClose={() => setClosing(false)} title="Encerrar caixinha">
        <p className="mb-5 text-sm text-fg-2">{formatBRL(b.balance)} voltam para a sua conta agora e a caixinha deixa de existir.</p>
        <Button variant="danger" loading={close.isPending} onClick={() => close.mutate()}>Encerrar e resgatar tudo</Button>
      </Modal>
    </div>
  )
}

export function SavingsPage() {
  const account = useAccount()
  const qc = useQueryClient()
  const [selected, setSelected] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const base = `/v1/accounts/${account?.id}/savings`
  const summary = useQuery({
    queryKey: ['savings', account?.id],
    enabled: !!account,
    queryFn: () => api<SavingsSummary>('GET', base),
  })
  if (!account || summary.isLoading) return <Loading />
  const s = summary.data!
  const current = s.boxes.find((b) => b.id === selected) ?? s.boxes[0]
  return (
    <div className="space-y-6">
      <PageTitle subtitle={`Separe dinheiro para seus planos. Rende ${rate(s.boxes[0]?.annual_rate_bp ?? 1065)} ao ano, todo dia, e você resgata quando quiser.`}
        action={<Button icon={Plus} onClick={() => setCreating(true)}>Nova caixinha</Button>}>Caixinhas</PageTitle>
      {s.boxes.length === 0 ? (
        <Panel>
          <Empty icon={PiggyBank}>Crie uma caixinha para a viagem, a reserva de emergência ou o que quiser.</Empty>
          <div className="flex justify-center pb-2"><Button icon={Plus} onClick={() => setCreating(true)}>Criar minha primeira caixinha</Button></div>
        </Panel>
      ) : (
        <>
          <div data-reveal className="grid grid-cols-2 gap-4 sm:max-w-md">
            <Stat label="Total guardado"><span className="tabular">{formatBRL(s.total)}</span></Stat>
            <Stat label="Já rendeu"><span className="tabular text-positive">{formatBRL(s.yielded)}</span></Stat>
          </div>
          <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_420px]">
            <div data-reveal className="grid content-start gap-4 sm:grid-cols-2">
              {s.boxes.map((b) => <BoxCard key={b.id} box={b} selected={b.id === current.id} onSelect={() => setSelected(b.id)} />)}
            </div>
            <BoxDetail key={current.id} base={base} accountId={account.id} id={current.id} onClosed={() => setSelected(null)} />
          </div>
        </>
      )}
      <Modal open={creating} onClose={() => setCreating(false)} title="Nova caixinha">
        <BoxForm base={base} onDone={(b) => { setCreating(false); setSelected(b.id); qc.invalidateQueries({ queryKey: ['savings'] }) }} />
      </Modal>
    </div>
  )
}
