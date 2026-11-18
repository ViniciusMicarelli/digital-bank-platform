import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useAccount } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import { useStepUp } from '../components/StepUp'
import { Eye, EyeOff, Globe, Lock, LockOpen, Nfc, Pause, Play, Plus, ShoppingCart, Smartphone, Trash2, Wifi } from 'lucide-react'
import { BankCard } from '../components/BankCard'
import { Alert, Badge, Button, Empty, Field, IconButton, IconTile, Loading, Modal, PageTitle, Panel, Sandbox, Select, Toggle } from '../components/ui'
import { cx } from '../lib/format'
import { api } from '../lib/api'
import { errorMessage, formatBRL, type Card, type CardDetails, type CardToken } from '@bank/core'

const wallets: Record<string, string> = { APPLE_PAY: 'Apple Pay', GOOGLE_PAY: 'Google Pay', SAMSUNG_PAY: 'Samsung Pay', MERCHANT: 'Loja' }

function Wallets({ accountId, card, confirm }: { accountId: string; card: Card; confirm: () => Promise<string> }) {
  const qc = useQueryClient()
  const base = `/v1/accounts/${accountId}/cards/${card.id}/tokens`
  const [open, setOpen] = useState(false)
  const [wallet, setWallet] = useState('APPLE_PAY')
  const [device, setDevice] = useState('')
  const [created, setCreated] = useState<CardToken | null>(null)
  const [error, setError] = useState('')
  const tokens = useQuery({ queryKey: ['cards', card.id, 'tokens'], queryFn: async () => (await api<{ tokens: CardToken[] }>('GET', base)).tokens })
  const refresh = () => qc.invalidateQueries({ queryKey: ['cards', card.id, 'tokens'] })
  const add = useMutation({
    mutationFn: async () => api<CardToken>('POST', base, { body: { wallet, device_name: device }, stepUp: await confirm() }),
    onSuccess: (t) => { setCreated(t); refresh() },
    onError: (e) => setError(e instanceof Error && e.message === 'cancelled' ? '' : errorMessage(e)),
  })
  const act = useMutation({
    mutationFn: async ({ t, action }: { t: CardToken; action: 'suspend' | 'resume' | 'delete' }) =>
      action === 'delete' ? api('DELETE', `${base}/${t.id}`)
        : api('POST', `${base}/${t.id}/${action}`, action === 'resume' ? { stepUp: await confirm() } : {}),
    onSuccess: refresh,
  })
  return (
    <Panel title="Carteiras digitais" icon={Smartphone} action={<Button variant="ghost" size="sm" icon={Plus} onClick={() => { setOpen(true); setCreated(null); setError('') }}>Adicionar</Button>}>
      {!tokens.data?.length ? <Empty icon={Smartphone}>Use o cartão no celular sem mostrar o número dele.</Empty> : (
        <ul className="-my-1 divide-y divide-line">
          {tokens.data.map((t) => (
            <li key={t.id} className="flex items-center gap-2 py-3">
              <IconTile icon={Smartphone} tone={t.status === 'ACTIVE' ? 'brand' : 'amber'} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-semibold text-fg">{wallets[t.wallet] ?? t.wallet} · {t.device_name}</p>
                <p className="text-xs text-muted">Token final {t.last4}</p>
              </div>
              <Badge tone={t.status === 'ACTIVE' ? 'green' : 'amber'}>{t.status === 'ACTIVE' ? 'Ativo' : 'Suspenso'}</Badge>
              <IconButton icon={t.status === 'ACTIVE' ? Pause : Play} label={t.status === 'ACTIVE' ? 'Suspender' : 'Reativar'}
                onClick={() => act.mutate({ t, action: t.status === 'ACTIVE' ? 'suspend' : 'resume' })} />
              <IconButton icon={Trash2} label="Remover" className="hover:text-danger" onClick={() => act.mutate({ t, action: 'delete' })} />
            </li>
          ))}
        </ul>
      )}
      <Modal open={open} onClose={() => setOpen(false)} title="Adicionar à carteira">
        {created ? (
          <div className="space-y-3 text-sm">
            <Alert tone="green">Cartão adicionado ao {wallets[created.wallet]}.</Alert>
            <p className="text-fg-2">Número do token (só a carteira recebe; mostrado aqui por ser o ambiente de testes):</p>
            <p className="font-mono text-base">{created.token_pan}</p>
          </div>
        ) : (
          <form onSubmit={(e) => { e.preventDefault(); add.mutate() }} className="space-y-4">
            <Select label="Carteira" value={wallet} onChange={(e) => setWallet(e.target.value)}>
              {Object.entries(wallets).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </Select>
            <Field label="Aparelho" required placeholder="iPhone da Ana" value={device} onChange={(e) => setDevice(e.target.value)} />
            <Alert>{error}</Alert>
            <Button type="submit" loading={add.isPending}>Adicionar</Button>
          </form>
        )}
      </Modal>
    </Panel>
  )
}

function SimulatePurchase({ card, reveal }: { card: Card; reveal: () => Promise<CardDetails> }) {
  const qc = useQueryClient()
  const [amount, setAmount] = useState<string | null>(null)
  const [merchant, setMerchant] = useState('PADARIA SAO JOSE')
  const [fn, setFn] = useState('DEBIT')
  const [country, setCountry] = useState('BR')
  const [result, setResult] = useState('')
  const buy = useMutation({
    mutationFn: async () => {
      const d = await reveal()
      const auth = await api<{ id: string; status: string; response_code: string; reason: string }>('POST', '/v1/sandbox/card-network/authorizations', {
        body: { pan: d.pan, expiry_month: d.expiry_month, expiry_year: d.expiry_year, cvv: d.cvv, amount, merchant_name: merchant, mcc: '5462', country, channel: 'ONLINE', function: fn } })
      if (auth.status === 'APPROVED') await api('POST', `/v1/sandbox/card-network/authorizations/${auth.id}/clear`, { body: {} })
      return auth
    },
    onSuccess: (a) => {
      setResult(a.status === 'APPROVED' ? `Aprovada: ${formatBRL(amount!)} em ${merchant}.` : `Recusada (código ${a.response_code}): ${a.reason}.`)
      qc.invalidateQueries({ queryKey: ['balance'] })
    },
    onError: (e) => setResult(e instanceof Error && e.message === 'cancelled' ? '' : errorMessage(e)),
  })
  if (card.status !== 'ACTIVE') return null
  return (
    <Sandbox>
      <form onSubmit={(e) => { e.preventDefault(); buy.mutate() }} className="grid gap-3 sm:grid-cols-2">
        <MoneyField label="Simular compra de" onValue={setAmount} />
        <Field label="Loja" value={merchant} onChange={(e) => setMerchant(e.target.value.toUpperCase())} />
        <Select label="Função" value={fn} onChange={(e) => setFn(e.target.value)}><option value="DEBIT">Débito</option><option value="CREDIT">Crédito</option></Select>
        <Select label="País" value={country} onChange={(e) => setCountry(e.target.value)}><option value="BR">Brasil</option><option value="US">Estados Unidos</option></Select>
        <div className="sm:col-span-2"><Button type="submit" icon={ShoppingCart} loading={buy.isPending} disabled={!amount}>Passar o cartão</Button></div>
      </form>
      {result && <p className="mt-3 text-sm font-medium text-warning">{result}</p>}
    </Sandbox>
  )
}

function CardView({ accountId, card }: { accountId: string; card: Card }) {
  const qc = useQueryClient()
  const { confirm, modal } = useStepUp()
  const [details, setDetails] = useState<CardDetails | null>(null)
  const [error, setError] = useState('')
  const base = `/v1/accounts/${accountId}/cards/${card.id}`
  const refresh = () => qc.invalidateQueries({ queryKey: ['cards', accountId] })
  const reveal = async () => {
    if (details) return details
    const d = await api<CardDetails>('POST', `${base}/reveal`, { stepUp: await confirm() })
    setDetails(d)
    return d
  }
  const show = useMutation({ mutationFn: reveal, onError: (e) => setError(e instanceof Error && e.message === 'cancelled' ? '' : errorMessage(e)) })
  const block = useMutation({
    mutationFn: () => card.status === 'ACTIVE' ? api('POST', `${base}/block`, { body: { reason: 'Bloqueado pelo cliente' } }) : api('POST', `${base}/unblock`),
    onSuccess: refresh,
  })
  const controls = useMutation({
    mutationFn: (c: Card['controls']) => api('PUT', `${base}/controls`, { body: c }),
    onSuccess: refresh,
  })
  const setControl = (k: keyof Card['controls']) => (v: boolean) => controls.mutate({ ...card.controls, [k]: v })
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <div className="space-y-5">
        <div data-reveal className="mx-auto max-w-md">
          <BankCard holder={card.holder_name} last4={card.last4}
            expiry={`${String(card.expiry_month).padStart(2, '0')}/${String(card.expiry_year).slice(2)}`}
            skin={card.status === 'BLOCKED' ? 'blocked' : 'debit'} label={card.kind === 'VIRTUAL' ? 'Virtual' : 'Físico'}
            revealed={details} />
        </div>
        <div data-reveal className="grid grid-cols-2 gap-3">
          {details ? <Button variant="secondary" icon={EyeOff} onClick={() => setDetails(null)}>Ocultar dados</Button>
            : <Button variant="secondary" icon={Eye} loading={show.isPending} onClick={() => show.mutate()} disabled={card.status !== 'ACTIVE'}>Ver dados do cartão</Button>}
          <Button variant={card.status === 'ACTIVE' ? 'danger' : 'primary'} icon={card.status === 'ACTIVE' ? Lock : LockOpen} loading={block.isPending} onClick={() => block.mutate()}>
            {card.status === 'ACTIVE' ? 'Bloquear' : 'Desbloquear'}
          </Button>
        </div>
        {card.status === 'BLOCKED' && <Alert tone="amber">Cartão bloqueado: compras são recusadas até você desbloquear.</Alert>}
        <Alert>{error}</Alert>
        <SimulatePurchase card={card} reveal={reveal} />
      </div>
      <div className="space-y-6">
        <Panel title="Controles" icon={Lock}>
          <div className="-my-2 divide-y divide-line">
            <Toggle label="Compras online" description="Sites e aplicativos" checked={card.controls.online} onChange={setControl('online')} />
            <Toggle label="Compras internacionais" description="Fora do Brasil" checked={card.controls.international} onChange={setControl('international')} />
            <Toggle label="Pagamento por aproximação" description="Encostar o cartão na maquininha" checked={card.controls.contactless} onChange={setControl('contactless')} />
          </div>
        </Panel>
        <Wallets accountId={accountId} card={card} confirm={confirm} />
      </div>
      {modal}
    </div>
  )
}

