import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { api, ApiError } from '../lib/api'
import type { Account, Balance, FeedItem, Profile } from '@bank/core'

export const keys = {
  profile: ['profile'] as const,
  accounts: ['accounts'] as const,
  balance: (id: string) => ['balance', id] as const,
  statement: (id: string) => ['statement', id] as const,
  notifications: ['notifications'] as const,
}

/** The profile, or null when the customer has not started onboarding. */
export function useProfile() {
  return useQuery({
    queryKey: keys.profile,
    queryFn: async () => {
      try {
        return await api<Profile>('GET', '/v1/me/profile')
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null
        throw e
      }
    },
  })
}

export function useAccounts() {
  return useQuery({
    queryKey: keys.accounts,
    queryFn: async () => (await api<{ accounts: Account[] }>('GET', '/v1/accounts')).accounts,
  })
}

/** The account the app works with: the first active one. */
export function useAccount(): Account | undefined {
  const { data } = useAccounts()
  // A closed account is history (statement, receipts), not one to work with.
  return data?.find((a) => a.status === 'ACTIVE') ?? data?.find((a) => a.status !== 'CLOSED')
}

export function useBalance(id?: string) {
  return useQuery({
    queryKey: keys.balance(id ?? ''),
    queryFn: () => api<Balance>('GET', `/v1/accounts/${id}/balance`),
    enabled: !!id,
  })
}

export function useStatement(accountId?: string, pageSize = 20) {
  return useInfiniteQuery({
    queryKey: ['statement', accountId, pageSize],
    enabled: !!accountId,
    initialPageParam: '',
    queryFn: ({ pageParam }) =>
      api<{ items: FeedItem[]; next_cursor: string }>('GET', `/v1/accounts/${accountId}/statement?limit=${pageSize}${pageParam ? `&cursor=${encodeURIComponent(pageParam)}` : ''}`),
    getNextPageParam: (last) => last.next_cursor || undefined,
    // The statement is a projection that can trail the events; keep it fresh.
    refetchInterval: 10_000,
  })
}

/** Who is signed in, and with which role (CUSTOMER or ANALYST). */
export function useMe() {
  return useQuery({ queryKey: ['me'], queryFn: () => api<{ customer_id: string; role: 'CUSTOMER' | 'ANALYST'; mfa: boolean }>('GET', '/v1/me'), staleTime: Infinity })
}
