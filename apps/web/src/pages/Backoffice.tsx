import { disputeReasons, errorMessage, formatBRL, type Account, type Dispute, type Money, type Profile } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, CreditCard, FileClock, Inbox, Landmark, Lock, LockOpen, LogOut, Search, ShieldAlert, ShieldCheck, UserRound, Users, X } from 'lucide-react'
import { useState } from 'react'
import { useMe } from '../app/queries'
import { Aurora } from '../components/Aurora'
import { Brand } from '../components/Brand'
import { ThemeSwitch } from '../components/ThemeSwitch'
import { Alert, Badge, Button, Empty, Field, IconTile, Loading, Panel, Select } from '../components/ui'
import { api, logout } from '../lib/api'
import { cx, dateTimeBR } from '../lib/format'
import { useReveal } from '../lib/motion'
import { TOTP } from './Settings'

interface Review extends Profile { customer_id: string; reviewed_by: string; updated_at: string }
interface CustomerFile { customer_id: string; profile?: Review; accounts: { account: Account; available?: Money }[] }
interface Approval {
  id: string; kind: 'UNBLOCK_ACCOUNT'; target_id: string; reason: string; requested_by: string
  status: 'PENDING' | 'APPROVED' | 'EXECUTED' | 'REJECTED' | 'FAILED' | 'EXPIRED'
  decided_by: string; decision_reason: string; failure_reason: string; created_at: string; expires_at: string
  customer_id?: string; account?: Account
}
interface AuditEvent { seq: number; actor_id: string; action: string; target_type: string; target_id: string; details: Record<string, string>; at: string }

const actionLabels: Record<string, string> = {
  CUSTOMER_FILE_VIEW: 'Ficha consultada', KYC_APPROVE: 'Cadastro aprovado', KYC_REJECT: 'Cadastro recusado',
  ACCOUNT_BLOCK: 'Conta bloqueada', APPROVAL_REQUESTED: 'Desbloqueio solicitado', APPROVAL_APPROVED: 'Desbloqueio aprovado',
  APPROVAL_REJECTED: 'Desbloqueio recusado', APPROVAL_EXECUTED: 'Conta desbloqueada', APPROVAL_FAILED: 'Desbloqueio falhou',
  APPROVAL_EXPIRED: 'Pedido expirado', DISPUTE_ACCEPT: 'Contestação aceita (estorno)', DISPUTE_REJECT: 'Contestação recusada',
}
const actionLabel = (a: string) => actionLabels[a] ?? (a.endsWith('_FAILED') ? `${actionLabels[a.slice(0, -7)] ?? a} (falhou)` : a)
const short = (id: string) => id.slice(0, 8)

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
  const [sent, setSent] = useState(false)
  const blocking = a.status === 'ACTIVE'
  const act = useMutation({
    mutationFn: () => api('POST', `/v1/backoffice/accounts/${a.id}/${blocking ? 'block' : 'unblock'}`, { body: { reason } }),
    onSuccess: () => { setReason(''); setError(''); setSent(!blocking); onChange() },
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
          <div className="min-w-56 flex-1">
            <Field label={blocking ? 'Motivo do bloqueio' : 'Motivo do desbloqueio'} value={reason} onChange={(e) => setReason(e.target.value)}
              hint={blocking ? undefined : 'Outro analista precisa aprovar o desbloqueio.'} />
          </div>
          <Button variant={blocking ? 'danger' : 'secondary'} icon={blocking ? Lock : LockOpen}
            disabled={!reason.trim()} loading={act.isPending} onClick={() => act.mutate()}>
            {blocking ? 'Bloquear conta' : 'Pedir desbloqueio'}
          </Button>
        </div>
      )}
      {sent && <Alert tone="green">Pedido enviado. A conta é desbloqueada quando outro analista aprovar.</Alert>}
      <Alert>{error}</Alert>
    </li>
  )
}

