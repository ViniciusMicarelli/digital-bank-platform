import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { keys, useAccounts, useProfile } from '../app/queries'
import { Brand } from '../components/Layout'
import { Alert, Badge, Button, Field, Loading, Panel, Sandbox, Select } from '../components/ui'
import { api, logout } from '../lib/api'
import { errorMessage } from '../lib/errors'
import { maskCPF } from '../lib/format'
import type { Profile } from '../lib/types'

const UFS = 'AC AL AP AM BA CE DF ES GO MA MT MS MG PA PB PR PE PI RJ RN RS RO RR SC SP SE TO'.split(' ')

const maskCEP = (v: string) => v.replace(/\D/g, '').slice(0, 8).replace(/(\d{5})(\d)/, '$1-$2')

const reasons: Record<string, string> = {
  UNDERAGE: 'É preciso ter 18 anos ou mais.',
  CPF_IRREGULAR: 'O CPF está com situação irregular na Receita Federal.',
  PEP: 'Pessoa politicamente exposta: análise manual.',
}

function Steps({ step }: { step: number }) {
  const names = ['Seus dados', 'Verificação', 'Conta']
  return (
    <ol className="mb-8 flex gap-2">
      {names.map((n, i) => (
        <li key={n} className="flex-1">
          <div className={`h-1.5 rounded-full ${i <= step ? 'bg-brand-600' : 'bg-slate-200'}`} />
          <p className={`mt-2 text-xs font-medium ${i <= step ? 'text-brand-700' : 'text-slate-400'}`}>{i + 1}. {n}</p>
        </li>
      ))}
    </ol>
  )
}

function ProfileForm({ profile, onSaved }: { profile: Profile | null; onSaved: () => void }) {
  const locked = profile !== null && profile.kyc_status !== 'NOT_STARTED' && profile.kyc_status !== 'REJECTED'
  const [f, setF] = useState({
    full_name: profile?.full_name ?? '', cpf: '', birth_date: profile?.birth_date ?? '', phone: profile?.phone ?? '',
    postal_code: profile?.address.postal_code ? maskCEP(profile.address.postal_code) : '', street: profile?.address.street ?? '',
    number: profile?.address.number ?? '', complement: profile?.address.complement ?? '', district: profile?.address.district ?? '',
    city: profile?.address.city ?? '', state: profile?.address.state ?? 'SP',
  })
  const [error, setError] = useState('')
  const save = useMutation({
    mutationFn: () => api('PUT', '/v1/me/profile', {
      body: { full_name: f.full_name, cpf: f.cpf, birth_date: f.birth_date, phone: f.phone,
        address: { postal_code: f.postal_code, street: f.street, number: f.number, complement: f.complement, district: f.district, city: f.city, state: f.state } },
    }),
    onSuccess: onSaved,
    onError: (e) => setError(errorMessage(e)),
  })
  const set = (k: keyof typeof f, mask?: (v: string) => string) => (e: { target: { value: string } }) =>
    setF({ ...f, [k]: mask ? mask(e.target.value) : e.target.value })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    setError('')
    save.mutate()
  }
  return (
    <form onSubmit={submit} className="space-y-4">
      <Field label="Nome completo" required disabled={locked} value={f.full_name} onChange={set('full_name')} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="CPF" required inputMode="numeric" placeholder={profile?.cpf ?? '000.000.000-00'} value={f.cpf} onChange={set('cpf', maskCPF)}
          hint={profile ? 'Digite o CPF de novo para confirmar.' : undefined} />
        <Field label="Data de nascimento" type="date" required value={f.birth_date} onChange={set('birth_date')} />
      </div>
      <Field label="Celular" type="tel" placeholder="+55 11 91234-5678" value={f.phone} onChange={set('phone')} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="CEP" required inputMode="numeric" value={f.postal_code} onChange={set('postal_code', maskCEP)} />
        <div className="sm:col-span-2"><Field label="Rua" required value={f.street} onChange={set('street')} /></div>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Field label="Número" required value={f.number} onChange={set('number')} />
        <Field label="Complemento" value={f.complement} onChange={set('complement')} />
        <Field label="Bairro" value={f.district} onChange={set('district')} />
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <div className="sm:col-span-2"><Field label="Cidade" required value={f.city} onChange={set('city')} /></div>
        <Select label="UF" value={f.state} onChange={set('state')}>{UFS.map((u) => <option key={u}>{u}</option>)}</Select>
      </div>
      <Alert>{error}</Alert>
      <Button type="submit" loading={save.isPending} className="w-full">Salvar e continuar</Button>
    </form>
  )
}

