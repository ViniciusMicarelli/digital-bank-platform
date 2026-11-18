import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import QRCode from 'qrcode'
import { useState } from 'react'
import { useAccount, useProfile } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import { Alert, Badge, Button, Field, PageTitle, Panel } from '../components/ui'
import { api, logout } from '../lib/api'
import { errorMessage } from '@bank/core'
import { formatBRL } from '@bank/core'

function TOTP() {
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
    onSuccess: () => { setEnroll(null); setMsg({ tone: 'green', text: 'Autenticador ativado. Ele será pedido no login e nas ações sensíveis.' }) },
    onError: (e) => setMsg({ tone: 'red', text: errorMessage(e) }),
  })
  return (
    <Panel title="Verificação em duas etapas">
      {enroll ? (
        <form onSubmit={(e) => { e.preventDefault(); confirm.mutate() }} className="space-y-4">
          <p className="text-sm text-slate-600">Escaneie com Google Authenticator, 1Password ou similar e digite o código.</p>
          <img src={enroll.image} alt="QR Code do autenticador" className="rounded-xl ring-1 ring-slate-200" />
          <p className="font-mono text-xs break-all text-slate-500">{enroll.secret}</p>
          <Field label="Código de 6 dígitos" inputMode="numeric" maxLength={6} required value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))} />
          <Button type="submit" loading={confirm.isPending}>Ativar</Button>
        </form>
      ) : (
        <>
          <p className="mb-4 text-sm text-slate-600">Além da senha, peça um código do seu celular para entrar.</p>
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
    <Panel title="Limites de transação" action={!edit && <Button variant="ghost" onClick={() => setEdit({})}>Alterar</Button>}>
      {l && !edit && (
        <dl className="grid grid-cols-3 gap-3 text-sm">
          <div><dt className="text-slate-500">Por transação</dt><dd className="font-semibold">{formatBRL(l.limits.per_transaction)}</dd></div>
          <div><dt className="text-slate-500">Por dia</dt><dd className="font-semibold">{formatBRL(l.limits.daily)}</dd><dd className="text-xs text-slate-400">usado {formatBRL(l.used.daily)}</dd></div>
          <div><dt className="text-slate-500">À noite (20h–6h)</dt><dd className="font-semibold">{formatBRL(l.limits.nightly)}</dd><dd className="text-xs text-slate-400">usado {formatBRL(l.used.nightly)}</dd></div>
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
        <Panel title={<span className="flex items-center gap-2">Meus dados {profile?.kyc_status === 'APPROVED' && <Badge tone="green">Verificado</Badge>}</span>}>
          {profile && (
            <dl className="space-y-2 text-sm">
              <div className="flex justify-between gap-4"><dt className="text-slate-500">Nome</dt><dd className="text-right font-medium">{profile.full_name}</dd></div>
              <div className="flex justify-between gap-4"><dt className="text-slate-500">CPF</dt><dd className="font-medium">{profile.cpf}</dd></div>
              <div className="flex justify-between gap-4"><dt className="text-slate-500">Celular</dt><dd className="font-medium">{profile.phone || '—'}</dd></div>
              <div className="flex justify-between gap-4"><dt className="text-slate-500">Endereço</dt>
                <dd className="text-right font-medium">{profile.address.street}, {profile.address.number} · {profile.address.city}/{profile.address.state}</dd></div>
              {account && <div className="flex justify-between gap-4"><dt className="text-slate-500">Conta</dt><dd className="font-medium">Ag. {account.branch} · {account.number}</dd></div>}
            </dl>
          )}
        </Panel>
        <TOTP />
        {account && <LimitsPanel accountId={account.id} />}
        <Panel title="Sessão">
          <p className="mb-4 text-sm text-slate-600">Encerra a sessão neste navegador e invalida o token de renovação.</p>
          <Button variant="danger" onClick={() => logout()}>Sair</Button>
        </Panel>
      </div>
    </div>
  )
}
