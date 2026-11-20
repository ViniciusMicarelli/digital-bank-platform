import { addBRL, ApiError, errorMessage, formatBRL, monthLabel, type Card, type CreditLine, type FeedItem, type Invoice, type SavingsSummary } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowDownLeft, ArrowLeftRight, ArrowUpRight, Barcode, Bell, ChevronRight, CreditCard, Eye, EyeOff, FileText, PieChart, PiggyBank, QrCode, ShoppingBag, Sparkles,
  Undo2, Wallet, Zap, type LucideIcon, Download } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { useAccount, useBalance, useStatement } from '../app/queries'
import { BankCard } from '../components/BankCard'
import { ReceiptModal } from '../components/Receipt'
import { Amount, Button, Empty, IconTile, Loading, Panel, Skeleton } from '../components/ui'
import { api, apiText } from '../lib/api'
import { cx, dateTimeBR, typeLabels } from '../lib/format'
import { useCountUp } from '../lib/motion'

const typeIcons: Record<string, LucideIcon> = {
  PIX: ArrowLeftRight, PIX_OUT: ArrowUpRight, PIX_IN: ArrowDownLeft, PIX_REFUND: Undo2, INTERNAL_TRANSFER: ArrowLeftRight,
  DEBIT_CARD_PURCHASE: ShoppingBag, CREDIT_INVOICE_PAYMENT: Wallet, BOLETO_PAYMENT: FileText, CARD_REFUND: Undo2,
  SAVINGS_DEPOSIT: PiggyBank, SAVINGS_WITHDRAWAL: PiggyBank,
}

export function FeedRow({ item }: { item: FeedItem }) {
  const credit = item.direction === 'CREDIT'
  const account = useAccount()
  const [open, setOpen] = useState(false)
  return (
    <li>
     <button onClick={() => setOpen(true)} aria-label={`Comprovante: ${item.counterparty_name || typeLabels[item.type] || item.type}`}
       className="-mx-2 flex w-[calc(100%+1rem)] items-center gap-3.5 rounded-xl px-2 py-3 text-left transition hover:bg-surface-2">
      <IconTile icon={credit ? ArrowDownLeft : (typeIcons[item.type] ?? ArrowUpRight)} tone={credit ? 'green' : 'slate'} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-semibold text-fg">{item.counterparty_name || item.description || typeLabels[item.type] || item.type}</p>
        <p className="text-xs text-muted">{typeLabels[item.type] ?? item.type} · {dateTimeBR(item.occurred_at)}</p>
      </div>
      <Amount value={item.amount} direction={item.direction} className="text-sm" />
     </button>
     {open && account && <ReceiptModal accountId={account.id} itemId={item.id} onClose={() => setOpen(false)} />}
    </li>
  )
}

const cents = (amount?: string) => (amount ? Number(amount) : 0)

function Initials({ name }: { name?: string }) {
  const p = (name ?? '').split(' ').filter(Boolean)
  return (
    <span className="gloss grid size-12 place-items-center rounded-full bg-brand-soft text-[15px] font-bold text-brand-strong">
      {(p[0]?.[0] ?? '') + (p.at(-1)?.[0] ?? '')}
    </span>
  )
}

function Account({ hidden }: { hidden: boolean }) {
  const account = useAccount()
  const balance = useBalance(account?.id)
  const feed = useStatement(account?.id, 50)
  const available = balance.data?.available.amount
  const ref = useCountUp(cents(available), (n) => formatBRL(n.toFixed(2)))
  const [month] = useState(() => new Date().toISOString().slice(0, 7))
  const items = (feed.data?.pages.flatMap((p) => p.items) ?? []).filter((i) => i.occurred_at.startsWith(month))
  const sum = (d: FeedItem['direction']) => addBRL('0.00', ...items.filter((i) => i.direction === d).map((i) => i.amount.amount))
  return (
    <Link to="/extrato" data-reveal className="glass group block rounded-[30px] p-6 transition hover:-translate-y-0.5 sm:p-7">
      <div className="flex items-center justify-between">
        <p className="text-[17px] font-semibold text-fg">Conta</p>
        <ChevronRight className="size-5 text-subtle transition group-hover:translate-x-0.5" />
      </div>
      <p className="mt-1 text-[13px] text-muted">Saldo disponível</p>
      <p className="tabular mt-0.5 text-[40px] leading-tight font-bold tracking-[-0.03em] text-fg">
        {balance.isLoading ? <span className="skeleton inline-block h-10 w-52 align-middle" /> : hidden ? 'R$ ••••••' : <span ref={ref}>{formatBRL(available)}</span>}
      </p>
      {!hidden && balance.data && balance.data.pending.amount !== '0.00' && (
        <p className="mt-1 text-[13px] text-muted">{formatBRL(balance.data.pending)} reservado em compras e Pix em andamento</p>
      )}
      <div className="mt-5 flex flex-wrap gap-2">
        <span className="inline-flex items-center gap-1.5 rounded-full bg-positive-soft px-3 py-1.5 text-xs font-semibold text-positive">
          <ArrowDownLeft className="size-3.5" /> Entradas no mês {hidden ? '••••' : formatBRL(sum('CREDIT'))}
        </span>
        <span className="inline-flex items-center gap-1.5 rounded-full bg-surface-2 px-3 py-1.5 text-xs font-semibold text-fg-2">
          <ArrowUpRight className="size-3.5" /> Saídas no mês {hidden ? '••••' : formatBRL(sum('DEBIT'))}
        </span>
      </div>
    </Link>
  )
}

