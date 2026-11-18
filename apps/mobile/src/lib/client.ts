import { createClient, fetchTransport, xhrTransport } from '@bank/core'
import Constants from 'expo-constants'
import * as Crypto from 'expo-crypto'
import * as SecureStore from 'expo-secure-store'
import { Platform } from 'react-native'

// crypto.randomUUID for idempotency keys (Hermes does not have it).
const g = globalThis as { crypto?: { randomUUID?: () => string } }
g.crypto ??= {}
g.crypto.randomUUID ??= () => Crypto.randomUUID()

/**
 * The gateway: EXPO_PUBLIC_API_URL, or the computer running the Expo dev
 * server (a phone on the same Wi-Fi reaches it by its LAN address), port 8000.
 */
function apiURL(): string {
  if (process.env.EXPO_PUBLIC_API_URL) return process.env.EXPO_PUBLIC_API_URL
  if (Platform.OS === 'web') return 'http://localhost:8000'
  const host = Constants.expoConfig?.hostUri?.split(':')[0] ?? 'localhost'
  return `http://${host}:8000`
}

const KEY = 'db.refresh'

// The refresh token goes to the Keychain (iOS) / Keystore (Android); on the
// web build, to sessionStorage.
const storage = {
  async get(): Promise<string | null> {
    if (Platform.OS === 'web') return globalThis.sessionStorage?.getItem(KEY) ?? null
    return SecureStore.getItemAsync(KEY)
  },
  async set(token: string | null) {
    if (Platform.OS === 'web') {
      if (token) globalThis.sessionStorage?.setItem(KEY, token)
      else globalThis.sessionStorage?.removeItem(KEY)
      return
    }
    if (token) await SecureStore.setItemAsync(KEY, token, { keychainAccessible: SecureStore.WHEN_UNLOCKED_THIS_DEVICE_ONLY })
    else await SecureStore.deleteItemAsync(KEY)
  },
}

export const client = createClient({ baseURL: apiURL(), save: storage.set })
export const api = client.api
export const streamTransport = Platform.OS === 'web' ? fetchTransport : xhrTransport

/** Loads the saved session before the first screen. */
export async function restoreSession() {
  client.restore(await storage.get())
}
