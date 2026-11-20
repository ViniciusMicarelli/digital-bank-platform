// Client for the public REST API (api-gateway), for any platform. Access
// tokens live in memory; the refresh token is handed to save() (web:
// sessionStorage; mobile: the device keychain). A 401 refreshes the pair
// once (single flight) and retries.

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
}

export interface RequestOptions {
  body?: unknown
  /** true: a new Idempotency-Key; a string: that key (reuse it on retries). */
  idempotent?: boolean | string
  stepUp?: string
  signal?: AbortSignal
}

export interface ClientOptions {
  /** '' for same-origin (web), e.g. 'http://192.168.0.10:8000' on a phone. */
  baseURL: string
  save: (refreshToken: string | null) => void | Promise<void>
}

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  // React Native without crypto.randomUUID: RFC 4122 v4 from Math.random is
  // fine for an idempotency key (uniqueness, not secrecy).
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16)
  })
}

export function createClient(opts: ClientOptions) {
  let access: string | null = null
  let refreshToken: string | null = null
  let refreshing: Promise<boolean> | null = null
  const listeners = new Set<(signedIn: boolean) => void>()
  const url = (path: string) => opts.baseURL + path

  function setTokens(t: Tokens | null) {
    access = t?.access_token ?? null
    refreshToken = t?.refresh_token ?? null
    void opts.save(refreshToken)
    listeners.forEach((l) => l(t !== null))
  }

  async function refresh(): Promise<boolean> {
    if (!refreshToken) return false
    const token = refreshToken
    refreshing ??= (async () => {
      try {
        const res = await fetch(url('/v1/auth/refresh'), { method: 'POST', body: JSON.stringify({ refresh_token: token }) })
        if (res.status === 401 || res.status === 400) {
          setTokens(null)
          return false
        }
        if (!res.ok) return false // gateway down: keep the session, try later
        setTokens(await res.json())
        return true
      } catch {
        return false
      } finally {
        refreshing = null
      }
    })()
    return refreshing
  }

  async function api<T>(method: string, path: string, o: RequestOptions = {}, retried = false): Promise<T> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (!access && !retried && refreshToken) await refresh()
    if (access) headers.Authorization = `Bearer ${access}`
    if (o.idempotent) headers['Idempotency-Key'] = typeof o.idempotent === 'string' ? o.idempotent : newIdempotencyKey()
    if (o.stepUp) headers['X-Step-Up-Token'] = o.stepUp
    const res = await fetch(url(path), {
      method,
      headers,
      body: o.body === undefined ? undefined : JSON.stringify(o.body),
      signal: o.signal,
    })
    if (res.status === 401 && !retried && !path.startsWith('/v1/auth/') && (await refresh())) {
      return api<T>(method, path, o, true)
    }
    if (res.status === 204) return undefined as T
    const data = await res.json().catch(() => ({}))
    if (!res.ok) {
      const e = data?.error ?? {}
      throw new ApiError(res.status, e.code ?? `HTTP_${res.status}`, e.message ?? res.statusText)
    }
    return data as T
  }

  /** A text response (e.g. a CSV export), with the same auth and refresh. */
  async function text(path: string, retried = false): Promise<string> {
    if (!access && !retried && refreshToken) await refresh()
    const res = await fetch(url(path), { headers: access ? { Authorization: `Bearer ${access}` } : {} })
    if (res.status === 401 && !retried && (await refresh())) return text(path, true)
    if (!res.ok) {
      const e = (await res.json().catch(() => ({})))?.error ?? {}
      throw new ApiError(res.status, e.code ?? `HTTP_${res.status}`, e.message ?? res.statusText)
    }
    return res.text()
  }

  return {
    api,
    text,
    url,
    /** Restores a saved session (the refresh token from storage). */
    restore(token: string | null) {
      refreshToken = token
    },
    hasSession: () => access !== null || refreshToken !== null,
    onSessionChange(l: (signedIn: boolean) => void) {
      listeners.add(l)
      return () => {
        listeners.delete(l)
      }
    },
    /** A valid access token, refreshing if needed (for streams). */
    async ensureAccess(): Promise<string | null> {
      if (access) return access
      return (await refresh()) ? access : null
    },
    async login(email: string, password: string, totpCode?: string) {
      setTokens(await api<Tokens>('POST', '/v1/auth/login', { body: { email, password, totp_code: totpCode || undefined } }))
    },
    async register(email: string, password: string, fullName: string) {
      await api('POST', '/v1/auth/register', { body: { email, password, full_name: fullName } })
    },
    async logout() {
      const token = refreshToken
      setTokens(null)
      if (token) await fetch(url('/v1/auth/logout'), { method: 'POST', body: JSON.stringify({ refresh_token: token }) }).catch(() => {})
    },
    /** Re-confirms the password (and TOTP) for a sensitive action. */
    async stepUp(password: string, totpCode?: string): Promise<string> {
      const r = await api<{ step_up_token: string }>('POST', '/v1/auth/step-up', { body: { password, totp_code: totpCode || undefined } })
      return r.step_up_token
    },
  }
}

export type Client = ReturnType<typeof createClient>