const shortcuts: [string, string, LucideIcon][] = [
  ['/pix', 'Área Pix', Zap],
  ['/boletos', 'Pagar', Barcode],
  ['/pix?aba=enviar', 'Transferir', ArrowUpRight],
  ['/pix?aba=cobrar', 'Cobrar', QrCode],
  ['/caixinhas', 'Caixinhas', PiggyBank],
  ['/gastos', 'Gastos', PieChart],
  ['/credito', 'Crédito', Wallet],
  ['/cartoes', 'Cartões', CreditCard],
]

function Shortcuts() {
  return (
    <div data-reveal className="no-scrollbar -mx-4 flex gap-4 overflow-x-auto px-4 pb-1 sm:mx-0 sm:grid sm:grid-cols-8 sm:px-0">
      {shortcuts.map(([to, label, Icon]) => (
        <Link key={to} to={to} className="group flex w-[72px] shrink-0 flex-col items-center gap-2 text-center sm:w-auto">
          <span className="glass grid size-[68px] place-items-center rounded-full text-fg transition duration-300 group-hover:-translate-y-1 group-hover:text-brand-strong group-active:scale-90">
            <Icon className="size-6" strokeWidth={2} />
          </span>
          <span className="text-[13px] leading-tight font-medium text-fg">{label}</span>
        </Link>
      ))}
    </div>
  )
}

function useCredit(accountId?: string) {
  const line = useQuery({
    queryKey: ['credit', accountId],
    enabled: !!accountId,
    queryFn: async () => {
      try {
        return await api<CreditLine>('GET', `/v1/accounts/${accountId}/credit`)
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null
        throw e
      }
    },
  })
  const invoices = useQuery({
    queryKey: ['invoices', accountId],
    enabled: !!line.data,
    queryFn: async () => (await api<{ invoices: Invoice[] }>('GET', `/v1/accounts/${accountId}/credit/invoices`)).invoices,
  })
  return { line: line.data, current: invoices.data?.find((i) => i.status === 'OPEN'), loading: line.isLoading }
}

function CreditSection({ accountId, hidden }: { accountId: string; hidden: boolean }) {
  const { line, current, loading } = useCredit(accountId)
  if (loading) return <Panel><Skeleton className="h-24" /></Panel>
  if (!line) {
    return (
      <Link to="/credito" data-reveal className="glass flex items-center gap-4 rounded-[30px] p-5 transition hover:-translate-y-0.5">
        <IconTile icon={Sparkles} tone="credit" className="size-12" />
        <div className="min-w-0 flex-1">
          <p className="text-[15px] font-semibold text-fg">Ative seu cartão de crédito</p>
          <p className="text-[13px] text-muted">Limite de R$ 1.000,00 no mesmo cartão. Sem anuidade.</p>
        </div>
        <ChevronRight className="size-5 text-subtle" />
      </Link>
    )
  }
  const pct = Math.min(100, (Number(line.used.amount) / Number(line.limit.amount)) * 100)
  return (
    <Link to="/credito" data-reveal className="glass group block rounded-[30px] p-6 transition hover:-translate-y-0.5">
      <div className="flex items-center justify-between">
        <p className="text-[17px] font-semibold text-fg">Cartão de crédito</p>
        <ChevronRight className="size-5 text-subtle transition group-hover:translate-x-0.5" />
      </div>
      <p className="mt-1 text-[13px] text-muted">Fatura atual</p>
      <p className="tabular text-[28px] font-bold tracking-tight text-credit">{hidden ? 'R$ ••••' : formatBRL(current?.total ?? '0.00')}</p>
      <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-surface-2">
        <div className="h-full rounded-full bg-gradient-to-r from-credit to-brand transition-all duration-700" style={{ width: `${pct}%` }} />
      </div>
      <p className="mt-2 text-[13px] text-muted">Limite disponível de {hidden ? '••••' : formatBRL(line.available)}</p>
    </Link>
  )
}

