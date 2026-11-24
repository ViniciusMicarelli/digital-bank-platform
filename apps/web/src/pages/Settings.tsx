import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import QRCode from 'qrcode'
import { useState } from 'react'
import { useAccount, useProfile } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import { DoorClosed, FileDown, KeyRound, LogOut, MonitorSmartphone, Palette, SlidersHorizontal, UserRound } from 'lucide-react'
import { ThemeSwitch } from '../components/ThemeSwitch'
import { Alert, Badge, Button, Field, Modal, PageTitle, Panel } from '../components/ui'
import { api, logout } from '../lib/api'
import { closureBlockerText, errorMessage, formatBRL, lastSeen, type ClosureCheck, type Session } from '@bank/core'
import { Link } from 'react-router'
import { useStepUp } from '../components/StepUp'

/** LGPD: everything the bank keeps about the customer, as a file. */
function DataPanel() {
  const { confirm, modal } = useStepUp()
  const [error, setError] = useState('')
  const download = useMutation({
    mutationFn: async () => api<unknown>('POST', '/v1/me/data-export', { stepUp: await confirm() }),
    onSuccess: (data) => {
      const link = document.createElement('a')
      link.href = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }))
      link.download = `meus-dados-${new Date().toISOString().slice(0, 10)}.json`
      link.click()
      URL.revokeObjectURL(link.href)
      setError('')
    },
    onError: (e) => { if (!(e instanceof Error && e.message === 'cancelled')) setError(errorMessage(e)) },
  })
  return (
    <Panel title="Seus dados" icon={FileDown}>
      <p className="mb-4 text-sm text-fg-2">
        Baixe tudo o que guardamos sobre você: cadastro, contas, chaves Pix, cartões (só os 4 últimos dígitos), caixinhas, extrato e acessos.
        Registros de conta e de transações ficam guardados pelo prazo exigido por lei, mesmo depois de encerrar a conta.
      </p>
      <Button variant="secondary" icon={FileDown} loading={download.isPending} onClick={() => download.mutate()}>Baixar meus dados</Button>
      {error && <div className="mt-3"><Alert>{error}</Alert></div>}
      {modal}
    </Panel>
  )
}

/** Closing the account: what is in the way, what will be cancelled, then
 * the password again. */
function ClosePanel({ accountId }: { accountId: string }) {
  const qc = useQueryClient()
  const { confirm, modal } = useStepUp()
  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const check = useQuery({
    queryKey: ['closure', accountId],
    enabled: open,
    queryFn: () => api<ClosureCheck>('GET', `/v1/accounts/${accountId}/closure`),
  })
  const close = useMutation({
    mutationFn: async () => api('POST', `/v1/accounts/${accountId}/close`, { body: { reason }, stepUp: await confirm() }),
    onSuccess: () => { setOpen(false); qc.invalidateQueries() },
    onError: (e) => { setError(errorMessage(e)); check.refetch() },
  })
  const c = check.data
  const wc = c?.will_cancel
  const cancels = wc ? [
    wc.pix_schedules && `${wc.pix_schedules} Pix agendado${wc.pix_schedules > 1 ? 's' : ''}`,
    wc.boleto_schedules && `${wc.boleto_schedules} boleto${wc.boleto_schedules > 1 ? 's' : ''} agendado${wc.boleto_schedules > 1 ? 's' : ''}`,
    wc.pix_automatico && `${wc.pix_automatico} autorizaç${wc.pix_automatico > 1 ? 'ões' : 'ão'} de Pix Automático`,
    wc.empty_savings_boxes && `${wc.empty_savings_boxes} caixinha${wc.empty_savings_boxes > 1 ? 's' : ''} vazia${wc.empty_savings_boxes > 1 ? 's' : ''}`,
  ].filter(Boolean) as string[] : []
  return (
    <Panel title="Encerrar conta" icon={DoorClosed}>
      <p className="mb-4 text-sm text-fg-2">Você pode encerrar sua conta quando quiser. O extrato e os comprovantes continuam disponíveis.</p>
      <Button variant="secondary" onClick={() => { setError(''); setOpen(true) }}>Encerrar minha conta</Button>
      <Modal open={open} onClose={() => setOpen(false)} title="Encerrar conta">
        {check.isLoading || !c ? <p className="text-sm text-muted">Verificando sua conta…</p> : (
          <div className="space-y-4">
            {c.blockers.length > 0 ? (
              <>
                <p className="text-sm text-fg-2">Antes de encerrar, resolva:</p>
                <ul className="space-y-2">
                  {c.blockers.map((b) => {
                    const t = closureBlockerText(b)
                    return (
                      <li key={b.code} className="rounded-2xl bg-surface-2 px-4 py-3 text-sm">
                        <p className="font-medium text-fg">{t.title}</p>
                        <p className="text-xs text-muted">{t.action} <Link className="font-medium text-brand-strong" to={t.route} onClick={() => setOpen(false)}>Resolver</Link></p>
                      </li>
                    )
                  })}
                </ul>
              </>
            ) : (
              <>
                <ul className="list-disc space-y-1 pl-5 text-sm text-fg-2">
                  <li>Seus cartões serão cancelados e suas chaves Pix, removidas.</li>
                  {cancels.length > 0 && <li>Também cancelamos: {cancels.join(', ')}.</li>}
                  <li>Não dá para desfazer; você pode abrir uma conta nova depois.</li>
                </ul>
                <Field label="Por que você está saindo? (opcional)" maxLength={200} value={reason} onChange={(e) => setReason(e.target.value)} />
                <Alert>{error}</Alert>
                <Button variant="danger" className="w-full" loading={close.isPending} onClick={() => { setError(''); close.mutate() }}>Encerrar definitivamente</Button>
              </>
            )}
          </div>
        )}
      </Modal>
      {modal}
    </Panel>
  )
}

