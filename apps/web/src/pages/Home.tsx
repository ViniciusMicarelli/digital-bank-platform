import { useState } from 'react'
import { Link } from 'react-router'
import { useAccount, useBalance, useStatement } from '../app/queries'
import { Amount, Button, Empty, Loading, Panel } from '../components/ui'
import { dateTimeBR, typeLabels } from '../lib/format'
import { formatBRL } from '@bank/core'
import type { FeedItem } from '@bank/core'


export function FeedRow({ item }: { item: FeedItem }) {
  return (
    <li className="flex items-center gap-3 py-3">
      <span className={`grid size-9 shrink-0 place-items-center rounded-full text-sm ${item.direction === 'CREDIT' ? 'bg-emerald-50 text-emerald-700' : 'bg-slate-100 text-slate-600'}`}>
        {item.direction === 'CREDIT' ? '↓' : '↑'}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{item.counterparty_name || item.description || typeLabels[item.type] || item.type}</p>
        <p className="text-xs text-slate-500">{typeLabels[item.type] ?? item.type} · {dateTimeBR(item.occurred_at)}</p>
      </div>
      <Amount value={item.amount} direction={item.direction} className="text-sm" />
    </li>
  )
}


export function HomePage() {
  const account = useAccount()
  const balance = useBalance(account?.id)
  const recent = useStatement(account?.id, 6)
  const [hidden, setHidden] = useState(false)
  const items = recent.data?.pages.flatMap((p) => p.items) ?? []

  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm text-slate-500">Olá,</p>
        <h1 className="text-2xl font-bold tracking-tight">{account?.holder_name.split(' ')[0]}</h1>
      </div>

      <section className="rounded-3xl bg-gradient-to-br from-brand-600 to-brand-900 p-6 text-white shadow-lg">
        <div className="flex items-center justify-between">
          <p className="text-sm text-brand-100">Saldo disponível</p>
          <button onClick={() => setHidden(!hidden)} className="rounded-lg px-2 py-1 text-xs text-brand-100 hover:bg-white/10">
            {hidden ? 'Mostrar' : 'Ocultar'}
          </button>
        </div>
        <p className="tabular mt-2 text-4xl font-bold tracking-tight">
          {balance.isLoading ? '…' : hidden ? 'R$ •••••' : formatBRL(balance.data?.available)}
        </p>
        {!hidden && balance.data && balance.data.pending.amount !== '0.00' && (
          <p className="mt-1 text-sm text-brand-100">{formatBRL(balance.data.pending)} reservado em compras e Pix em andamento</p>
        )}
        <p className="mt-6 text-xs text-brand-100">Ag. {account?.branch} · Conta {account?.number}</p>
      </section>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {[
          ['/pix?aba=enviar', 'Enviar Pix', '◇'],
          ['/pix?aba=cobrar', 'Cobrar', '▦'],
          ['/boletos', 'Pagar boleto', '▥'],
          ['/cartoes', 'Cartões', '▭'],
        ].map(([to, label, icon]) => (
          <Link key={to} to={to} className="flex flex-col items-start gap-3 rounded-2xl bg-white p-4 shadow-sm ring-1 ring-slate-100 transition hover:ring-brand-200">
            <span className="grid size-9 place-items-center rounded-xl bg-brand-50 text-brand-700">{icon}</span>
            <span className="text-sm font-semibold">{label}</span>
          </Link>
        ))}
      </div>

      <Panel title="Últimas movimentações" action={<Link to="/extrato" className="text-sm font-semibold text-brand-700">Ver extrato</Link>}>
        {recent.isLoading ? <Loading /> : items.length === 0 ? (
          <div className="text-center">
            <Empty>Nenhuma movimentação ainda.</Empty>
            <Link to="/pix?aba=chaves"><Button variant="secondary">Cadastrar chave Pix e receber</Button></Link>
          </div>
        ) : (
          <ul className="divide-y divide-slate-100">{items.map((i) => <FeedRow key={i.id} item={i} />)}</ul>
        )}
      </Panel>
    </div>
  )
}

export function StatementPage() {
  const account = useAccount()
  const feed = useStatement(account?.id, 25)
  const items = feed.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold tracking-tight">Extrato</h1>
      <Panel>
        {feed.isLoading ? <Loading /> : items.length === 0 ? <Empty>Nenhuma movimentação ainda.</Empty> : (
          <>
            <ul className="divide-y divide-slate-100">{items.map((i) => <FeedRow key={i.id} item={i} />)}</ul>
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
