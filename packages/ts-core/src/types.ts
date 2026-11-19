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
}

export interface Invoice {
  id: string
  status: 'OPEN' | 'CLOSED' | 'PAID' | 'OVERDUE'
  closing_date: string
  due_date: string
  total: Money
  paid: Money
  remaining: Money
  items?: { kind: string; amount: Money; description: string; occurred_at: string }[]
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