export function CardsPage() {
  const account = useAccount()
  const qc = useQueryClient()
  const [selected, setSelected] = useState<string | null>(null)
  const cards = useQuery({
    queryKey: ['cards', account?.id],
    enabled: !!account,
    queryFn: async () => (await api<{ cards: Card[] }>('GET', `/v1/accounts/${account!.id}/cards`)).cards,
  })
  const issue = useMutation({
    mutationFn: (kind: 'VIRTUAL' | 'PHYSICAL') => api<Card>('POST', `/v1/accounts/${account!.id}/cards`, { body: { kind }, idempotent: true }),
    onSuccess: (c) => { setSelected(c.id); qc.invalidateQueries({ queryKey: ['cards', account?.id] }) },
  })
  if (!account || cards.isLoading) return <Loading />
  const list = cards.data ?? []
  const card = list.find((c) => c.id === selected) ?? list[0]
  return (
    <div className="space-y-6">
      <PageTitle subtitle="Débito na conta e, com o crédito ativo, também no crédito.">Cartões</PageTitle>
      {list.length === 0 ? (
        <Panel>
          <Empty icon={Wifi}>Você ainda não tem cartão. O virtual fica pronto na hora.</Empty>
          <div className="flex flex-wrap justify-center gap-2 pb-2">
            <Button icon={Plus} loading={issue.isPending} onClick={() => issue.mutate('VIRTUAL')}>Criar cartão virtual</Button>
            <Button variant="secondary" icon={Nfc} loading={issue.isPending} onClick={() => issue.mutate('PHYSICAL')}>Pedir cartão físico</Button>
          </div>
        </Panel>
      ) : (
        <>
          <div data-reveal className="flex flex-wrap items-center gap-2">
            {list.map((c) => (
              <button key={c.id} onClick={() => setSelected(c.id)}
                className={cx('flex items-center gap-2 rounded-xl px-3.5 py-2 text-sm font-semibold ring-1 transition',
                  c.id === card.id ? 'bg-fg text-bg ring-fg' : 'bg-surface text-fg-2 ring-line hover:ring-line-strong')}>
                {c.kind === 'VIRTUAL' ? <Globe className="size-4" /> : <Nfc className="size-4" />}
                {c.kind === 'VIRTUAL' ? 'Virtual' : 'Físico'} •••• {c.last4}
              </button>
            ))}
            <Button variant="ghost" size="sm" icon={Plus} loading={issue.isPending} onClick={() => issue.mutate('VIRTUAL')}>Novo virtual</Button>
          </div>
          <CardView key={card.id} accountId={account.id} card={card} />
        </>
      )}
    </div>
  )
}
