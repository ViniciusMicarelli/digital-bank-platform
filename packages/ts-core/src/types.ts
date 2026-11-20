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
  kind: 'PHYSICAL' | 'VIRTUAL' | 'TEMPORARY'
  status: 'ACTIVE' | 'BLOCKED' | 'CANCELLED'
  status_reason: string
  /** Temporary cards: when they stop working. */
  valid_until?: string
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
  /** The monthly deposit, when there is one. */
  schedule?: SavingsSchedule
  movements?: SavingsMovement[]
}

/** A monthly deposit from the checking account into a box. */
export interface SavingsSchedule {
  amount: Money
  /** 1..28 of every month. */
  day: number
  /** YYYY-MM-DD. */
  next_run: string
  /** Why the last run moved nothing (e.g. INSUFFICIENT_FUNDS); empty if it ran. */
  last_failure: string
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

export type BudgetCategory = SpendingCategory | 'TOTAL'

/** A monthly budget with what was spent against it in the month asked for. */
export interface Budget {
  category: BudgetCategory
  amount: Money
  spent: Money
}

/** Name of what a budget is on. */
export const budgetLabel = (c: BudgetCategory) => (c === 'TOTAL' ? 'Total do mês' : spendingCategories[c].label)

/** Where a budget stands: under 80%, from 80%, or at / over 100% (the alerts' thresholds). */
export function budgetState(b: Budget): { pct: number; state: 'ok' | 'near' | 'over'; label: string } {
  const amount = Number(b.amount.amount)
  const pct = amount > 0 ? (Number(b.spent.amount) / amount) * 100 : 0
  if (pct >= 100) return { pct, state: 'over', label: pct > 100 ? 'Estourado' : 'No limite' }
  if (pct >= 80) return { pct, state: 'near', label: 'Perto do limite' }
  return { pct, state: 'ok', label: 'Dentro do orçamento' }
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
  /** The customer's score when the request was analyzed. */
  score?: number
  // Back office only.
  holder_name?: string
  customer_id?: string
  used?: Money
}

export interface PixContact {
  key: string
  name: string
  bank_name: string
  nickname: string
  favorite: boolean
  last_sent_at?: string
  times_sent: number
}

export type ScoreBand = 'LOW' | 'FAIR' | 'GOOD' | 'EXCELLENT'
export type ScoreFactorCode = 'PAYMENT_HISTORY' | 'LATE_PAYMENTS' | 'OVERDUE_NOW' | 'UTILIZATION' | 'RELATIONSHIP' | 'INVOICE_PLANS'

export interface ScoreFactor {
  code: ScoreFactorCode
  /** Points added (negative: taken) to the base of 500. */
  points: number
  /** A count, the % of the limit used, or months. */
  value: number
}

/** The bank's behaviour score for the card, 0..1000. */
export interface CreditScore {
  score: number
  band: ScoreBand
  factors: ScoreFactor[]
  /** How much more the automatic analysis grants at once (basis points; 0: an analyst decides). */
  auto_increase_bp: number
}

/** Band names; each app colors them with its theme's status tokens (danger, warning, positive, credit). */
export const scoreBands: Record<ScoreBand, string> = { LOW: 'Baixo', FAIR: 'Regular', GOOD: 'Bom', EXCELLENT: 'Excelente' }

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

/** What a factor means to the customer, in pt-BR: a title and the detail behind it. */
export function scoreFactorText(f: ScoreFactor): { title: string; detail: string } {
  switch (f.code) {
    case 'PAYMENT_HISTORY':
      return { title: 'Faturas pagas em dia', detail: f.value ? `${plural(f.value, 'fatura paga', 'faturas pagas')} em dia nos últimos 12 meses.` : 'Pague as faturas até o vencimento para subir o score.' }
    case 'LATE_PAYMENTS':
      return { title: 'Atrasos', detail: f.value ? `${plural(f.value, 'fatura paga', 'faturas pagas')} com atraso nos últimos 12 meses.` : 'Nenhum atraso nos últimos 12 meses.' }
    case 'OVERDUE_NOW':
      return { title: 'Fatura em atraso', detail: f.value ? 'Há uma fatura vencida e não paga agora.' : 'Nenhuma fatura vencida agora.' }
    case 'UTILIZATION':
      return { title: 'Uso do limite', detail: `Você está usando ${f.value}% do limite. Até 30% é o ideal.` }
    case 'RELATIONSHIP':
      return { title: 'Tempo de cartão', detail: f.value ? `${plural(f.value, 'mês', 'meses')} de relacionamento.` : 'Começa a contar no primeiro mês.' }
    case 'INVOICE_PLANS':
      return { title: 'Parcelamento de fatura', detail: f.value ? `${plural(f.value, 'fatura parcelada', 'faturas parceladas')} nos últimos 12 meses.` : 'Nenhuma fatura parcelada nos últimos 12 meses.' }
  }
}
