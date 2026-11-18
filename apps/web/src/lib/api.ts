// Client for the public REST API (api-gateway). Access tokens live in
// memory; the refresh token in sessionStorage (cleared when the tab
// closes). A 401 refreshes the pair once and retries.

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

interface Tokens {
  access_token: string
  refresh_token: string
  expires_at: string
}

const REFRESH_KEY = 'db.refresh'
let access: string | null = null
let refreshing: Promise<boolean> | null = null
const listeners = new Set<(signedIn: boolean) => void>()

function readRefresh(): string | null {
  try {
    return sessionStorage.getItem(REFRESH_KEY)
  } catch {
    return null
  }
}

function setTokens(t: Tokens | null) {
  access = t?.access_token ?? null
  try {
    if (t) sessionStorage.setItem(REFRESH_KEY, t.refresh_token)
    else sessionStorage.removeItem(REFRESH_KEY)
  } catch {
    /* private mode: the session lasts while the page is open */
  }
  listeners.forEach((l) => l(t !== null))
}

export function onSessionChange(l: (signedIn: boolean) => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export const hasSession = () => access !== null || readRefresh() !== null

export const accessToken = () => access

export function newIdempotencyKey(): string {
  return crypto.randomUUID()
}

async function refresh(): Promise<boolean> {
  const token = readRefresh()
  if (!token) return false
  refreshing ??= (async () => {
    try {
      const res = await fetch('/v1/auth/refresh', { method: 'POST', body: JSON.stringify({ refresh_token: token }) })
      if (!res.ok) {
        setTokens(null)
        return false
      }
      setTokens(await res.json())
      return true
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

/** Makes sure there is a valid access token (refreshing if needed). */
export async function ensureAccess(): Promise<string | null> {
  if (access) return access
  return (await refresh()) ? access : null
}

export interface RequestOptions {
  body?: unknown
  idempotent?: boolean | string
  stepUp?: string
  signal?: AbortSignal
}

export async function api<T>(method: string, path: string, opts: RequestOptions = {}, retried = false): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (!access && !retried && readRefresh()) await refresh()
  if (access) headers.Authorization = `Bearer ${access}`
  if (opts.idempotent) headers['Idempotency-Key'] = typeof opts.idempotent === 'string' ? opts.idempotent : newIdempotencyKey()
  if (opts.stepUp) headers['X-Step-Up-Token'] = opts.stepUp
  const res = await fetch(path, {
    method,
    headers,
    body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
    signal: opts.signal,
  })
  if (res.status === 401 && !retried && !path.startsWith('/v1/auth/') && (await refresh())) {
    return api<T>(method, path, opts, true)
  }
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const e = data?.error ?? {}
    throw new ApiError(res.status, e.code ?? `HTTP_${res.status}`, e.message ?? res.statusText)
  }
  return data as T
}

export async function login(email: string, password: string, totpCode?: string) {
  setTokens(await api<Tokens>('POST', '/v1/auth/login', { body: { email, password, totp_code: totpCode || undefined } }))
}

export async function register(email: string, password: string, fullName: string) {
  await api('POST', '/v1/auth/register', { body: { email, password, full_name: fullName } })
}

export async function logout() {
  const token = readRefresh()
  setTokens(null)
  if (token) await fetch('/v1/auth/logout', { method: 'POST', body: JSON.stringify({ refresh_token: token }) }).catch(() => {})
}

/** Re-confirms the password (and TOTP) for a sensitive action. */
export async function stepUp(password: string, totpCode?: string): Promise<string> {
  const r = await api<{ step_up_token: string }>('POST', '/v1/auth/step-up', { body: { password, totp_code: totpCode || undefined } })
  return r.step_up_token
}
