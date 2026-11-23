import { errorMessage, formatBRL, savingsMovementLabel, type SavingsBox, type SavingsSummary } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDownLeft, ArrowUpRight, CalendarClock, PiggyBank, Plus, Sparkles, X } from 'lucide-react-native'
import { useState } from 'react'
import { View } from 'react-native'
import { Button, Card, Field, Loading, MoneyField, Muted, Notice, Press, Row, Sandbox, Screen, Segmented, Toggle, Txt } from '../components/ui'
import { api } from '../lib/client'
import { date, dateTime, useAccount } from '../lib/queries'
import { font, useTheme } from '../lib/theme'

const rate = (bp: number) => `${(bp / 100).toLocaleString('pt-BR', { maximumFractionDigits: 2 })}%`

function NewBox({ base, onDone }: { base: string; onDone: (b: SavingsBox) => void }) {
  const [name, setName] = useState('')
  const [goal, setGoal] = useState<string | null>(null)
  const [error, setError] = useState('')
  const create = useMutation({
    mutationFn: () => api<SavingsBox>('POST', base, { body: { name, goal: goal ?? '' }, idempotent: true }),
    onSuccess: onDone,
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <Card title="Nova caixinha" icon={Plus}>
      <Field label="Nome" placeholder="Viagem, reserva…" maxLength={40} value={name} onChangeText={setName} />
      <MoneyField label="Meta (opcional)" onValue={setGoal} />
      <Notice>{error}</Notice>
      <Button title="Criar caixinha" icon={Plus} disabled={!name.trim()} loading={create.isPending} onPress={() => create.mutate()} />
    </Card>
  )
}

/** The monthly deposit: set, change or remove it. */
function ScheduleCard({ base, box, onChange }: { base: string; box: SavingsBox; onChange: () => void }) {
  const sc = box.schedule
  const [editing, setEditing] = useState(false)
  const [amount, setAmount] = useState<string | null>(null)
  const [day, setDay] = useState(String(sc?.day ?? 5))
  const [error, setError] = useState('')
  const n = Number(day)
  const validDay = Number.isInteger(n) && n >= 1 && n <= 28
  const save = useMutation({
    mutationFn: () => api('PUT', `${base}/${box.id}/schedule`, { body: { amount, day: n } }),
    onSuccess: () => { setEditing(false); setError(''); onChange() },
    onError: (e) => setError(errorMessage(e)),
  })
  const remove = useMutation({
    mutationFn: () => api('DELETE', `${base}/${box.id}/schedule`),
    onSuccess: () => { setEditing(false); onChange() },
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <Card title="Depósito programado" icon={CalendarClock} index={2}>
      {sc && !editing ? (
        <>
          <Txt>{formatBRL(sc.amount)} todo dia {sc.day}, da conta para a caixinha.</Txt>
          <Muted>Próximo depósito em {date(sc.next_run)}.</Muted>
          {!!sc.last_failure && (
            <Notice tone="amber">O último depósito não foi feito{sc.last_failure === 'INSUFFICIENT_FUNDS' ? ': faltou saldo na conta' : ''}. O próximo segue programado.</Notice>
          )}
          <Button title="Alterar" variant="secondary" onPress={() => setEditing(true)} />
        </>
      ) : !editing ? (
        <>
          <Muted>Guarde todo mês sem precisar lembrar: escolha o valor e o dia.</Muted>
          <Button title="Programar depósito mensal" icon={CalendarClock} variant="secondary" onPress={() => setEditing(true)} />
        </>
      ) : (
        <>
          <MoneyField label="Quanto por mês" onValue={setAmount} />
          <Field label="Todo dia (1 a 28)" keyboardType="number-pad" maxLength={2} value={day} onChangeText={setDay}
            error={day && !validDay ? 'Escolha um dia de 1 a 28' : undefined} />
          <Muted>Se faltar saldo no dia, o mês é pulado.</Muted>
          <Notice>{error}</Notice>
          <Button title={sc ? 'Salvar' : 'Programar'} disabled={!amount || !validDay} loading={save.isPending} onPress={() => save.mutate()} />
          {sc && <Button title="Remover" variant="ghost" loading={remove.isPending} onPress={() => remove.mutate()} />}
          <Button title="Cancelar" variant="ghost" onPress={() => { setEditing(false); setError('') }} />
        </>
      )}
    </Card>
  )
}

function BoxDetail({ base, accountId, id, onClosed }: { base: string; accountId: string; id: string; onClosed: () => void }) {
  const qc = useQueryClient()
  const [tab, setTab] = useState<'in' | 'out'>('in')
  const [amount, setAmount] = useState<string | null>(null)
  const [message, setMessage] = useState<{ tone: 'green' | 'red'; text: string } | null>(null)
  const detail = useQuery({ queryKey: ['savings', accountId, id], queryFn: () => api<SavingsBox>('GET', `${base}/${id}`) })
  const refresh = () => { for (const k of ['savings', 'balance', 'statement']) qc.invalidateQueries({ queryKey: [k] }) }
  const fail = (e: unknown) => setMessage({ tone: 'red', text: errorMessage(e) })
  const move = useMutation({
    mutationFn: () => api('POST', `${base}/${id}/${tab === 'in' ? 'deposits' : 'withdrawals'}`, { body: { amount }, idempotent: true }),
    onSuccess: () => { setMessage({ tone: 'green', text: tab === 'in' ? `${formatBRL(amount!)} guardados.` : `${formatBRL(amount!)} voltaram para a conta.` }); refresh() },
    onError: fail,
  })
  const close = useMutation({ mutationFn: () => api('DELETE', `${base}/${id}`, { idempotent: true }), onSuccess: () => { refresh(); onClosed() }, onError: fail })
  const roundUp = useMutation({ mutationFn: (enabled: boolean) => api('PUT', `${base}/${id}/round-up`, { body: { enabled } }), onSuccess: refresh, onError: fail })
  const runSchedule = useMutation({
    mutationFn: () => api('POST', `/v1/sandbox/accounts/${accountId}/savings/${id}/schedule/run`),
    onSuccess: refresh,
    onError: (e) => { fail(e); refresh() },
  })
  const simulate = useMutation({ mutationFn: () => api('POST', `/v1/sandbox/accounts/${accountId}/savings/${id}/yield`, { body: { days: 30 } }), onSuccess: refresh, onError: fail })
  const b = detail.data
  if (!b) return <Loading />
  return (
    <>
      <Card title={b.name} icon={PiggyBank} index={1}>
        <View style={{ flexDirection: 'row', gap: 24 }}>
          <View><Txt variant="caption" tone="muted">Saldo</Txt><Txt variant="heading" style={{ fontSize: 22 }}>{formatBRL(b.balance)}</Txt></View>
          <View><Txt variant="caption" tone="muted">Rendeu</Txt><Txt variant="heading" tone="positive" style={{ fontSize: 22 }}>{formatBRL(b.yielded)}</Txt></View>
        </View>
        <Txt variant="caption" tone="muted">Rende {rate(b.annual_rate_bp)} ao ano, todo dia. Resgate quando quiser.</Txt>
        <Segmented value={tab} onChange={(t) => { setTab(t); setMessage(null) }} options={[['in', 'Guardar'], ['out', 'Resgatar']]} />
        <MoneyField key={tab} label={tab === 'in' ? 'Quanto guardar' : 'Quanto resgatar'} onValue={setAmount} />
        <Button title={tab === 'in' ? 'Guardar' : 'Resgatar'} icon={tab === 'in' ? ArrowDownLeft : ArrowUpRight} disabled={!amount} loading={move.isPending} onPress={() => move.mutate()} />
        {message && <Notice tone={message.tone}>{message.text}</Notice>}
      </Card>
      <ScheduleCard base={base} box={b} onChange={refresh} />
      <Card index={3}>
        <Toggle label="Arredondar compras no débito" description="R$ 12,30 no débito guarda R$ 0,70 aqui." value={!!b.round_up} onChange={(on) => roundUp.mutate(on)} />
      </Card>
      <Card title="Histórico" index={3}>
        {!b.movements?.length ? <Muted>Nada guardado ainda.</Muted> : b.movements.map((m) => (
          <Row key={m.id} title={savingsMovementLabel(m) + (m.status === 'PENDING' ? ' · processando' : '')} subtitle={dateTime(m.created_at)}
            right={<Txt variant="label" tone={m.kind === 'WITHDRAWAL' ? 'fg' : 'positive'} style={{ fontFamily: font.semibold }}>{m.kind === 'WITHDRAWAL' ? '− ' : '+ '}{formatBRL(m.amount)}</Txt>} />
        ))}
      </Card>
      <Sandbox>
        <Button title="Simular 30 dias de rendimento" icon={Sparkles} variant="secondary" loading={simulate.isPending} onPress={() => simulate.mutate()} />
        {b.schedule && (
          <Button title="Fazer o depósito programado agora" icon={CalendarClock} variant="secondary" loading={runSchedule.isPending}
            onPress={() => { setMessage(null); runSchedule.mutate() }} />
        )}
      </Sandbox>
      <Button title="Encerrar e resgatar tudo" icon={X} variant="danger" loading={close.isPending} onPress={() => close.mutate()} />
    </>
  )
}

export default function Savings() {
  const { c } = useTheme()
  const account = useAccount()
  const qc = useQueryClient()
  const [selected, setSelected] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const base = `/v1/accounts/${account?.id}/savings`
  const summary = useQuery({ queryKey: ['savings', account?.id], enabled: !!account, queryFn: () => api<SavingsSummary>('GET', base) })
  if (!account || summary.isLoading || !summary.data) return <Screen><Loading /></Screen>
  const s = summary.data
  const current = s.boxes.find((b) => b.id === selected) ?? s.boxes[0]
  return (
    <Screen>
      {s.boxes.length > 0 && (
        <View style={{ flexDirection: 'row', gap: 24, paddingHorizontal: 4 }}>
          <View><Txt variant="caption" tone="muted">Total guardado</Txt><Txt variant="title" style={{ fontSize: 28 }}>{formatBRL(s.total)}</Txt></View>
          <View><Txt variant="caption" tone="muted">Já rendeu</Txt><Txt variant="title" tone="positive" style={{ fontSize: 28 }}>{formatBRL(s.yielded)}</Txt></View>
        </View>
      )}
      {(creating || s.boxes.length === 0) ? (
        <NewBox base={base} onDone={(b) => { setCreating(false); setSelected(b.id); qc.invalidateQueries({ queryKey: ['savings'] }) }} />
      ) : (
        <Button title="Nova caixinha" icon={Plus} variant="secondary" onPress={() => setCreating(true)} />
      )}
      {s.boxes.map((b) => {
        const pct = b.goal ? Math.min(100, (Number(b.balance.amount) / Number(b.goal.amount)) * 100) : null
        return (
          <Press key={b.id} accessibilityLabel={b.name} onPress={() => setSelected(b.id)}>
            <Card style={b.id === current?.id ? { borderColor: c.brand, borderWidth: 2 } : undefined}>
              <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline' }}>
                <Txt variant="heading">{b.name}</Txt>
                <Txt variant="heading">{formatBRL(b.balance)}</Txt>
              </View>
              {b.schedule && <Txt variant="caption" tone="muted">{formatBRL(b.schedule.amount)} todo dia {b.schedule.day}</Txt>}
              {pct !== null && (
                <View style={{ gap: 6 }}>
                  <View style={{ height: 6, borderRadius: 3, backgroundColor: c.surface2, overflow: 'hidden' }}>
                    <View style={{ height: 6, width: `${pct}%`, backgroundColor: c.brand }} />
                  </View>
                  <Txt variant="caption" tone="muted">{Math.floor(pct)}% de {formatBRL(b.goal)} · rendeu {formatBRL(b.yielded)}</Txt>
                </View>
              )}
            </Card>
          </Press>
        )
      })}
      {current && <BoxDetail key={current.id} base={base} accountId={account.id} id={current.id} onClosed={() => setSelected(null)} />}
    </Screen>
  )
}
