import { addBRL, ApiError, formatBRL, type Card, type CreditLine, type FeedItem } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowDownLeft, ArrowLeftRight, ArrowUpRight, Barcode, CreditCard, Eye, EyeOff, FileText, QrCode, ShoppingBag, Undo2, Wallet, type LucideIcon,
} from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { useAccount, useBalance, useStatement } from '../app/queries'
import { BankCard } from '../components/BankCard'
import { Amount, Button, Empty, IconTile, Loading, Panel } from '../components/ui'
import { api } from '../lib/api'
import { cx, dateTimeBR, typeLabels } from '../lib/format'
import { useCountUp } from '../lib/motion'

const typeIcons: Record<string, LucideIcon> = {
  PIX: ArrowLeftRight, PIX_OUT: ArrowUpRight, PIX_IN: ArrowDownLeft, PIX_REFUND: Undo2, INTERNAL_TRANSFER: ArrowLeftRight,
  DEBIT_CARD_PURCHASE: ShoppingBag, CREDIT_INVOICE_PAYMENT: Wallet, BOLETO_PAYMENT: FileText,
}

export function FeedRow({ item }: { item: FeedItem }) {
  const credit = item.direction === 'CREDIT'
  return (
    <li className="flex items-center gap-3.5 py-3">
      <IconTile icon={credit ? ArrowDownLeft : (typeIcons[item.type] ?? ArrowUpRight)} tone={credit ? 'green' : 'slate'} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-semibold text-fg">{item.counterparty_name || item.description || typeLabels[item.type] || item.type}</p>
        <p className="text-xs text-muted">{typeLabels[item.type] ?? item.type} · {dateTimeBR(item.occurred_at)}</p>
      </div>
      <Amount value={item.amount} direction={item.direction} className="text-sm" />
    </li>
  )
}

const cents = (amount?: string) => (amount ? Number(amount) : 0)

