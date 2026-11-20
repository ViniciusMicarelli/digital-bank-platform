import { errorMessage, formatBRL, type Receipt as ReceiptT } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import { Check, Copy, Printer, Share2, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { api } from '../lib/api'
import { dateTimeBR, typeLabels } from '../lib/format'
import { Brand } from './Brand'
import { Alert, Button, Loading, Modal } from './ui'

const fullDate = (iso: string) => new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'medium' })
const account = (branch: string, number: string) => (number ? `Ag ${branch} · Conta ${number}` : '')

/** The receipt body: shared by the modal and the public check page. */
export function ReceiptBody({ r, verified }: { r: ReceiptT; verified?: boolean }) {
  const debit = r.item.direction === 'DEBIT'
  const me = { name: r.holder_name, detail: account(r.branch, r.number) + (account(r.branch, r.number) ? ' · Digital Bank' : 'Digital Bank') }
  const them = { name: r.item.counterparty_name || r.item.description || '—', detail: account(r.counterparty_branch, r.counterparty_number) }
  const [from, to] = debit ? [me, them] : [them, me]
  const rows: [string, string][] = [
    ['Tipo', typeLabels[r.item.type] ?? r.item.type],
    ['Data e hora', fullDate(r.item.occurred_at)],
    ...(r.item.description ? [['Descrição', r.item.description] as [string, string]] : []),
    ...(r.item.end_to_end_id ? [['ID da transação Pix', r.item.end_to_end_id] as [string, string]] : []),
  ]
  return (
    <div className="receipt space-y-5 text-sm">
      <div className="flex items-center justify-between">
        <Brand />
        {verified && <span className="flex items-center gap-1.5 text-xs font-semibold text-positive"><ShieldCheck className="size-4" /> Autêntico</span>}
      </div>
      <div>
        <p className="text-muted">{debit ? 'Você enviou' : 'Você recebeu'}</p>
        <p className="tabular text-3xl font-bold tracking-tight text-fg">{formatBRL(r.item.amount)}</p>
      </div>
      <div className="grid gap-3 rounded-2xl bg-surface-2 p-4">
        <div><p className="text-xs text-muted">De</p><p className="font-semibold text-fg">{from.name}</p>{from.detail && <p className="text-xs text-muted">{from.detail}</p>}</div>
        <div><p className="text-xs text-muted">Para</p><p className="font-semibold text-fg">{to.name}</p>{to.detail && <p className="text-xs text-muted">{to.detail}</p>}</div>
      </div>
      <dl className="space-y-2">
        {rows.map(([k, v]) => (
          <div key={k} className="flex justify-between gap-4"><dt className="text-muted">{k}</dt><dd className="text-right font-medium break-all text-fg">{v}</dd></div>
        ))}
      </dl>
      <div className="border-t border-dashed border-line pt-4">
        <p className="text-xs text-muted">Código de autenticação</p>
        <p className="mt-1 font-mono text-xs break-all text-fg">{r.authentication_code}</p>
      </div>
    </div>
  )
}

export function ReceiptModal({ accountId, itemId, onClose }: { accountId: string; itemId: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const canShare = typeof navigator.share === 'function'
  const receipt = useQuery({
    queryKey: ['receipt', itemId],
    queryFn: () => api<ReceiptT>('GET', `/v1/accounts/${accountId}/statement/${itemId}/receipt`),
  })
  const r = receipt.data
  const link = r ? `${window.location.origin}/comprovante/${r.authentication_code}` : ''
  const share = async () => {
    const text = `Comprovante Digital Bank: ${formatBRL(r!.item.amount)} em ${dateTimeBR(r!.item.occurred_at)}. Confira: ${link}`
    if (canShare) {
      await navigator.share({ title: 'Comprovante', text }).catch(() => {})
      return
    }
    await navigator.clipboard.writeText(text)
    setCopied(true)
  }
  return (
    <Modal open onClose={onClose} title="Comprovante">
      {receipt.isLoading ? <Loading rows={4} /> : receipt.isError ? <Alert>{errorMessage(receipt.error)}</Alert> : r && (
        <div className="space-y-5">
          <ReceiptBody r={r} />
          <p className="text-xs text-muted">Qualquer pessoa pode conferir este comprovante em <a href={link} className="font-medium break-all text-brand-strong">{link}</a></p>
          <div className="flex flex-wrap gap-2 print:hidden">
            <Button icon={Printer} variant="secondary" onClick={() => window.print()}>Imprimir ou salvar PDF</Button>
            <Button icon={copied ? Check : canShare ? Share2 : Copy} onClick={() => void share()}>{copied ? 'Copiado' : 'Compartilhar'}</Button>
          </div>
        </div>
      )}
    </Modal>
  )
}
