import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, createClient } from './client'

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status })

describe('client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('refreshes once on 401 and retries, saving the new refresh token', async () => {
    const saved: (string | null)[] = []
    const c = createClient({ baseURL: 'http://api', save: (t) => { saved.push(t) } })
    c.restore('r1')
    const calls: string[] = []
    let accounts = 0
    vi.stubGlobal('fetch', vi.fn(async (u: string, init: RequestInit) => {
      calls.push(`${init.method} ${u}`)
      if (u.endsWith('/v1/auth/refresh')) return json(200, { access_token: 'a2', refresh_token: 'r2' })
      accounts++
      return accounts === 1 && !(init.headers as Record<string, string>).Authorization?.includes('a2')
        ? json(401, { error: { code: 'UNAUTHENTICATED' } })
        : json(200, { accounts: [] })
    }))
    await expect(c.api('GET', '/v1/accounts')).resolves.toEqual({ accounts: [] })
    expect(saved).toEqual(['r2'])
    expect(calls.filter((x) => x.includes('refresh'))).toHaveLength(1)
  })

  it('turns error envelopes into ApiError', async () => {
    const c = createClient({ baseURL: '', save: () => {} })
    vi.stubGlobal('fetch', vi.fn(async () => json(422, { error: { code: 'INSUFFICIENT_FUNDS', message: 'no' } })))
    const err = await c.api('POST', '/v1/x').catch((e) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err.code).toBe('INSUFFICIENT_FUNDS')
  })

  it('keeps the session when the gateway is down during refresh', async () => {
    const saved: (string | null)[] = []
    const c = createClient({ baseURL: '', save: (t) => { saved.push(t) } })
    c.restore('r1')
    vi.stubGlobal('fetch', vi.fn(async () => json(503, {})))
    expect(await c.ensureAccess()).toBeNull()
    expect(c.hasSession()).toBe(true)
    expect(saved).toEqual([])
  })
})