/** Where the customer is signed in; ends any other session, or all of them. */
function SessionsPanel() {
  const qc = useQueryClient()
  const [error, setError] = useState('')
  const sessions = useQuery({ queryKey: ['sessions'], queryFn: async () => (await api<{ sessions: Session[] }>('GET', '/v1/me/sessions')).sessions })
  const done = { onSuccess: () => { setError(''); qc.invalidateQueries({ queryKey: ['sessions'] }) }, onError: (e: unknown) => setError(errorMessage(e)) }
  const end = useMutation({ mutationFn: (id: string) => api('DELETE', `/v1/me/sessions/${id}`), ...done })
  const endOthers = useMutation({ mutationFn: () => api('POST', '/v1/me/sessions/end-others'), ...done })
  const list = sessions.data ?? []
  const others = list.filter((s) => !s.current)
  return (
    <Panel title="Dispositivos conectados" icon={MonitorSmartphone}>
      <p className="mb-3 text-sm text-fg-2">Onde sua conta está aberta. Não reconhece algum? Encerre o acesso e troque sua senha.</p>
      <ul className="divide-y divide-line border-y border-line">
        {list.map((s) => (
          <li key={s.id} className="flex items-center gap-3 py-3 text-sm">
            <div className="min-w-0 flex-1">
              <p className="flex items-center gap-2 font-medium text-fg">{s.device}{s.current && <Badge tone="green">Este dispositivo</Badge>}</p>
              <p className="text-xs text-muted">{s.current ? 'Em uso agora' : `Último acesso ${lastSeen(s.last_used_at)}`} · IP {s.ip || '—'}</p>
            </div>
            {s.current
              ? <Button variant="ghost" size="sm" icon={LogOut} onClick={() => logout()}>Sair</Button>
              : <Button variant="ghost" size="sm" loading={end.isPending && end.variables === s.id} onClick={() => end.mutate(s.id)}>Encerrar</Button>}
          </li>
        ))}
      </ul>
      {error && <div className="mt-3"><Alert>{error}</Alert></div>}
      {others.length > 0 && (
        <Button className="mt-4" variant="danger" loading={endOthers.isPending} onClick={() => endOthers.mutate()}>
          Encerrar {others.length === 1 ? 'o outro acesso' : `os outros ${others.length} acessos`}
        </Button>
      )}
      {list.length === 0 && !sessions.isLoading && (
        <Button variant="danger" icon={LogOut} onClick={() => logout()}>Sair</Button>
      )}
    </Panel>
  )
}

export function TOTP({ onEnabled }: { onEnabled?: () => void } = {}) {
  const [enroll, setEnroll] = useState<{ secret: string; image: string } | null>(null)
  const [code, setCode] = useState('')
  const [msg, setMsg] = useState<{ tone: 'green' | 'red'; text: string } | null>(null)
  const start = useMutation({
    mutationFn: () => api<{ secret: string; otpauth_uri: string }>('POST', '/v1/auth/mfa/totp'),
    onSuccess: async (r) => setEnroll({ secret: r.secret, image: await QRCode.toDataURL(r.otpauth_uri, { margin: 1, width: 200 }) }),
    onError: (e) => setMsg({ tone: 'red', text: errorMessage(e) }),
  })
  const confirm = useMutation({
    mutationFn: () => api('POST', '/v1/auth/mfa/totp/confirm', { body: { code } }),
    onSuccess: () => { setEnroll(null); setMsg({ tone: 'green', text: 'Autenticador ativado. Ele será pedido no login e nas ações sensíveis.' }); onEnabled?.() },
    onError: (e) => setMsg({ tone: 'red', text: errorMessage(e) }),
  })
  return (
    <Panel title="Verificação em duas etapas" icon={KeyRound}>
      {enroll ? (
        <form onSubmit={(e) => { e.preventDefault(); confirm.mutate() }} className="space-y-4">
          <p className="text-sm text-fg-2">Escaneie com Google Authenticator, 1Password ou similar e digite o código.</p>
          <div className="w-fit rounded-2xl bg-white p-3 ring-1 ring-line"><img src={enroll.image} alt="QR Code do autenticador" className="size-44" /></div>
          <p className="font-mono text-xs break-all text-muted">{enroll.secret}</p>
          <Field label="Código de 6 dígitos" inputMode="numeric" maxLength={6} required value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} />
          <Button type="submit" loading={confirm.isPending}>Ativar</Button>
        </form>
      ) : (
        <>
          <p className="mb-4 text-sm text-fg-2">Além da senha, peça um código do seu celular para entrar.</p>
          <Button variant="secondary" loading={start.isPending} onClick={() => start.mutate()}>Configurar autenticador</Button>
        </>
      )}
      {msg && <div className="mt-4"><Alert tone={msg.tone}>{msg.text}</Alert></div>}
    </Panel>
  )
}

