import type { Money } from './money'

export interface Account {
  id: string
  holder_name: string
  branch: string
  number: string
  status: 'ACTIVE' | 'BLOCKED' | 'CLOSED'
  opened_at: string
}

export interface Balance {
  available: Money
  posted: Money
  pending: Money
}

export interface FeedItem {
  id: string
  type: string
  direction: 'DEBIT' | 'CREDIT'
  amount: Money
  counterparty_name: string
  description: string
  occurred_at: string
  end_to_end_id?: string
}

export interface Receipt {
  item: FeedItem
  holder_name: string
  branch: string
  number: string
  counterparty_branch: string
  counterparty_number: string
  authentication_code: string
}

export interface Profile {
  full_name: string
  cpf: string
  birth_date: string
  phone: string
  kyc_status: 'NOT_STARTED' | 'PENDING' | 'APPROVED' | 'REJECTED' | 'MANUAL_REVIEW'
  kyc_reason: string
  address: { postal_code: string; street: string; number: string; complement: string; district: string; city: string; state: string }
}

export interface PixKey {
  id: string
  type: 'CPF' | 'EMAIL' | 'PHONE' | 'EVP'
  value: string
  created_at: string
}

export interface Party {
  name: string
  document: string
  bank: string
}

export interface PixTransfer {
  id: string
  status: string
  key: string
  counterparty: Party
  amount: Money
  description: string
  created_at: string
  refunded?: Money
}

export interface Card {
  id: string
  kind: 'PHYSICAL' | 'VIRTUAL'
  status: 'ACTIVE' | 'BLOCKED' | 'CANCELLED'
  status_reason: string
  last4: string
  holder_name: string
  expiry_month: number
  expiry_year: number
  controls: { online: boolean; international: boolean; contactless: boolean }
}

export interface CardDetails {
  pan: string
  expiry_month: number
  expiry_year: number
  cvv: string
}

export interface CardToken {
  id: string
  wallet: string
  device_name: string
  status: 'ACTIVE' | 'SUSPENDED' | 'DELETED'
  last4: string
  token_pan?: string
}

export interface CreditLine {
  id: string
  status: string
  limit: Money
  used: Money
  available: Money
  closing_day: number
  monthly_interest_rate: number
  /** Installments of past purchases still to come (part of used). */
  upcoming_installments?: Money
}

export interface InstallmentPlan {
  description: string
  total: Money
  installments: number
  billed: number
  installment_amount: Money
  remaining: Money
  purchased_at: string
}

export interface Invoice {
  id: string
  status: 'OPEN' | 'CLOSED' | 'PAID' | 'OVERDUE'
  closing_date: string
  due_date: string
  total: Money
  paid: Money
  remaining: Money
  minimum_payment?: Money
  items?: { kind: string; amount: Money; description: string; occurred_at: string }[]
}

export interface PlanQuote {
  installments: number
  installment: Money
  total: Money
  interest: Money
  monthly_rate: number
  /** CET: total yearly cost, in percent. */
  annual_cost: number
}

export interface BoletoQuote {
  line: string
  formatted_line: string
  payee: { name: string; document: string; bank: string }
  due_date: string
  nominal: Money
  fine: Money
  interest: Money
  total: Money
}

export interface BoletoPayment {
  id: string
  status: string
  failure_reason: string
  payee: { name: string; bank: string }
  due_date: string
  amount: Money
  scheduled_for: string
  created_at: string
}

export interface Notification {
  id: string
  category: string
  title: string
  body: string
  occurred_at: string
  read: boolean
}

export type DisputeReason = 'NOT_RECOGNIZED' | 'NOT_RECEIVED' | 'DUPLICATE' | 'WRONG_AMOUNT' | 'CANCELLED'

export interface Dispute {
  id: string
  authorization_id: string
  card_id: string
  account_id: string
  reason: DisputeReason
  amount: Money
  description: string
  status: 'OPEN' | 'WON' | 'LOST'
  decided_by: string
  decision_reason: string
  created_at: string
  decided_at?: string
  merchant_name: string
  purchased_at: string
}