function SavingsSection({ accountId, hidden }: { accountId: string; hidden: boolean }) {
  const savings = useQuery({ queryKey: ['savings', accountId], queryFn: () => api<SavingsSummary>('GET', `/v1/accounts/${accountId}/savings`) })
  const s = savings.data
  if (!s) return null
  return (
    <Link to="/caixinhas" data-reveal className="glass group flex items-center gap-4 rounded-[30px] p-5 transition hover:-translate-y-0.5">
      <IconTile icon={PiggyBank} tone="brand" className="size-12" />
      <div className="min-w-0 flex-1">
        <p className="text-[15px] font-semibold text-fg">Caixinhas</p>
        <p className="text-[13px] text-muted">
          {s.boxes.length === 0 ? 'Separe dinheiro para seus planos, rendendo todo dia.'
            : <>Guardado <span className="tabular font-semibold text-fg">{hidden ? 'R$ ••••' : formatBRL(s.total)}</span> · rendeu {hidden ? '••••' : formatBRL(s.yielded)}</>}
        </p>
      </div>
      <ChevronRight className="size-5 text-subtle transition group-hover:translate-x-0.5" />
    </Link>
  )
}

function CardPreview({ accountId }: { accountId: string }) {
  const cards = useQuery({ queryKey: ['cards', accountId], queryFn: async () => (await api<{ cards: Card[] }>('GET', `/v1/accounts/${accountId}/cards`)).cards })
  const card = cards.data?.[0]
  if (!card) return null
  return (
    <Link to="/cartoes" data-reveal className="block">
      <BankCard holder={card.holder_name} last4={card.last4} expiry={`${String(card.expiry_month).padStart(2, '0')}/${String(card.expiry_year).slice(2)}`}
        skin={card.status === 'BLOCKED' ? 'blocked' : 'debit'} label={card.kind === 'VIRTUAL' ? 'Virtual' : 'Físico'} />
    </Link>
  )
}

export function HomePage() {
  const account = useAccount()
  const recent = useStatement(account?.id, 50)
  const items = (recent.data?.pages.flatMap((p) => p.items) ?? []).slice(0, 6)
  const [hidden, setHidden] = useState(false)
  const [hour] = useState(() => new Date().getHours())
  const greeting = hour < 12 ? 'Bom dia' : hour < 18 ? 'Boa tarde' : 'Boa noite'
  const round = 'glass grid size-11 place-items-center rounded-full text-fg-2 transition hover:text-fg active:scale-90'

  return (
    <div className="space-y-6">
      <header data-reveal className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Initials name={account?.holder_name} />
          <div>
            <p className="text-[13px] text-muted">{greeting},</p>
            <h1 className="text-[22px] leading-tight font-bold tracking-tight text-fg">{account?.holder_name.split(' ')[0]}</h1>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => setHidden(!hidden)} aria-label={hidden ? 'Mostrar saldo' : 'Ocultar saldo'} className={round}>
            {hidden ? <EyeOff className="size-5" /> : <Eye className="size-5" />}
          </button>
          <Link to="/notificacoes" aria-label="Notificações" className={round}><Bell className="size-5" /></Link>
        </div>
      </header>

      <div className="grid gap-6 xl:grid-cols-[1fr_360px]">
        <div className="min-w-0 space-y-6">
          <Account hidden={hidden} />
          <Shortcuts />
          <Link to="/cartoes" data-reveal className="glass flex items-center gap-3 rounded-full px-5 py-4 text-[15px] font-semibold text-fg transition hover:-translate-y-0.5">
            <CreditCard className="size-5 text-fg-2" /> Meus cartões <ChevronRight className="ml-auto size-5 text-subtle" />
          </Link>
          {account && <SavingsSection accountId={account.id} hidden={hidden} />}
          {account && <div className="xl:hidden"><CreditSection accountId={account.id} hidden={hidden} /></div>}
          <Panel title="Movimentações" action={<Link to="/extrato" className="text-sm font-semibold text-brand-strong">Ver tudo</Link>}>
            {recent.isLoading ? <Loading /> : items.length === 0 ? (
              <div className="text-center">
                <Empty icon={ArrowLeftRight}>Nenhuma movimentação ainda.</Empty>
                <Link to="/pix?aba=chaves"><Button variant="secondary">Cadastrar chave Pix e receber</Button></Link>
              </div>
            ) : (
              <ul className="-my-1 divide-y divide-line">{items.map((i) => <FeedRow key={i.id} item={i} />)}</ul>
            )}
          </Panel>
        </div>
        {account && (
          <aside className="hidden space-y-6 xl:block">
            <CardPreview accountId={account.id} />
            <CreditSection accountId={account.id} hidden={hidden} />
          </aside>
        )}
      </div>
    </div>
  )
}