interface Limits { limits: { per_transaction: string; daily: string; nightly: string }; used: { daily: string; nightly: string } }

function LimitsPanel({ accountId }: { accountId: string }) {
  const qc = useQueryClient()
  const url = `/v1/accounts/${accountId}/limits`
  const limits = useQuery({ queryKey: ['limits', accountId], queryFn: () => api<Limits>('GET', url) })
  const [edit, setEdit] = useState<{ per?: string | null; daily?: string | null; nightly?: string | null } | null>(null)
  const [error, setError] = useState('')
  const save = useMutation({
    mutationFn: () => api('PUT', url, { body: { per_transaction: edit!.per ?? limits.data!.limits.per_transaction,
      daily: edit!.daily ?? limits.data!.limits.daily, nightly: edit!.nightly ?? limits.data!.limits.nightly } }),
    onSuccess: () => { setEdit(null); qc.invalidateQueries({ queryKey: ['limits'] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  const l = limits.data
  return (
    <Panel icon={SlidersHorizontal} title="Limites de transação" action={!edit && <Button variant="ghost" onClick={() => setEdit({})}>Alterar</Button>}>
      {l && !edit && (
        <dl className="grid grid-cols-3 gap-3 text-sm">
          <div><dt className="text-muted">Por transação</dt><dd className="font-semibold">{formatBRL(l.limits.per_transaction)}</dd></div>
          <div><dt className="text-muted">Por dia</dt><dd className="font-semibold">{formatBRL(l.limits.daily)}</dd><dd className="text-xs text-subtle">usado {formatBRL(l.used.daily)}</dd></div>
          <div><dt className="text-muted">À noite (20h–6h)</dt><dd className="font-semibold">{formatBRL(l.limits.nightly)}</dd><dd className="text-xs text-subtle">usado {formatBRL(l.used.nightly)}</dd></div>
        </dl>
      )}
      {edit && (
        <form onSubmit={(e) => { e.preventDefault(); save.mutate() }} className="space-y-3">
          <MoneyField label="Por transação" optional hint={`Atual: ${formatBRL(l?.limits.per_transaction)}`} onValue={(per) => setEdit({ ...edit, per })} />
          <MoneyField label="Por dia" optional hint={`Atual: ${formatBRL(l?.limits.daily)}`} onValue={(daily) => setEdit({ ...edit, daily })} />
          <MoneyField label="À noite" optional hint={`Atual: ${formatBRL(l?.limits.nightly)}`} onValue={(nightly) => setEdit({ ...edit, nightly })} />
          <Alert>{error}</Alert>
          <div className="flex gap-2"><Button type="submit" loading={save.isPending}>Salvar</Button><Button type="button" variant="ghost" onClick={() => setEdit(null)}>Cancelar</Button></div>
        </form>
      )}
    </Panel>
  )
}

export function SettingsPage() {
  const profile = useProfile().data
  const account = useAccount()
  return (
    <div className="space-y-6">
      <PageTitle>Ajustes</PageTitle>
      <div className="grid gap-6 lg:grid-cols-2">
        <Panel icon={UserRound} title={<span className="flex items-center gap-2">Meus dados {profile?.kyc_status === 'APPROVED' && <Badge tone="green">Verificado</Badge>}</span>}>
          {profile && (
            <dl className="space-y-2 text-sm">
              <div className="flex justify-between gap-4"><dt className="text-muted">Nome</dt><dd className="text-right font-medium">{profile.full_name}</dd></div>
              <div className="flex justify-between gap-4"><dt className="text-muted">CPF</dt><dd className="font-medium">{profile.cpf}</dd></div>
              <div className="flex justify-between gap-4"><dt className="text-muted">Celular</dt><dd className="font-medium">{profile.phone || '—'}</dd></div>
              <div className="flex justify-between gap-4"><dt className="text-muted">Endereço</dt>
                <dd className="text-right font-medium">{profile.address.street}, {profile.address.number} · {profile.address.city}/{profile.address.state}</dd></div>
              {account && <div className="flex justify-between gap-4"><dt className="text-muted">Conta</dt><dd className="font-medium">Ag. {account.branch} · {account.number}</dd></div>}
            </dl>
          )}
        </Panel>
        <TOTP />
        {account && <LimitsPanel accountId={account.id} />}
        <Panel title="Aparência" icon={Palette}>
          <p className="mb-4 text-sm text-fg-2">Siga o sistema ou escolha o tema claro ou escuro.</p>
          <ThemeSwitch />
        </Panel>
        <SessionsPanel />
        <DataPanel />
        {account?.status === 'ACTIVE' && <ClosePanel accountId={account.id} />}
      </div>
    </div>
  )
}