function AuditPanel({ targets }: { targets: string[] }) {
  const trail = useQuery({
    queryKey: ['backoffice', 'audit', ...targets],
    queryFn: async () => {
      const lists = await Promise.all(targets.map((t) => api<{ events: AuditEvent[] }>('GET', `/v1/backoffice/audit?target_id=${t}&limit=50`)))
      return lists.flatMap((l) => l.events).sort((x, y) => y.seq - x.seq)
    },
  })
  const chain = useQuery({ queryKey: ['backoffice', 'audit-verify'], queryFn: () => api<{ intact: boolean; events: number; broken_at: number }>('GET', '/v1/backoffice/audit/verify') })
  return (
    <Panel title="Trilha de auditoria" icon={FileClock}
      action={chain.data && (chain.data.intact
        ? <Badge tone="green">Cadeia íntegra · {chain.data.events} eventos</Badge>
        : <Badge tone="red">Cadeia quebrada no evento {chain.data.broken_at}</Badge>)}>
      {trail.isLoading ? <Loading /> : !trail.data?.length ? <Empty icon={FileClock}>Nenhuma ação registrada.</Empty> : (
        <ol className="space-y-3">
          {trail.data.map((e) => (
            <li key={e.seq} className="flex items-start gap-3 text-sm">
              <span className="mt-0.5 w-12 shrink-0 font-mono text-xs text-subtle">#{e.seq}</span>
              <div className="min-w-0 flex-1">
                <p className="font-medium text-fg">{actionLabel(e.action)}</p>
                <p className="text-xs text-muted">
                  {e.actor_id === 'system' ? 'Sistema' : `Analista ${short(e.actor_id)}`} · {dateTimeBR(e.at)}
                  {e.details.reason ? ` · ${reasons[e.details.reason] ?? e.details.reason}` : ''}
                  {e.details.error ? ` · ${e.details.error}` : ''}
                </p>
              </div>
            </li>
          ))}
        </ol>
      )}
    </Panel>
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
      <AuditPanel targets={[f.customer_id, ...f.accounts.map((a) => a.account.id)]} />
    </div>
  )
}

function ApprovalItem({ item, me, onOpen }: { item: Approval; me?: string; onOpen: () => void }) {
  const qc = useQueryClient()
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const decide = useMutation({
    mutationFn: (approve: boolean) => api<Approval>('POST', `/v1/backoffice/approvals/${item.id}/${approve ? 'approve' : 'reject'}`, { body: { reason } }),
    onSuccess: (a) => {
      setError(a.status === 'FAILED' ? `Aprovado, mas o desbloqueio falhou: ${a.failure_reason}` : '')
      qc.invalidateQueries({ queryKey: ['backoffice'] })
    },
    onError: (e) => setError(errorMessage(e)),
  })
  const mine = item.requested_by === me
  return (
    <li className="space-y-3 rounded-2xl px-3 py-3 hover:bg-surface-2">
      <div className="flex items-start gap-3">
        <IconTile icon={LockOpen} tone="credit" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-fg">
            Desbloquear {item.account ? `${item.account.holder_name} · conta ${item.account.number}` : `conta ${short(item.target_id)}`}
          </p>
          <p className="text-xs text-muted">{item.reason} · por {mine ? 'você' : `analista ${short(item.requested_by)}`} · expira {dateTimeBR(item.expires_at)}</p>
        </div>
      </div>
      {mine ? <p className="text-xs text-subtle">Aguardando outro analista (quatro olhos).</p> : rejecting ? (
        <form className="flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); decide.mutate(false) }}>
          <div className="flex-1"><Field label="Motivo da recusa" value={reason} onChange={(e) => setReason(e.target.value)} /></div>
          <Button type="submit" variant="danger" size="sm" disabled={!reason.trim()} loading={decide.isPending}>Recusar</Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => setRejecting(false)}>Voltar</Button>
        </form>
      ) : (
        <div className="flex flex-wrap gap-2">
          <Button size="sm" icon={Check} loading={decide.isPending} onClick={() => decide.mutate(true)}>Aprovar</Button>
          <Button size="sm" variant="secondary" icon={X} onClick={() => setRejecting(true)}>Recusar</Button>
          {item.customer_id && <Button size="sm" variant="ghost" onClick={onOpen}>Ver cliente</Button>}
        </div>
      )}
      <Alert>{error}</Alert>
    </li>
  )
}

function ApprovalsPanel({ me, onOpenCustomer }: { me?: string; onOpenCustomer: (customerId: string) => void }) {
  const approvals = useQuery({
    queryKey: ['backoffice', 'approvals'],
    queryFn: async () => (await api<{ approvals: Approval[] }>('GET', '/v1/backoffice/approvals')).approvals,
    refetchInterval: 15_000,
  })
  const list = approvals.data ?? []
  return (
    <Panel title={`Aprovações · ${list.length}`} icon={Users}>
      {approvals.isLoading ? <Loading /> : !list.length ? <Empty icon={Users}>Nenhum pedido esperando um segundo analista.</Empty> : (
        <ul className="-mx-2 space-y-1">{list.map((a) => <ApprovalItem key={a.id} item={a} me={me} onOpen={() => a.customer_id && onOpenCustomer(a.customer_id)} />)}</ul>
      )}
    </Panel>
  )
}

