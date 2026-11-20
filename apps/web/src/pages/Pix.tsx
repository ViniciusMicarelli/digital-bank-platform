import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import QRCode from 'qrcode'
import { Check, Copy, IdCard, KeyRound, Mail, QrCode, Smartphone, Star, Trash2, type LucideIcon } from 'lucide-react'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { gsap, reducedMotion, useGSAP } from '../lib/motion'
import { useSearchParams } from 'react-router'
import { useAccount } from '../app/queries'
import { MoneyField } from '../components/MoneyField'
import { Alert, Amount, Badge, Button, Empty, Field, IconTile, Loading, Modal, PageTitle, Panel, Sandbox, Select, Tabs, Toggle } from '../components/ui'
import { dateTimeBR } from '../lib/format'
import { api, newIdempotencyKey } from '../lib/api'
import { errorMessage, formatBRL, type Money, type Party, type PixContact, type PixKey, type PixTransfer } from '@bank/core'

type Tab = 'enviar' | 'cobrar' | 'pagar' | 'chaves' | 'historico'

function usePixKeys(accountId?: string) {
  return useQuery({
    queryKey: ['pix', accountId, 'keys'],
    enabled: !!accountId,
    queryFn: async () => (await api<{ keys: PixKey[] }>('GET', `/v1/accounts/${accountId}/pix/keys`)).keys,
  })
}

const keyIcons: Record<string, LucideIcon> = { EMAIL: Mail, PHONE: Smartphone, CPF: IdCard, EVP: KeyRound }

function Avatar({ name }: { name: string }) {
  const p = name.split(' ').filter(Boolean)
  return <span className="grid size-12 shrink-0 place-items-center rounded-full bg-brand-soft text-sm font-bold text-brand-strong">{(p[0]?.[0] ?? '') + (p.at(-1)?.[0] ?? '')}</span>
}

function Receipt({ t, title }: { t: PixTransfer; title: string }) {
  const root = useRef<HTMLDivElement>(null)
  useGSAP(() => {
    if (reducedMotion()) return
    gsap.timeline()
      .from('[data-check]', { scale: 0, rotate: -90, duration: 0.6, ease: 'back.out(2)' })
      .from('[data-line]', { y: 10, autoAlpha: 0, stagger: 0.07, duration: 0.4, ease: 'power2.out' }, '-=0.2')
  }, { scope: root })
  return (
    <div ref={root} className="space-y-2 py-4 text-center">
      <div data-check className="mx-auto mb-4 grid size-16 place-items-center rounded-full bg-brand text-brand-fg shadow-[0_10px_30px_-8px_var(--brand)]"><Check className="size-8" strokeWidth={3} /></div>
      <p data-line className="font-semibold text-fg">{title}</p>
      <p data-line className="tabular text-4xl font-bold tracking-tight text-fg">{formatBRL(t.amount)}</p>
      <p data-line className="text-sm text-fg-2">{t.counterparty.name} · {t.counterparty.bank}</p>
      {t.status !== 'COMPLETED' && <Badge tone="amber">Em processamento</Badge>}
    </div>
  )
}

function useContacts(accountId: string) {
  return useQuery({
    queryKey: ['pix', accountId, 'contacts'],
    queryFn: async () => (await api<{ contacts: PixContact[] }>('GET', `/v1/accounts/${accountId}/pix/contacts`)).contacts,
  })
}

