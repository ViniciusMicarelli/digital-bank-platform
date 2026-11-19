import { ApiError, type Account, type Balance, type FeedItem, type Profile } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import { createContext, useContext } from 'react'
import { api } from './client'

export const SessionContext = createContext(false)
export const useSignedIn = () => useContext(SessionContext)

export function useProfile() {
  return useQuery({
    queryKey: ['profile'],
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
  return useQuery({ queryKey: ['accounts'], queryFn: async () => (await api<{ accounts: Account[] }>('GET', '/v1/accounts')).accounts })
}

export function useAccount(): Account | undefined {
  const { data } = useAccounts()
  return data?.find((a) => a.status === 'ACTIVE') ?? data?.[0]
}

export function useBalance(id?: string) {
  return useQuery({ queryKey: ['balance', id], enabled: !!id, queryFn: () => api<Balance>('GET', `/v1/accounts/${id}/balance`) })
}

export function useStatement(id?: string, limit = 30) {
  return useQuery({
    queryKey: ['statement', id, limit],
    enabled: !!id,
    // The statement is a projection that can trail the events; keep it fresh.
    refetchInterval: 10_000,
    queryFn: async () => (await api<{ items: FeedItem[] }>('GET', `/v1/accounts/${id}/statement?limit=${limit}`)).items,
  })
}

export const typeLabels: Record<string, string> = {
  INTERNAL_TRANSFER: 'Transferência', PIX: 'Pix', PIX_OUT: 'Pix enviado', PIX_IN: 'Pix recebido', PIX_REFUND: 'Devolução Pix',
  DEBIT_CARD_PURCHASE: 'Compra no débito', CREDIT_INVOICE_PAYMENT: 'Pagamento de fatura', BOLETO_PAYMENT: 'Boleto',
  CARD_REFUND: 'Estorno no cartão',
}

export const dateTime = (iso: string) =>
  new Date(iso).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
export const date = (iso: string) => (iso ? new Date(iso.length === 10 ? `${iso}T12:00:00` : iso).toLocaleDateString('pt-BR') : '—')

export function useMe() {
  return useQuery({ queryKey: ['me'], queryFn: () => api<{ customer_id: string; role: 'CUSTOMER' | 'ANALYST' }>('GET', '/v1/me'), staleTime: Infinity })
}