function DisputeItem({ item, onOpen }: { item: Dispute & { customer_id?: string; holder_name?: string }; onOpen: () => void }) {
  const qc = useQueryClient()
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const decide = useMutation({
    mutationFn: (won: boolean) => api('POST', `/v1/backoffice/disputes/${item.id}/decide`, { body: { won, reason } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backoffice'] }),
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <li className="space-y-3 rounded-2xl px-3 py-3 hover:bg-surface-2">
      <div className="flex items-start gap-3">
        <IconTile icon={CreditCard} tone={item.reason === 'NOT_RECOGNIZED' ? 'red' : 'amber'} />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-fg">{item.merchant_name} · {formatBRL(item.amount)}</p>
          <p className="text-xs text-muted">{disputeReasons[item.reason]}{item.holder_name ? ` · ${item.holder_name}` : ''} · compra em {dateTimeBR(item.purchased_at)}</p>
          {item.description && <p className="mt-1 text-xs text-fg-2">“{item.description}”</p>}
        </div>
      </div>
      {rejecting ? (
        <form className="flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); decide.mutate(false) }}>
          <div className="flex-1"><Field label="Motivo da recusa" value={reason} onChange={(e) => setReason(e.target.value)} /></div>
          <Button type="submit" variant="danger" size="sm" disabled={!reason.trim()} loading={decide.isPending}>Recusar</Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => setRejecting(false)}>Voltar</Button>
        </form>
      ) : (
        <div className="flex flex-wrap gap-2">
          <Button size="sm" icon={Check} loading={decide.isPending} onClick={() => decide.mutate(true)}>Aceitar e estornar</Button>
          <Button size="sm" variant="secondary" icon={X} onClick={() => setRejecting(true)}>Recusar</Button>
          {item.customer_id && <Button size="sm" variant="ghost" onClick={onOpen}>Ver cliente</Button>}
        </div>
      )}
      <Alert>{error}</Alert>
    </li>
  )
}

function DisputesPanel({ onOpenCustomer }: { onOpenCustomer: (customerId: string) => void }) {
  const disputes = useQuery({
    queryKey: ['backoffice', 'disputes'],
    queryFn: async () => (await api<{ disputes: (Dispute & { customer_id?: string; holder_name?: string })[] }>('GET', '/v1/backoffice/disputes')).disputes,
    refetchInterval: 15_000,
  })
  const list = disputes.data ?? []
  return (
    <Panel title={`Contestações · ${list.length}`} icon={CreditCard}>
      {disputes.isLoading ? <Loading /> : !list.length ? <Empty icon={CreditCard}>Nenhuma compra contestada esperando análise.</Empty> : (
        <ul className="-mx-2 space-y-1">{list.map((d) => <DisputeItem key={d.id} item={d} onOpen={() => d.customer_id && onOpenCustomer(d.customer_id)} />)}</ul>
      )}
    </Panel>
  )
}

/** The back office needs a second factor; a password-only analyst enrolls here first. */
function MFAGate() {
  const [enabled, setEnabled] = useState(false)
  return (
    <main className="mx-auto max-w-lg space-y-5 px-4 py-12">
      <div>
        <h1 className="text-[34px] font-bold tracking-[-0.02em] text-fg">Proteja seu acesso</h1>
        <p className="text-sm text-muted">O back office exige verificação em duas etapas. Configure o autenticador e entre de novo.</p>
      </div>
      <TOTP onEnabled={() => setEnabled(true)} />
      {enabled && <Button icon={ShieldCheck} onClick={() => logout()}>Entrar de novo</Button>}
    </main>
  )
}

export function BackofficePage() {
  const me = useMe()
  const [selected, setSelected] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const mfa = me.data?.mfa !== false
  const queue = useQuery({
    queryKey: ['backoffice', 'reviews'],
    queryFn: async () => (await api<{ reviews: Review[] }>('GET', '/v1/backoffice/kyc/reviews')).reviews,
    refetchInterval: 15_000,
    enabled: mfa,
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
      {!mfa ? <MFAGate /> : <main ref={scope} className="mx-auto grid max-w-7xl gap-6 px-4 py-8 sm:px-8 lg:grid-cols-[380px_1fr]">
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
          <ApprovalsPanel me={me.data?.customer_id} onOpenCustomer={setSelected} />
          <DisputesPanel onOpenCustomer={setSelected} />
        </div>
        <div>
          {selected ? <CustomerPanel key={selected} id={selected} onDecided={() => setSelected(selected)} /> : (
            <Panel><Empty icon={UserRound}>Escolha um cadastro na fila ou busque um cliente.</Empty></Panel>
          )}
        </div>
      </main>}
    </div>
  )
}