export interface CardPurchase {
  id: string
  status: 'APPROVED' | 'DECLINED' | 'CLEARED' | 'REVERSED'
  response_code: string
  reason: string
  amount: Money
  cleared_amount: Money
  refunded_amount: Money
  merchant_name: string
  function: 'DEBIT' | 'CREDIT'
  installments?: number
  created_at: string
  dispute?: Dispute
}

export const disputeReasons: Record<DisputeReason, string> = {
  NOT_RECOGNIZED: 'Não reconheço esta compra',
  NOT_RECEIVED: 'Não recebi o produto ou serviço',
  DUPLICATE: 'Fui cobrado duas vezes',
  WRONG_AMOUNT: 'Valor diferente do combinado',
  CANCELLED: 'Cancelei e continuei sendo cobrado',
}

export interface SavingsBox {
  id: string
  name: string
  goal?: Money
  balance: Money
  yielded: Money
  status: 'ACTIVE' | 'CLOSED'
  created_at: string
  annual_rate_bp: number
  movements?: SavingsMovement[]
}

export interface SavingsMovement {
  id: string
  kind: 'DEPOSIT' | 'WITHDRAWAL' | 'YIELD'
  amount: Money
  status: 'PENDING' | 'COMPLETED' | 'FAILED'
  failure_reason: string
  created_at: string
}

export interface SavingsSummary {
  boxes: SavingsBox[]
  total: Money
  yielded: Money
}

export type SpendingCategory =
  | 'FOOD' | 'TRANSPORT' | 'SHOPPING' | 'SUBSCRIPTIONS' | 'HEALTH' | 'LEISURE' | 'TRAVEL' | 'HOME' | 'EDUCATION' | 'BILLS' | 'TRANSFERS' | 'OTHER'

export interface SpendingSummary {
  month: string
  total: Money
  previous_total: Money
  categories: { category: SpendingCategory; amount: Money; count: number }[]
  merchants: { name: string; category: SpendingCategory; amount: Money; count: number }[]
}

/** Label and chart color of each spending category (the same in both themes). */
export const spendingCategories: Record<SpendingCategory, { label: string; color: string }> = {
  FOOD: { label: 'Alimentação', color: '#f59e0b' },
  TRANSPORT: { label: 'Transporte', color: '#0ea5e9' },
  SHOPPING: { label: 'Compras', color: '#8b5cf6' },
  SUBSCRIPTIONS: { label: 'Assinaturas', color: '#ec4899' },
  HEALTH: { label: 'Saúde', color: '#10b981' },
  LEISURE: { label: 'Lazer', color: '#f97316' },
  TRAVEL: { label: 'Viagens', color: '#06b6d4' },
  HOME: { label: 'Casa', color: '#84cc16' },
  EDUCATION: { label: 'Educação', color: '#6366f1' },
  BILLS: { label: 'Contas e boletos', color: '#64748b' },
  TRANSFERS: { label: 'Pix e transferências', color: '#14b8a6' },
  OTHER: { label: 'Outros', color: '#a1a1aa' },
}

/** The month before or after a YYYY-MM month. */
export function shiftMonth(month: string, delta: number): string {
  const [y, m] = month.split('-').map(Number)
  const d = new Date(Date.UTC(y, m - 1 + delta, 1))
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
}

/** "Novembro de 2026" for 2026-11. */
export function monthLabel(month: string): string {
  const [y, m] = month.split('-').map(Number)
  const s = new Date(Date.UTC(y, m - 1, 15)).toLocaleDateString('pt-BR', { month: 'long', year: 'numeric', timeZone: 'UTC' })
  return s.charAt(0).toUpperCase() + s.slice(1)
}

export interface LimitRequest {
  id: string
  account_id: string
  current_limit: Money
  requested: Money
  status: 'APPROVED' | 'REJECTED' | 'IN_REVIEW'
  granted: Money
  reason: string
  decided_by: string
  created_at: string
  decided_at?: string
  // Back office only.
  holder_name?: string
  customer_id?: string
  used?: Money
}