/** Downloads a month of the statement as CSV (opens in Excel / Sheets). */
function ExportCSV({ accountId }: { accountId: string }) {
  const months = Array.from({ length: 12 }, (_, i) => {
    const d = new Date()
    d.setDate(1)
    d.setMonth(d.getMonth() - i)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  })
  const [month, setMonth] = useState(months[0])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const download = async () => {
    setBusy(true)
    setError('')
    try {
      const [y, m] = month.split('-').map(Number)
      const last = new Date(y, m, 0).getDate()
      const csv = await apiText(`/v1/accounts/${accountId}/statement.csv?from=${month}-01&to=${month}-${last}`)
      const link = document.createElement('a')
      link.href = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }))
      link.download = `extrato-${month}.csv`
      link.click()
      URL.revokeObjectURL(link.href)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex items-end gap-2">
      <label className="text-xs text-muted">
        <span className="sr-only">Mês do extrato</span>
        <select value={month} onChange={(e) => setMonth(e.target.value)} aria-label="Mês do extrato"
          className="rounded-full bg-surface-2 px-4 py-2.5 text-sm font-medium text-fg">
          {months.map((m) => <option key={m} value={m}>{monthLabel(m)}</option>)}
        </select>
      </label>
      <Button variant="secondary" size="sm" icon={Download} loading={busy} onClick={() => void download()}>Exportar CSV</Button>
      {error && <span className="text-xs text-danger">{error}</span>}
    </div>
  )
}

function dayLabel(iso: string) {
  const d = new Date(iso)
  const today = new Date()
  const yesterday = new Date(Date.now() - 86_400_000)
  if (d.toDateString() === today.toDateString()) return 'Hoje'
  if (d.toDateString() === yesterday.toDateString()) return 'Ontem'
  return d.toLocaleDateString('pt-BR', { weekday: 'long', day: '2-digit', month: 'long' })
}

export function StatementPage() {
  const account = useAccount()
  const feed = useStatement(account?.id, 25)
  const items = feed.data?.pages.flatMap((p) => p.items) ?? []
  const groups = items.reduce<[string, FeedItem[]][]>((acc, item) => {
    const label = dayLabel(item.occurred_at)
    const last = acc.at(-1)
    if (last && last[0] === label) last[1].push(item)
    else acc.push([label, [item]])
    return acc
  }, [])
  return (
    <div>
      <div data-reveal className="mb-6 flex flex-wrap items-end justify-between gap-3">
        <h1 className="text-[34px] font-bold tracking-[-0.02em] text-fg">Extrato</h1>
        {account && <ExportCSV accountId={account.id} />}
      </div>
      <Panel>
        {feed.isLoading ? <Loading rows={6} /> : items.length === 0 ? <Empty icon={ArrowLeftRight}>Nenhuma movimentação ainda.</Empty> : (
          <>
            {groups.map(([label, list]) => (
              <div key={label} className="mb-2">
                <p className={cx('sticky top-0 z-10 -mx-2 bg-surface px-2 py-2 text-xs font-semibold tracking-wide text-muted capitalize')}>{label}</p>
                <ul className="divide-y divide-line">{list.map((i) => <FeedRow key={i.id} item={i} />)}</ul>
              </div>
            ))}
            {feed.hasNextPage && (
              <div className="mt-4 text-center">
                <Button variant="secondary" loading={feed.isFetchingNextPage} onClick={() => feed.fetchNextPage()}>Carregar mais</Button>
              </div>
            )}
          </>
        )}
      </Panel>
    </div>
  )
}
