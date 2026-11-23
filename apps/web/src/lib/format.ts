// Formatting helpers shared by the pages.

export const cx = (...c: (string | false | undefined | null)[]) => c.filter(Boolean).join(' ')

export function dateBR(iso: string) {
  if (!iso) return '—'
  const d = iso.length === 10 ? new Date(`${iso}T12:00:00`) : new Date(iso)
  return d.toLocaleDateString('pt-BR')
}

export function dateTimeBR(iso: string) {
  return new Date(iso).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}


export const maskCPF = (v: string) =>
  v.replace(/\D/g, '').slice(0, 11).replace(/(\d{3})(\d)/, '$1.$2').replace(/(\d{3})(\d)/, '$1.$2').replace(/(\d{3})(\d{1,2})$/, '$1-$2')

export const typeLabels: Record<string, string> = {
  INTERNAL_TRANSFER: 'Transferência',
  PIX: 'Pix',
  PIX_OUT: 'Pix enviado',
  PIX_IN: 'Pix recebido',
  PIX_REFUND: 'Devolução Pix',
  DEBIT_CARD_PURCHASE: 'Compra no débito',
  CREDIT_INVOICE_PAYMENT: 'Pagamento de fatura',
  BOLETO_PAYMENT: 'Boleto',
  CARD_REFUND: 'Estorno no cartão',
  SAVINGS_DEPOSIT: 'Guardado na caixinha',
  SAVINGS_WITHDRAWAL: 'Resgate da caixinha',
  MANUAL_CREDIT: 'Crédito do banco',
}