export function OnboardingPage() {
  const qc = useQueryClient()
  const nav = useNavigate()
  const profile = useProfile()
  const accounts = useAccounts()
  const [editing, setEditing] = useState(false)
  const [error, setError] = useState('')
  const refresh = () => qc.invalidateQueries({ queryKey: keys.profile })

  const submitKYC = useMutation({ mutationFn: () => api('POST', '/v1/me/kyc'), onSuccess: refresh, onError: (e) => setError(errorMessage(e)) })
  const review = useMutation({
    mutationFn: (approve: boolean) => api('POST', '/v1/sandbox/kyc/review', { body: { approve, reason: approve ? '' : 'DOCUMENTS_NOT_CONFIRMED' } }),
    onSuccess: refresh,
  })
  const open = useMutation({
    mutationFn: () => api('POST', '/v1/accounts', { body: {}, idempotent: true }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: keys.accounts })
      nav('/')
    },
    onError: (e) => setError(errorMessage(e)),
  })

  if (profile.isLoading || accounts.isLoading) return <Loading />
  const p = profile.data ?? null
  const status = p?.kyc_status
  const step = !p || editing || status === 'REJECTED' ? 0 : status === 'APPROVED' ? 2 : 1

  return (
    <div className="min-h-full bg-slate-50">
      <header className="flex items-center justify-between px-6 py-4">
        <Brand />
        <button onClick={() => logout()} className="text-sm text-slate-500 hover:text-slate-800">Sair</button>
      </header>
      <main className="mx-auto max-w-xl px-4 py-6">
        <h1 className="text-2xl font-bold tracking-tight">Vamos abrir sua conta</h1>
        <p className="mt-1 mb-6 text-sm text-slate-500">Leva dois minutos. Seus dados são verificados com o bureau de identidade.</p>
        <Steps step={step} />

        {step === 0 && (
          <Panel title="Seus dados">
            {status === 'REJECTED' && (
              <div className="mb-4"><Alert tone="amber">Não conseguimos verificar sua identidade: {reasons[p!.kyc_reason] ?? p!.kyc_reason}. Corrija os dados e envie de novo.</Alert></div>
            )}
            <ProfileForm profile={p} onSaved={() => { setEditing(false); refresh() }} />
          </Panel>
        )}

        {step === 1 && p && (
          <Panel title="Verificação de identidade">
            <dl className="mb-5 grid grid-cols-2 gap-3 text-sm">
              <div><dt className="text-slate-500">Nome</dt><dd className="font-medium">{p.full_name}</dd></div>
              <div><dt className="text-slate-500">CPF</dt><dd className="font-medium">{p.cpf}</dd></div>
            </dl>
            {status === 'NOT_STARTED' && (
              <div className="flex flex-col gap-2 sm:flex-row">
                <Button loading={submitKYC.isPending} onClick={() => submitKYC.mutate()}>Enviar para verificação</Button>
                <Button variant="secondary" onClick={() => setEditing(true)}>Editar dados</Button>
              </div>
            )}
            {status === 'PENDING' && <Alert tone="brand">Verificando com o bureau… isso costuma levar segundos.</Alert>}
            {status === 'MANUAL_REVIEW' && (
              <div className="space-y-4">
                <Alert tone="amber">Seu cadastro está em análise manual ({reasons[p.kyc_reason] ?? p.kyc_reason}). Avisamos assim que terminar.</Alert>
                <Sandbox>
                  <p className="mb-3 text-sm text-slate-600">Faça o papel do analista:</p>
                  <div className="flex gap-2">
                    <Button loading={review.isPending} onClick={() => review.mutate(true)}>Aprovar</Button>
                    <Button variant="danger" loading={review.isPending} onClick={() => review.mutate(false)}>Recusar</Button>
                  </div>
                </Sandbox>
              </div>
            )}
            <div className="mt-4"><Alert>{error}</Alert></div>
          </Panel>
        )}

        {step === 2 && (
          <Panel title={<span className="flex items-center gap-2">Identidade verificada <Badge tone="green">Aprovado</Badge></span>}>
            <p className="mb-5 text-sm text-slate-600">Tudo certo, {p!.full_name.split(' ')[0]}. Sua conta corrente será aberta com o nome e o CPF verificados.</p>
            <Alert>{error}</Alert>
            <Button loading={open.isPending} onClick={() => open.mutate()} className="w-full">Abrir minha conta</Button>
          </Panel>
        )}
      </main>
    </div>
  )
}
