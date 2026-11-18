import { errorMessage, formatBRL, type Account, type Money, type Profile } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Inbox, Landmark, Lock, LockOpen, LogOut, Search, ShieldAlert, UserRound, X } from 'lucide-react'
import { useState } from 'react'
import { useMe } from '../app/queries'
import { Aurora } from '../components/Aurora'
import { Brand } from '../components/Brand'
import { ThemeSwitch } from '../components/ThemeSwitch'
import { Alert, Badge, Button, Empty, Field, IconTile, Loading, Panel, Select } from '../components/ui'
import { api, logout } from '../lib/api'
import { cx, dateTimeBR } from '../lib/format'
import { useReveal } from '../lib/motion'

interface Review extends Profile { customer_id: string; reviewed_by: string; updated_at: string }
interface CustomerFile { customer_id: string; profile?: Review; accounts: { account: Account; available?: Money }[] }

const reasons: Record<string, string> = {
  PEP: 'Pessoa politicamente exposta', CPF_IRREGULAR: 'CPF irregular', UNDERAGE: 'Menor de idade',
  DOCUMENTS_NOT_CONFIRMED: 'Documentos não confirmados', FRAUD_SUSPECTED: 'Suspeita de fraude', SANCTIONS: 'Lista de sanções',
}
const statusTone = { APPROVED: 'green', REJECTED: 'red', MANUAL_REVIEW: 'amber', PENDING: 'brand', NOT_STARTED: 'slate' } as const

