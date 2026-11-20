// The web app's API client: same origin (Vite proxy / nginx), refresh
// token in sessionStorage (cleared when the tab closes).
import { createClient } from '@bank/core'

export { ApiError, newIdempotencyKey } from '@bank/core'
export type { RequestOptions } from '@bank/core'

const KEY = 'db.refresh'

export const client = createClient({
  baseURL: '',
  save: (token) => {
    try {
      if (token) sessionStorage.setItem(KEY, token)
      else sessionStorage.removeItem(KEY)
    } catch {
      /* private mode: the session lasts while the page is open */
    }
  },
})

try {
  client.restore(sessionStorage.getItem(KEY))
} catch {
  /* no storage */
}

export const api = client.api
export const apiText = client.text
export const login = client.login
export const register = client.register
export const logout = client.logout
export const stepUp = client.stepUp
export const ensureAccess = client.ensureAccess
export const hasSession = client.hasSession
export const onSessionChange = client.onSessionChange
