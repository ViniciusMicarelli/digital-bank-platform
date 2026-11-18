import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router'
import { Brand } from '../components/Layout'
import { Alert, Button, Field } from '../components/ui'
import { ApiError, login, register } from '../lib/api'
import { errorMessage } from '../lib/errors'

function AuthShell({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="grid min-h-full lg:grid-cols-2">
      <div className="hidden flex-col justify-between bg-brand-900 p-12 text-white lg:flex">
        <span className="text-white"><Brand /></span>
        <div>
          <p className="text-4xl leading-tight font-bold">Seu dinheiro, em tempo real.</p>
          <p className="mt-4 max-w-md text-brand-100">Pix, cartões de débito e crédito, boletos e notificações na hora — sobre um ledger de partidas dobradas.</p>
        </div>
        <p className="text-sm text-brand-100/70">Projeto de portfólio · ambiente de testes</p>
      </div>
      <div className="flex items-center justify-center px-4 py-12">
        <div className="w-full max-w-sm">
          <div className="mb-8 lg:hidden"><Brand /></div>
          <h1 className="mb-6 text-2xl font-bold tracking-tight">{title}</h1>
          {children}
        </div>
      </div>
    </div>
  )
}

export function LoginPage() {
  const nav = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [totp, setTotp] = useState('')
  const [needsTotp, setNeedsTotp] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await login(email, password, totp)
      nav('/')
    } catch (err) {
      if (err instanceof ApiError && err.code === 'MFA_REQUIRED') setNeedsTotp(true)
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell title="Entrar">
      <form onSubmit={submit} className="space-y-4">
        <Field label="E-mail" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        <Field label="Senha" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        {needsTotp && (
          <Field label="Código do autenticador" inputMode="numeric" autoComplete="one-time-code" maxLength={6} autoFocus
            value={totp} onChange={(e) => setTotp(e.target.value.replace(/\D/g, ''))} />
        )}
        <Alert>{error}</Alert>
        <Button type="submit" loading={busy} className="w-full">Entrar</Button>
      </form>
      <p className="mt-6 text-center text-sm text-slate-600">
        Ainda não tem conta? <Link to="/criar-conta" className="font-semibold text-brand-700">Criar conta</Link>
      </p>
    </AuthShell>
  )
}

export function RegisterPage() {
  const nav = useNavigate()
  const [form, setForm] = useState({ name: '', email: '', password: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value })

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await register(form.email, form.password, form.name)
      await login(form.email, form.password)
      nav('/')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell title="Criar conta">
      <form onSubmit={submit} className="space-y-4">
        <Field label="Nome completo" autoComplete="name" required value={form.name} onChange={set('name')} />
        <Field label="E-mail" type="email" autoComplete="email" required value={form.email} onChange={set('email')} />
        <Field label="Senha" type="password" autoComplete="new-password" required minLength={10} maxLength={128}
          hint="De 10 a 128 caracteres. Uma frase é ótima." value={form.password} onChange={set('password')} />
        <Alert>{error}</Alert>
        <Button type="submit" loading={busy} className="w-full">Criar conta</Button>
      </form>
      <p className="mt-6 text-center text-sm text-slate-600">
        Já tem conta? <Link to="/entrar" className="font-semibold text-brand-700">Entrar</Link>
      </p>
    </AuthShell>
  )
}
