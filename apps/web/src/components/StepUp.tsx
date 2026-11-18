import { useRef, useState, type FormEvent } from 'react'
import { ApiError, stepUp } from '../lib/api'
import { errorMessage } from '@bank/core'
import { Alert, Button, Field, Modal } from './ui'

let cached: { token: string; until: number } | null = null

/**
 * Sensitive actions need a fresh password confirmation (step-up). confirm()
 * resolves with a step-up token, asking for the password when the cached one
 * (valid 5 minutes; reused for 4) is gone.
 */
export function useStepUp() {
  const [open, setOpen] = useState(false)
  const [password, setPassword] = useState('')
  const [totp, setTotp] = useState('')
  const [needsTotp, setNeedsTotp] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const pending = useRef<{ resolve: (t: string) => void; reject: (e: Error) => void } | null>(null)

  function confirm(): Promise<string> {
    if (cached && cached.until > Date.now()) return Promise.resolve(cached.token)
    setPassword('')
    setTotp('')
    setError('')
    setOpen(true)
    return new Promise((resolve, reject) => { pending.current = { resolve, reject } })
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      const token = await stepUp(password, totp)
      cached = { token, until: Date.now() + 4 * 60_000 }
      setOpen(false)
      pending.current?.resolve(token)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'MFA_REQUIRED') setNeedsTotp(true)
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  function close() {
    setOpen(false)
    pending.current?.reject(new Error('cancelled'))
  }

  const modal = (
    <Modal open={open} onClose={close} title="Confirme que é você">
      <form onSubmit={submit} className="space-y-4">
        <p className="text-sm text-fg-2">Por segurança, digite sua senha para continuar.</p>
        <Field label="Senha" type="password" autoComplete="current-password" required autoFocus value={password} onChange={(e) => setPassword(e.target.value)} />
        {needsTotp && <Field label="Código do autenticador" inputMode="numeric" maxLength={6} value={totp} onChange={(e) => setTotp(e.target.value.replace(/\D/g, ''))} />}
        <Alert>{error}</Alert>
        <Button type="submit" loading={busy} className="w-full">Confirmar</Button>
      </form>
    </Modal>
  )
  return { confirm, modal }
}