function Balance({ accountLabel }: { accountLabel: string }) {
  const account = useAccount()
  const balance = useBalance(account?.id)
  const feed = useStatement(account?.id, 50)
  const [hidden, setHidden] = useState(false)
  const available = balance.data?.available.amount
  const ref = useCountUp(cents(available), (n) => formatBRL(n.toFixed(2)))

  // This month's money in and out, from the statement.
  const [month] = useState(() => new Date().toISOString().slice(0, 7))
  const items = (feed.data?.pages.flatMap((p) => p.items) ?? []).filter((i) => i.occurred_at.startsWith(month))
  const sum = (d: FeedItem['direction']) => addBRL('0.00', ...items.filter((i) => i.direction === d).map((i) => i.amount.amount))

  return (
    <section data-reveal className="hero-surface relative overflow-hidden rounded-[28px] p-6 text-white shadow-lift sm:p-8">
      <div aria-hidden className="pointer-events-none absolute -top-24 -right-16 size-72 rounded-full bg-[#2ed3a0]/20 blur-3xl" />
      <div className="relative flex items-center justify-between">
        <p className="text-sm font-medium text-white/70">Saldo disponível</p>
        <button onClick={() => setHidden(!hidden)} aria-label={hidden ? 'Mostrar saldo' : 'Ocultar saldo'}
          className="grid size-9 place-items-center rounded-xl bg-white/10 text-white/80 transition hover:bg-white/20">
          {hidden ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
        </button>
      </div>
      <p className="tabular relative mt-2 text-[40px] leading-none font-bold tracking-tight sm:text-5xl">
        {balance.isLoading ? <span className="inline-block h-11 w-56 animate-pulse rounded-xl bg-white/10" /> : hidden ? 'R$ ••••••' : <span ref={ref}>{formatBRL(available)}</span>}
      </p>
      {!hidden && balance.data && balance.data.pending.amount !== '0.00' && (
        <p className="relative mt-2 text-sm text-white/60">{formatBRL(balance.data.pending)} reservado em compras e Pix em andamento</p>
      )}
      <div className="relative mt-7 grid grid-cols-2 gap-3 sm:max-w-md">
        <div className="rounded-2xl bg-white/[0.07] p-3.5 ring-1 ring-white/10 backdrop-blur">
          <p className="flex items-center gap-1.5 text-xs text-white/60"><ArrowDownLeft className="size-3.5 text-[#2ed3a0]" /> Entradas no mês</p>
          <p className="tabular mt-1 font-semibold">{hidden ? '••••' : formatBRL(sum('CREDIT'))}</p>
        </div>
        <div className="rounded-2xl bg-white/[0.07] p-3.5 ring-1 ring-white/10 backdrop-blur">
          <p className="flex items-center gap-1.5 text-xs text-white/60"><ArrowUpRight className="size-3.5" /> Saídas no mês</p>
          <p className="tabular mt-1 font-semibold">{hidden ? '••••' : formatBRL(sum('DEBIT'))}</p>
        </div>
      </div>
      <p className="relative mt-6 text-xs text-white/50">{accountLabel}</p>
    </section>
  )
}

const actions: [string, string, LucideIcon][] = [
  ['/pix?aba=enviar', 'Enviar Pix', ArrowUpRight],
  ['/pix?aba=cobrar', 'Cobrar', QrCode],
  ['/boletos', 'Pagar boleto', Barcode],
  ['/cartoes', 'Cartões', CreditCard],
]

function Products({ accountId }: { accountId: string }) {
  const cards = useQuery({ queryKey: ['cards', accountId], queryFn: async () => (await api<{ cards: Card[] }>('GET', `/v1/accounts/${accountId}/cards`)).cards })
  const credit = useQuery({
    queryKey: ['credit', accountId],
    queryFn: async () => {
      try {
        return await api<CreditLine>('GET', `/v1/accounts/${accountId}/credit`)
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null
        throw e
      }
    },
  })
  const card = cards.data?.[0]
  const line = credit.data
  const pct = line ? Math.min(100, (Number(line.used.amount) / Number(line.limit.amount)) * 100) : 0
  return (
    <div className="space-y-6">
      <Panel title="Seu cartão" icon={CreditCard} action={<Link to="/cartoes" className="text-sm font-semibold text-brand-strong">Gerenciar</Link>}>
        {card ? (
          <Link to="/cartoes" className="block">
            <BankCard holder={card.holder_name} last4={card.last4} expiry={`${String(card.expiry_month).padStart(2, '0')}/${String(card.expiry_year).slice(2)}`}
              skin={card.status === 'BLOCKED' ? 'blocked' : line ? 'credit' : 'debit'} label={line ? 'Débito e crédito' : 'Débito'} />
          </Link>
        ) : (
          <Empty icon={CreditCard}>Crie um cartão virtual para comprar online agora mesmo.</Empty>
        )}
      </Panel>
      <Panel title="Crédito" icon={Wallet}>
        {line ? (
          <div>
            <p className="text-xs text-muted">Limite disponível</p>
            <p className="tabular mt-0.5 text-2xl font-bold text-fg">{formatBRL(line.available)}</p>
            <div className="mt-3 h-2 overflow-hidden rounded-full bg-surface-3">
              <div className="h-full rounded-full bg-gradient-to-r from-credit to-brand transition-all duration-700" style={{ width: `${pct}%` }} />
            </div>
            <p className="mt-2 text-xs text-muted">Usado {formatBRL(line.used)} de {formatBRL(line.limit)}</p>
          </div>
        ) : (
          <div className="text-sm text-fg-2">
            <p>Ative o crédito e use o mesmo cartão, com fatura mensal.</p>
            <Link to="/credito"><Button variant="credit" size="sm" className="mt-3">Ativar crédito</Button></Link>
          </div>
        )}
      </Panel>
    </div>
  )
}

export function HomePage() {
  const account = useAccount()
  const recent = useStatement(account?.id, 50)
  const items = (recent.data?.pages.flatMap((p) => p.items) ?? []).slice(0, 6)
  const [hour] = useState(() => new Date().getHours())
  const greeting = hour < 12 ? 'Bom dia' : hour < 18 ? 'Boa tarde' : 'Boa noite'

  return (
    <div className="space-y-6">
      <div data-reveal>
        <p className="text-sm text-muted">{greeting},</p>
        <h1 className="text-[28px] font-bold tracking-tight text-fg">{account?.holder_name.split(' ')[0]}</h1>
      </div>
      <div className="grid gap-6 xl:grid-cols-[1fr_340px]">
        <div className="min-w-0 space-y-6">
          <Balance accountLabel={`Agência ${account?.branch} · Conta ${account?.number}`} />
          <div data-reveal className="grid grid-cols-4 gap-2 sm:gap-3">
            {actions.map(([to, label, Icon]) => (
              <Link key={to} to={to}
                className="group flex flex-col items-center gap-2.5 rounded-3xl bg-surface p-3 text-center shadow-card ring-1 ring-line transition hover:-translate-y-0.5 hover:shadow-lift sm:items-start sm:p-4 sm:text-left">
                <span className="grid size-11 place-items-center rounded-2xl bg-brand-soft text-brand-strong transition group-hover:scale-110 group-hover:bg-brand group-hover:text-brand-fg">
                  <Icon className="size-5" strokeWidth={2.2} />
                </span>
                <span className="text-xs font-semibold text-fg sm:text-sm">{label}</span>
              </Link>
            ))}
          </div>
          <Panel title="Últimas movimentações" action={<Link to="/extrato" className="text-sm font-semibold text-brand-strong">Ver extrato</Link>}>
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
        {account && <Products accountId={account.id} />}
      </div>
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
      <h1 data-reveal className="mb-6 text-[28px] font-bold tracking-tight text-fg">Extrato</h1>
      <Panel>
        {feed.isLoading ? <Loading rows={6} /> : items.length === 0 ? <Empty icon={ArrowLeftRight}>Nenhuma movimentação ainda.</Empty> : (
          <>
            {groups.map(([label, list]) => (
              <div key={label} className="mb-2">
                <p className={cx('sticky top-0 z-10 -mx-2 bg-surface/90 px-2 py-2 text-xs font-semibold tracking-wide text-muted capitalize backdrop-blur')}>{label}</p>
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