function Decision({ review, onDone }: { review: Review; onDone: () => void }) {
  const [reason, setReason] = useState('DOCUMENTS_NOT_CONFIRMED')
  const [error, setError] = useState('')
  const decide = useMutation({
    mutationFn: (approve: boolean) => api('POST', `/v1/backoffice/kyc/reviews/${review.customer_id}`, { body: { approve, reason: approve ? '' : reason } }),
    onSuccess: onDone,
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <Panel title="Decisão" icon={ShieldAlert}>
      <p className="mb-4 text-sm text-fg-2">Motivo da análise: <Badge tone="amber">{reasons[review.kyc_reason] ?? review.kyc_reason}</Badge></p>
      <div className="grid gap-3 sm:grid-cols-[1fr_auto_auto] sm:items-end">
        <Select label="Motivo da recusa" value={reason} onChange={(e) => setReason(e.target.value)}>
          {['DOCUMENTS_NOT_CONFIRMED', 'FRAUD_SUSPECTED', 'SANCTIONS'].map((r) => <option key={r} value={r}>{reasons[r]}</option>)}
        </Select>
        <Button variant="danger" icon={X} loading={decide.isPending} onClick={() => decide.mutate(false)}>Recusar</Button>
        <Button icon={Check} loading={decide.isPending} onClick={() => decide.mutate(true)}>Aprovar</Button>
      </div>
      <div className="mt-3"><Alert>{error}</Alert></div>
    </Panel>
  )
}

function AccountRow({ item, onChange }: { item: CustomerFile['accounts'][number]; onChange: () => void }) {
  const a = item.account
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const act = useMutation({
    mutationFn: () => a.status === 'ACTIVE'
      ? api('POST', `/v1/backoffice/accounts/${a.id}/block`, { body: { reason } })
      : api('POST', `/v1/backoffice/accounts/${a.id}/unblock`),
    onSuccess: () => { setReason(''); setError(''); onChange() },
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <li className="space-y-3 py-4">
      <div className="flex items-center gap-3">
        <IconTile icon={Landmark} tone={a.status === 'ACTIVE' ? 'brand' : 'red'} />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-fg">Ag {a.branch} · Conta {a.number}</p>
          <p className="text-xs text-muted">Saldo {formatBRL(item.available)} · aberta em {dateTimeBR(a.opened_at)}</p>
        </div>
        <Badge tone={a.status === 'ACTIVE' ? 'green' : 'red'}>{a.status === 'ACTIVE' ? 'Ativa' : a.status === 'BLOCKED' ? 'Bloqueada' : 'Encerrada'}</Badge>
      </div>
      {a.status !== 'CLOSED' && (
        <div className="flex flex-wrap items-end gap-2">
          {a.status === 'ACTIVE' && <div className="min-w-56 flex-1"><Field label="Motivo do bloqueio" value={reason} onChange={(e) => setReason(e.target.value)} /></div>}
          <Button variant={a.status === 'ACTIVE' ? 'danger' : 'secondary'} icon={a.status === 'ACTIVE' ? Lock : LockOpen}
            disabled={a.status === 'ACTIVE' && !reason.trim()} loading={act.isPending} onClick={() => act.mutate()}>
            {a.status === 'ACTIVE' ? 'Bloquear conta' : 'Desbloquear'}
          </Button>
        </div>
      )}
      <Alert>{error}</Alert>
    </li>
  )
}

function CustomerPanel({ id, onDecided }: { id: string; onDecided: () => void }) {
  const qc = useQueryClient()
  const file = useQuery({ queryKey: ['backoffice', 'customer', id], queryFn: () => api<CustomerFile>('GET', `/v1/backoffice/customers/${id}`) })
  const refresh = () => qc.invalidateQueries({ queryKey: ['backoffice'] })
  if (file.isLoading) return <Panel><Loading rows={4} /></Panel>
  if (file.isError) return <Panel><Alert>{errorMessage(file.error)}</Alert></Panel>
  const f = file.data!
  const p = f.profile
  return (
    <div className="space-y-5">
      <Panel title={p?.full_name ?? 'Cliente'} icon={UserRound}
        action={p && <Badge tone={statusTone[p.kyc_status]}>{p.kyc_status === 'MANUAL_REVIEW' ? 'Em análise' : p.kyc_status === 'APPROVED' ? 'Aprovado' : p.kyc_status === 'REJECTED' ? 'Recusado' : p.kyc_status}</Badge>}>
        {p ? (
          <dl className="grid gap-4 text-sm sm:grid-cols-2">
            {([['CPF', p.cpf], ['Nascimento', p.birth_date], ['Celular', p.phone || '—'],
              ['Endereço', `${p.address.street}, ${p.address.number} · ${p.address.city}/${p.address.state}`],
              ['Analista responsável', p.reviewed_by || '—'], ['Cliente', f.customer_id]] as const).map(([k, v]) => (
              <div key={k}><dt className="text-xs text-muted">{k}</dt><dd className="font-medium break-all text-fg">{v}</dd></div>
            ))}
          </dl>
        ) : <Empty icon={UserRound}>Este cliente ainda não preencheu o cadastro.</Empty>}
      </Panel>
      {p?.kyc_status === 'MANUAL_REVIEW' && <Decision review={p} onDone={() => { refresh(); onDecided() }} />}
      <Panel title="Contas" icon={Landmark}>
        {!f.accounts.length ? <Empty icon={Landmark}>Nenhuma conta aberta.</Empty> : (
          <ul className="-my-2 divide-y divide-line">{f.accounts.map((a) => <AccountRow key={a.account.id} item={a} onChange={refresh} />)}</ul>
        )}
      </Panel>
    </div>
  )
}

export function BackofficePage() {
  const me = useMe()
  const [selected, setSelected] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const queue = useQuery({
    queryKey: ['backoffice', 'reviews'],
    queryFn: async () => (await api<{ reviews: Review[] }>('GET', '/v1/backoffice/kyc/reviews')).reviews,
    refetchInterval: 15_000,
  })
  const scope = useReveal<HTMLElement>([])
  const list = queue.data ?? []
  return (
    <div className="min-h-full">
      <Aurora />
      <header className="glass sticky top-0 z-30 flex items-center justify-between gap-3 rounded-none border-x-0 border-t-0 px-5 py-3 sm:px-8">
        <div className="flex items-center gap-3"><Brand /><Badge tone="credit">Back office</Badge></div>
        <div className="flex items-center gap-2">
          <ThemeSwitch compact />
          <Button variant="ghost" size="sm" icon={LogOut} onClick={() => logout()}>Sair</Button>
        </div>
      </header>
      <main ref={scope} className="mx-auto grid max-w-7xl gap-6 px-4 py-8 sm:px-8 lg:grid-cols-[380px_1fr]">
        <div className="space-y-5">
          <div data-reveal>
            <h1 className="text-[34px] font-bold tracking-[-0.02em] text-fg">Análises</h1>
            <p className="text-sm text-muted">Cadastros que o bureau mandou para revisão manual. Analista {me.data?.customer_id.slice(0, 8)}.</p>
          </div>
          <form data-reveal className="flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); if (search.trim()) setSelected(search.trim()) }}>
            <div className="flex-1"><Field label="Buscar cliente pelo id" value={search} onChange={(e) => setSearch(e.target.value)} placeholder="id do cliente" /></div>
            <Button type="submit" variant="secondary" icon={Search}>Abrir</Button>
          </form>
          <Panel title={`Análises pendentes · ${list.length}`} icon={Inbox}>
            {queue.isLoading ? <Loading /> : !list.length ? <Empty icon={Inbox}>Nenhum cadastro esperando análise.</Empty> : (
              <ul className="-mx-2 space-y-1">
                {list.map((r) => (
                  <li key={r.customer_id}>
                    <button onClick={() => setSelected(r.customer_id)}
                      className={cx('flex w-full items-center gap-3 rounded-2xl px-3 py-3 text-left transition',
                        selected === r.customer_id ? 'glass-strong' : 'hover:bg-surface-2')}>
                      <IconTile icon={ShieldAlert} tone="amber" />
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-semibold text-fg">{r.full_name}</p>
                        <p className="text-xs text-muted">{r.cpf} · {reasons[r.kyc_reason] ?? r.kyc_reason}</p>
                      </div>
                      <span className="text-xs text-subtle">{dateTimeBR(r.updated_at)}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>
        <div>
          {selected ? <CustomerPanel key={selected} id={selected} onDecided={() => setSelected(selected)} /> : (
            <Panel><Empty icon={UserRound}>Escolha um cadastro na fila ou busque um cliente.</Empty></Panel>
          )}
        </div>
      </main>
    </div>
  )
}
