import { ArrowRight, Bell, ShieldCheck, Zap } from 'lucide-react'
import { useRef, useState, type FormEvent, type ReactNode } from 'react'
import { Link, useNavigate } from 'react-router'
import { Aurora } from '../components/Aurora'
import { BankCard } from '../components/BankCard'
import { Brand } from '../components/Brand'
import { ThemeSwitch } from '../components/ThemeSwitch'
import { Alert, Button, Field } from '../components/ui'
import { ApiError, login, register } from '../lib/api'
import { errorMessage } from '@bank/core'
import { gsap, reducedMotion, useGSAP } from '../lib/motion'

function Showcase() {
  const root = useRef<HTMLDivElement>(null)
  useGSAP(() => {
    if (reducedMotion()) return
    const tl = gsap.timeline({ defaults: { ease: 'power3.out' } })
    tl.from('[data-hero-text]', { y: 30, autoAlpha: 0, duration: 0.8, stagger: 0.12 })
      .from('[data-hero-card]', { y: 60, rotate: -8, autoAlpha: 0, duration: 1.1, stagger: 0.15 }, 0.2)
      .from('[data-hero-feature]', { y: 16, autoAlpha: 0, duration: 0.6, stagger: 0.08 }, 0.7)
    gsap.to('[data-hero-card="back"]', { y: -10, rotate: -10, duration: 4, yoyo: true, repeat: -1, ease: 'sine.inOut' })
    gsap.to('[data-hero-card="front"]', { y: 8, rotate: 4, duration: 5, yoyo: true, repeat: -1, ease: 'sine.inOut' })
  }, { scope: root })
  return (
    <div ref={root} className="hero-surface relative hidden flex-col justify-between gap-10 overflow-hidden p-12 text-white lg:flex">
      <Brand light />
      <div>
        <h2 data-hero-text className="max-w-md text-[44px] leading-[1.05] font-bold tracking-tight">Seu dinheiro, <span className="text-[#2ed3a0]">em tempo real.</span></h2>
        <p data-hero-text className="mt-4 max-w-md text-[15px] text-white/70">Pix, cartões de débito e crédito, boletos e notificações na hora, sobre um ledger de partidas dobradas.</p>
        <div className="relative mt-10 h-56 max-w-md">
          <div data-hero-card="back" className="absolute top-0 left-16 w-64 -rotate-6 opacity-90">
            <BankCard holder="ANA SOUZA" last4="4821" expiry="11/31" skin="credit" label="Crédito" interactive={false} />
          </div>
          <div data-hero-card="front" className="absolute top-12 left-0 w-72 rotate-3">
            <BankCard holder="ANA SOUZA" last4="0938" expiry="11/31" interactive={false} />
          </div>
        </div>
      </div>
      <ul className="grid grid-cols-3 gap-4 text-sm">
        {([[Zap, 'Pix em segundos'], [ShieldCheck, 'Confirmação de senha em ações sensíveis'], [Bell, 'Avisos na hora']] as const).map(([Icon, text]) => (
          <li key={text} data-hero-feature className="flex items-start gap-2 text-white/75"><Icon className="mt-0.5 size-4 shrink-0 text-[#2ed3a0]" />{text}</li>
        ))}
      </ul>
    </div>
  )
}

function AuthShell({ title, subtitle, children }: { title: string; subtitle: string; children: ReactNode }) {
  const form = useRef<HTMLDivElement>(null)
  useGSAP(() => {
    if (!reducedMotion()) gsap.from('[data-reveal]', { y: 16, autoAlpha: 0, duration: 0.6, stagger: 0.06, ease: 'power3.out', clearProps: 'all' })
  }, { scope: form })
  return (
    <div className="grid min-h-full lg:grid-cols-[1.1fr_1fr]">
      <Aurora />
      <Showcase />
      <div className="relative flex items-center justify-center px-5 py-12">
        <div className="absolute top-5 right-5"><ThemeSwitch compact /></div>
        <div ref={form} className="glass-strong w-full max-w-md rounded-[34px] p-7 sm:p-9">
          <div data-reveal className="mb-10 lg:hidden"><Brand /></div>
          <h1 data-reveal className="text-[30px] font-bold tracking-[-0.02em] text-fg">{title}</h1>
          <p data-reveal className="mt-2 mb-8 text-sm text-muted">{subtitle}</p>
          <div data-reveal>{children}</div>
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
    <AuthShell title="Bem-vindo de volta" subtitle="Entre para ver seu saldo e movimentar sua conta.">
      <form onSubmit={submit} className="space-y-4">
        <Field label="E-mail" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        <Field label="Senha" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        {needsTotp && (
          <Field label="Código do autenticador" inputMode="numeric" autoComplete="one-time-code" maxLength={6} autoFocus
            value={totp} onChange={(e) => setTotp(e.target.value.replace(/\D/g, ''))} />
        )}
        <Alert>{error}</Alert>
        <Button type="submit" loading={busy} size="lg" className="w-full">Entrar <ArrowRight className="size-4" /></Button>
      </form>
      <p className="mt-8 text-center text-sm text-muted">
        Ainda não tem conta? <Link to="/criar-conta" className="font-semibold text-brand-strong hover:underline">Criar conta</Link>
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

  const strength = Math.min(4, Math.floor(form.password.length / 5))
  return (
    <AuthShell title="Abra sua conta" subtitle="Sem tarifa de manutenção. Leva dois minutos.">
      <form onSubmit={submit} className="space-y-4">
        <Field label="Nome completo" autoComplete="name" required value={form.name} onChange={set('name')} />
        <Field label="E-mail" type="email" autoComplete="email" required value={form.email} onChange={set('email')} />
        <div>
          <Field label="Senha" type="password" autoComplete="new-password" required minLength={10} maxLength={128}
            hint="De 10 a 128 caracteres. Uma frase é ótima." value={form.password} onChange={set('password')} />
          <div aria-hidden className="mt-2 grid grid-cols-4 gap-1">
            {[0, 1, 2, 3].map((i) => (
              <span key={i} className={`h-1 rounded-full transition-colors duration-300 ${i < strength ? (strength < 2 ? 'bg-warning' : 'bg-brand') : 'bg-surface-3'}`} />
            ))}
          </div>
        </div>
        <Alert>{error}</Alert>
        <Button type="submit" loading={busy} size="lg" className="w-full">Criar conta</Button>
      </form>
      <p className="mt-8 text-center text-sm text-muted">
        Já tem conta? <Link to="/entrar" className="font-semibold text-brand-strong hover:underline">Entrar</Link>
      </p>
    </AuthShell>
  )
}
