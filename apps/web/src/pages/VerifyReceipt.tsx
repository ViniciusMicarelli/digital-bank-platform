import { errorMessage, type Receipt } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { useParams } from 'react-router'
import { ReceiptBody } from '../components/Receipt'
import { Alert, Loading } from '../components/ui'
import { api } from '../lib/api'

/** Public: anyone holding a receipt checks it was issued by this bank. */
export function VerifyReceiptPage() {
  const { code = '' } = useParams()
  const check = useQuery({ queryKey: ['verify', code], queryFn: () => api<Receipt>('GET', `/v1/receipts/${encodeURIComponent(code)}`), retry: false })
  return (
    <main className="safe-top mx-auto max-w-md px-5 py-10">
      <h1 className="mb-1 text-2xl font-bold tracking-tight text-fg">Conferir comprovante</h1>
      <p className="mb-6 text-sm text-muted">Confirma se um comprovante foi emitido pelo Digital Bank. Os nomes aparecem abreviados.</p>
      <div className="glass rounded-[28px] p-6">
        {check.isLoading ? <Loading rows={4} /> : check.isError ? (
          <div className="space-y-3 text-center">
            <ShieldAlert className="mx-auto size-10 text-danger" />
            <Alert>{errorMessage(check.error)}</Alert>
          </div>
        ) : check.data && <ReceiptBody r={check.data} verified />}
      </div>
    </main>
  )
}