/** Favorites and recent receivers, one tap to send again. */
function Contacts({ accountId, onPick }: { accountId: string; onPick: (key: string) => void }) {
  const contacts = useContacts(accountId)
  if (!contacts.data?.length) return null
  return (
    <div className="mb-5">
      <p className="mb-2 text-xs font-medium text-muted">Contatos</p>
      <ul className="no-scrollbar -mx-1 flex gap-3 overflow-x-auto px-1 pb-1">
        {contacts.data.map((c) => (
          <li key={c.key}>
            <button type="button" onClick={() => onPick(c.key)} title={c.key}
              className="flex w-20 flex-col items-center gap-1.5 rounded-2xl py-2 text-center transition hover:bg-surface-2">
              <span className="relative">
                <Avatar name={c.nickname || c.name || c.key} />
                {c.favorite && <Star className="absolute -right-1 -bottom-1 size-4 fill-warning text-warning" />}
              </span>
              <span className="w-full truncate text-xs font-medium text-fg">{c.nickname || c.name.split(' ')[0] || c.key}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}

function FavoriteToggle({ accountId, pixKey }: { accountId: string; pixKey: string }) {
  const qc = useQueryClient()
  const contacts = useContacts(accountId)
  const [naming, setNaming] = useState(false)
  const [nickname, setNickname] = useState('')
  const norm = (k: string) => k.toLowerCase().replace(/[^a-z0-9@.]/g, '')
  const favorite = contacts.data?.find((c) => norm(c.key) === norm(pixKey))?.favorite
  const refresh = () => qc.invalidateQueries({ queryKey: ['pix', accountId, 'contacts'] })
  const save = useMutation({
    mutationFn: () => api('PUT', `/v1/accounts/${accountId}/pix/contacts/favorites`, { body: { key: pixKey, nickname } }),
    onSuccess: () => { setNaming(false); refresh() },
  })
  const remove = useMutation({
    mutationFn: () => api('DELETE', `/v1/accounts/${accountId}/pix/contacts/favorites?key=${encodeURIComponent(pixKey)}`),
    onSuccess: refresh,
  })
  if (favorite) return <Button type="button" variant="ghost" size="sm" icon={Star} loading={remove.isPending} onClick={() => remove.mutate()}>Remover dos favoritos</Button>
  if (!naming) return <Button type="button" variant="ghost" size="sm" icon={Star} onClick={() => setNaming(true)}>Favoritar</Button>
  return (
    <div className="flex items-end gap-2">
      <div className="flex-1"><Field label="Apelido (opcional)" maxLength={30} value={nickname} onChange={(e) => setNickname(e.target.value)} /></div>
      <Button type="button" size="sm" loading={save.isPending} onClick={() => save.mutate()}>Salvar</Button>
    </div>
  )
}

function SendPix({ accountId }: { accountId: string }) {
  const qc = useQueryClient()
  const [key, setKey] = useState('')
  const [amount, setAmount] = useState<string | null>(null)
  const [description, setDescription] = useState('')
  const [owner, setOwner] = useState<Party | null>(null)
  const [done, setDone] = useState<PixTransfer | null>(null)
  const [error, setError] = useState('')
  const [idem, setIdem] = useState(newIdempotencyKey)

  const lookup = useMutation({
    mutationFn: (k: string = key) => api<{ owner: Party }>('GET', `/v1/pix/keys/${encodeURIComponent(k.trim())}`),
    onSuccess: (r) => { setOwner(r.owner); setError('') },
    onError: (e) => setError(errorMessage(e)),
  })
  const send = useMutation({
    mutationFn: () => api<PixTransfer>('POST', `/v1/accounts/${accountId}/pix/transfers`, { body: { key: key.trim(), amount, description }, idempotent: idem }),
    onSuccess: (t) => { setDone(t); for (const k of ['balance', 'statement', 'pix']) qc.invalidateQueries({ queryKey: [k] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  const reset = () => { setKey(''); setOwner(null); setDone(null); setError(''); setIdem(newIdempotencyKey()) }

  if (done) return <Panel><Receipt t={done} title="Pix enviado" /><Button className="mt-6 w-full" variant="secondary" onClick={reset}>Novo Pix</Button></Panel>

  return (
    <Panel title="Enviar Pix">
      {!owner ? (
        <form onSubmit={(e) => { e.preventDefault(); lookup.mutate(key) }} className="space-y-4">
          <Contacts accountId={accountId} onPick={(k) => { setKey(k); lookup.mutate(k) }} />
          <Field label="Chave Pix" placeholder="CPF, e-mail, celular ou chave aleatória" required value={key} onChange={(e) => setKey(e.target.value)} />
          <Alert>{error}</Alert>
          <Button type="submit" loading={lookup.isPending}>Continuar</Button>
        </form>
      ) : (
        <form onSubmit={(e) => { e.preventDefault(); send.mutate() }} className="space-y-4">
          <div className="flex items-center gap-3 rounded-2xl bg-surface-2 p-4 text-sm">
            <Avatar name={owner.name} />
            <div className="min-w-0"><p className="truncate font-semibold text-fg">{owner.name}</p><p className="text-muted">{owner.document} · {owner.bank}</p></div>
          </div>
          <FavoriteToggle accountId={accountId} pixKey={key.trim()} />
          <MoneyField label="Valor" onValue={setAmount} />
          <Field label="Mensagem (opcional)" maxLength={140} value={description} onChange={(e) => setDescription(e.target.value)} />
          <Alert>{error}</Alert>
          <div className="flex gap-2">
            <Button type="submit" loading={send.isPending} disabled={!amount}>Enviar {amount ? formatBRL(amount) : ''}</Button>
            <Button type="button" variant="ghost" onClick={reset}>Voltar</Button>
          </div>
        </form>
      )}
    </Panel>
  )
}

function ChargePix({ accountId, keys }: { accountId: string; keys: PixKey[] }) {
  const [key, setKey] = useState(keys[0]?.value ?? '')
  const [amount, setAmount] = useState<string | null>(null)
  const [oneTime, setOneTime] = useState(true)
  const [qr, setQr] = useState<{ payload: string; image: string } | null>(null)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)
  const create = useMutation({
    mutationFn: () => api<{ payload: string }>('POST', `/v1/accounts/${accountId}/pix/qr-codes`, {
      body: { key, amount: amount ?? undefined, description: 'Cobrança', one_time: oneTime, expires_in_seconds: oneTime ? 3600 : 0 } }),
    onSuccess: async (r) => setQr({ payload: r.payload, image: await QRCode.toDataURL(r.payload, { margin: 1, width: 240 }) }),
    onError: (e) => setError(errorMessage(e)),
  })
  if (keys.length === 0) return <Panel><Empty>Cadastre uma chave Pix para cobrar.</Empty></Panel>
  return (
    <Panel title="Cobrar com QR Code">
      {qr ? (
        <div className="space-y-4 text-center">
          <div className="mx-auto w-fit rounded-3xl bg-white p-4 shadow-card ring-1 ring-line">
            <img src={qr.image} alt="QR Code Pix" className="size-56" />
          </div>
          <p className="text-sm text-fg-2">{amount ? formatBRL(amount) : 'Valor livre'} · {oneTime ? 'uso único, vale 1 hora' : 'pode ser pago várias vezes'}</p>
          <div className="flex justify-center gap-2">
            <Button variant="secondary" icon={copied ? Check : Copy} onClick={async () => { await navigator.clipboard.writeText(qr.payload); setCopied(true) }}>
              {copied ? 'Copiado!' : 'Copiar Pix copia e cola'}
            </Button>
            <Button variant="ghost" onClick={() => { setQr(null); setCopied(false) }}>Nova cobrança</Button>
          </div>
        </div>
      ) : (
        <form onSubmit={(e: FormEvent) => { e.preventDefault(); create.mutate() }} className="space-y-4">
          <Select label="Receber na chave" value={key} onChange={(e) => setKey(e.target.value)}>
            {keys.map((k) => <option key={k.id} value={k.value}>{k.type} · {k.value}</option>)}
          </Select>
          <MoneyField label="Valor (opcional)" optional onValue={setAmount} hint="Sem valor, quem paga escolhe." />
          <Toggle label="Uso único" description="O QR Code só pode ser pago uma vez e expira em 1 hora." checked={oneTime} onChange={setOneTime} />
          <Alert>{error}</Alert>
          <Button type="submit" icon={QrCode} loading={create.isPending}>Gerar QR Code</Button>
        </form>
      )}
    </Panel>
  )
}

interface Decoded { receiver: Party; amount?: Money; description: string; one_time: boolean }

function PayQR({ accountId }: { accountId: string }) {
  const qc = useQueryClient()
  const [payload, setPayload] = useState('')
  const [decoded, setDecoded] = useState<Decoded | null>(null)
  const [amount, setAmount] = useState<string | null>(null)
  const [done, setDone] = useState<PixTransfer | null>(null)
  const [error, setError] = useState('')
  const [idem] = useState(newIdempotencyKey)
  const decode = useMutation({
    mutationFn: () => api<Decoded>('POST', '/v1/pix/qr-codes/decode', { body: { payload: payload.trim() } }),
    onSuccess: (d) => { setDecoded(d); setError('') },
    onError: (e) => setError(errorMessage(e)),
  })
  const pay = useMutation({
    mutationFn: () => api<PixTransfer>('POST', `/v1/accounts/${accountId}/pix/qr-payments`, {
      body: { payload: payload.trim(), amount: decoded?.amount ? undefined : amount }, idempotent: idem }),
    onSuccess: (t) => { setDone(t); qc.invalidateQueries({ queryKey: ['balance'] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  if (done) return <Panel><Receipt t={done} title="QR Code pago" /></Panel>
  return (
    <Panel title="Pagar QR Code">
      {!decoded ? (
        <form onSubmit={(e) => { e.preventDefault(); decode.mutate() }} className="space-y-4">
          <label className="block">
            <span className="mb-1 block text-sm font-medium text-fg-2">Pix copia e cola</span>
            <textarea required rows={4} value={payload} onChange={(e) => setPayload(e.target.value)}
              className="block w-full rounded-xl border-0 bg-surface px-3.5 py-3 font-mono text-xs text-fg ring-1 ring-line-strong focus:ring-2 focus:ring-brand focus:outline-none" />
          </label>
          <Alert>{error}</Alert>
          <Button type="submit" loading={decode.isPending}>Ler código</Button>
        </form>
      ) : (
        <div className="space-y-4">
          <div className="rounded-xl bg-surface-2 p-4 text-sm">
            <p className="font-semibold">{decoded.receiver.name}</p>
            <p className="text-fg-2">{decoded.receiver.document} · {decoded.receiver.bank}</p>
            {decoded.amount && <p className="tabular mt-2 text-2xl font-bold">{formatBRL(decoded.amount)}</p>}
          </div>
          {!decoded.amount && <MoneyField label="Valor" onValue={setAmount} />}
          <Alert>{error}</Alert>
          <Button loading={pay.isPending} disabled={!decoded.amount && !amount} onClick={() => pay.mutate()}>Pagar</Button>
        </div>
      )}
    </Panel>
  )
}

function Keys({ accountId, keys }: { accountId: string; keys: PixKey[] }) {
  const qc = useQueryClient()
  const [type, setType] = useState<PixKey['type']>('EMAIL')
  const [value, setValue] = useState('')
  const [error, setError] = useState('')
  const refresh = () => qc.invalidateQueries({ queryKey: ['pix', accountId, 'keys'] })
  const add = useMutation({
    mutationFn: () => api('POST', `/v1/accounts/${accountId}/pix/keys`, { body: { type, value: type === 'EVP' ? '' : value.trim() } }),
    onSuccess: () => { setValue(''); setError(''); refresh() },
    onError: (e) => setError(errorMessage(e)),
  })
  const remove = useMutation({ mutationFn: (id: string) => api('DELETE', `/v1/accounts/${accountId}/pix/keys/${id}`), onSuccess: refresh })
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Panel title="Minhas chaves">
        {keys.length === 0 ? <Empty>Nenhuma chave cadastrada.</Empty> : (
          <ul className="-my-1 divide-y divide-line">
            {keys.map((k) => (
              <li key={k.id} className="flex items-center gap-3 py-3">
                <IconTile icon={keyIcons[k.type] ?? KeyRound} tone="brand" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-semibold text-fg">{k.value}</p>
                  <p className="text-xs text-muted">{{ EMAIL: 'E-mail', PHONE: 'Celular', CPF: 'CPF', EVP: 'Chave aleatória' }[k.type]}</p>
                </div>
                <Button variant="ghost" size="sm" icon={Trash2} onClick={() => remove.mutate(k.id)}>Excluir</Button>
              </li>
            ))}
          </ul>
        )}
      </Panel>
      <Panel title="Cadastrar chave">
        <form onSubmit={(e) => { e.preventDefault(); add.mutate() }} className="space-y-4">
          <Select label="Tipo" value={type} onChange={(e) => setType(e.target.value as PixKey['type'])}>
            <option value="EMAIL">E-mail</option>
            <option value="PHONE">Celular</option>
            <option value="CPF">CPF</option>
            <option value="EVP">Chave aleatória</option>
          </Select>
          {type !== 'EVP' && (
            <Field label="Chave" required value={value} onChange={(e) => setValue(e.target.value)}
              placeholder={{ EMAIL: 'voce@exemplo.com', PHONE: '+5511912345678', CPF: 'seu CPF' }[type]} />
          )}
          <Alert>{error}</Alert>
          <Button type="submit" loading={add.isPending}>Cadastrar</Button>
        </form>
      </Panel>
    </div>
  )
}

function History({ accountId }: { accountId: string }) {
  const qc = useQueryClient()
  const [direction, setDirection] = useState<'incoming' | 'outgoing'>('incoming')
  const [refunding, setRefunding] = useState<PixTransfer | null>(null)
  const [amount, setAmount] = useState<string | null>(null)
  const [error, setError] = useState('')
  const list = useQuery({
    queryKey: ['pix', accountId, 'transfers', direction],
    queryFn: async () => (await api<{ transfers: PixTransfer[] }>('GET', `/v1/accounts/${accountId}/pix/transfers?direction=${direction}`)).transfers,
  })
  const refund = useMutation({
    mutationFn: () => api('POST', `/v1/accounts/${accountId}/pix/transfers/${refunding!.id}/refunds`, { body: { amount, reason: 'Devolução solicitada' }, idempotent: true }),
    onSuccess: () => { setRefunding(null); qc.invalidateQueries({ queryKey: ['pix'] }); qc.invalidateQueries({ queryKey: ['balance'] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <Panel title="Histórico de Pix" action={
      <Select label="" value={direction} onChange={(e) => setDirection(e.target.value as 'incoming' | 'outgoing')}>
        <option value="incoming">Recebidos</option>
        <option value="outgoing">Enviados</option>
      </Select>}>
      {list.isLoading ? <Loading /> : !list.data?.length ? <Empty>Nada por aqui.</Empty> : (
        <ul className="-my-1 divide-y divide-line">
          {list.data.map((t) => (
            <li key={t.id} className="flex items-center gap-3 py-3">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{t.counterparty.name}</p>
                <p className="text-xs text-muted">{dateTimeBR(t.created_at)} · {t.status === 'COMPLETED' ? 'Concluído' : t.status}
                  {t.refunded && t.refunded.amount !== '0.00' && ` · ${formatBRL(t.refunded)} devolvido`}</p>
              </div>
              <Amount value={t.amount} direction={direction === 'incoming' ? 'CREDIT' : 'DEBIT'} className="text-sm" />
              {direction === 'incoming' && t.status === 'COMPLETED' && (
                <Button variant="ghost" onClick={() => { setRefunding(t); setError('') }}>Devolver</Button>
              )}
            </li>
          ))}
        </ul>
      )}
      <Modal open={!!refunding} onClose={() => setRefunding(null)} title="Devolver Pix">
        <form onSubmit={(e) => { e.preventDefault(); refund.mutate() }} className="space-y-4">
          <p className="text-sm text-fg-2">Para {refunding?.counterparty.name}, até {formatBRL(refunding?.amount)}.</p>
          <MoneyField label="Valor a devolver" onValue={setAmount} />
          <Alert>{error}</Alert>
          <Button type="submit" loading={refund.isPending} disabled={!amount}>Devolver</Button>
        </form>
      </Modal>
    </Panel>
  )
}

function ReceiveSandbox({ keys }: { keys: PixKey[] }) {
  const [amount, setAmount] = useState<string | null>(null)
  const [msg, setMsg] = useState('')
  const receive = useMutation({
    mutationFn: () => api('POST', '/v1/sandbox/pix-in', { body: { key: keys[0].value, amount, payer_name: 'Empresa X Ltda' } }),
    onSuccess: () => setMsg(`${formatBRL(amount!)} a caminho — veja a notificação.`),
    onError: (e) => setMsg(errorMessage(e)),
  })
  if (keys.length === 0) return null
  return (
    <Sandbox>
      <form onSubmit={(e) => { e.preventDefault(); receive.mutate() }} className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="flex-1"><MoneyField label={`Receber um Pix de outro banco em ${keys[0].value}`} onValue={setAmount} /></div>
        <Button type="submit" loading={receive.isPending} disabled={!amount}>Receber</Button>
      </form>
      {msg && <p className="mt-2 text-sm text-warning">{msg}</p>}
    </Sandbox>
  )
}

export function PixPage() {
  const account = useAccount()
  const [params, setParams] = useSearchParams()
  const tab = (params.get('aba') as Tab) || 'enviar'
  const keys = usePixKeys(account?.id)
  useEffect(() => { if (!params.get('aba')) setParams({ aba: 'enviar' }, { replace: true }) }, [params, setParams])
  if (!account || keys.isLoading) return <Loading />
  const k = keys.data ?? []
  return (
    <div className="space-y-6">
      <PageTitle subtitle="Transferências instantâneas, 24 horas por dia.">Pix</PageTitle>
      <Tabs<Tab> value={tab} onChange={(t) => setParams({ aba: t })} tabs={[
        ['enviar', 'Enviar'], ['cobrar', 'Cobrar'], ['pagar', 'Pagar QR Code'], ['chaves', 'Minhas chaves'], ['historico', 'Histórico']]} />
      {tab === 'enviar' && <SendPix accountId={account.id} />}
      {tab === 'cobrar' && <ChargePix accountId={account.id} keys={k} />}
      {tab === 'pagar' && <PayQR accountId={account.id} />}
      {tab === 'chaves' && <Keys accountId={account.id} keys={k} />}
      {tab === 'historico' && <History accountId={account.id} />}
      <ReceiveSandbox keys={k} />
    </div>
  )
}
