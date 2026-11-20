import { errorMessage, formatBRL, type Party, type PixContact, type PixKey } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, IdCard, KeyRound, Mail, QrCode, Send, Share2, Smartphone, Star, type LucideIcon } from 'lucide-react-native'
import QRCode from 'qrcode'
import { useState } from 'react'
import { ScrollView, Share, View } from 'react-native'
import Animated, { ZoomIn } from 'react-native-reanimated'
import { SvgXml } from 'react-native-svg'
import { Button, Card, Field, MoneyField, Muted, Notice, Press, Row, Sandbox, Screen, Segmented, Txt } from '../../components/ui'
import { api } from '../../lib/client'
import { useAccount } from '../../lib/queries'
import { font, useTheme } from '../../lib/theme'

type Tab = 'enviar' | 'receber' | 'pagar'
const keyIcons: Record<string, LucideIcon> = { EMAIL: Mail, PHONE: Smartphone, CPF: IdCard, EVP: KeyRound }

function Avatar({ name }: { name: string }) {
  const { c } = useTheme()
  const p = name.split(' ').filter(Boolean)
  return (
    <View style={{ width: 48, height: 48, borderRadius: 24, backgroundColor: c.credit, alignItems: 'center', justifyContent: 'center' }}>
      <Txt tone="white" style={{ fontFamily: font.bold }}>{(p[0]?.[0] ?? '') + (p.at(-1)?.[0] ?? '')}</Txt>
    </View>
  )
}

function Success({ title, detail, onAgain }: { title: string; detail: string; onAgain: () => void }) {
  const { c } = useTheme()
  return (
    <Card>
      <View style={{ alignItems: 'center', gap: 10, paddingVertical: 10 }}>
        <Animated.View entering={ZoomIn.springify().damping(12)} style={{ width: 72, height: 72, borderRadius: 36, backgroundColor: c.brand, alignItems: 'center', justifyContent: 'center' }}>
          <Check size={36} color={c.brandFg} strokeWidth={3} />
        </Animated.View>
        <Txt variant="heading">{title}</Txt>
        <Txt variant="label" tone="fg2">{detail}</Txt>
      </View>
      <Button title="Novo Pix" variant="secondary" onPress={onAgain} />
    </Card>
  )
}

const norm = (k: string) => k.toLowerCase().replace(/[^a-z0-9@.]/g, '')

function SendPix({ accountId }: { accountId: string }) {
  const { c } = useTheme()
  const qc = useQueryClient()
  const [key, setKey] = useState('')
  const [owner, setOwner] = useState<Party | null>(null)
  const [amount, setAmount] = useState<string | null>(null)
  const [done, setDone] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [idem, setIdem] = useState(() => crypto.randomUUID())
  const contacts = useQuery({
    queryKey: ['pix', accountId, 'contacts'],
    queryFn: async () => (await api<{ contacts: PixContact[] }>('GET', `/v1/accounts/${accountId}/pix/contacts`)).contacts,
  })
  const favorite = contacts.data?.find((x) => norm(x.key) === norm(key))?.favorite
  const toggleFavorite = useMutation({
    mutationFn: () => favorite
      ? api('DELETE', `/v1/accounts/${accountId}/pix/contacts/favorites?key=${encodeURIComponent(key.trim())}`)
      : api('PUT', `/v1/accounts/${accountId}/pix/contacts/favorites`, { body: { key: key.trim() } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['pix', accountId, 'contacts'] }),
  })
  const lookup = useMutation({
    mutationFn: (k: string = key) => api<{ owner: Party }>('GET', `/v1/pix/keys/${encodeURIComponent(k.trim())}`),
    onSuccess: (r) => { setOwner(r.owner); setError('') }, onError: (e) => setError(errorMessage(e)),
  })
  const send = useMutation({
    mutationFn: () => api('POST', `/v1/accounts/${accountId}/pix/transfers`, { body: { key: key.trim(), amount, description: '' }, idempotent: idem }),
    onSuccess: () => { setDone(`${formatBRL(amount!)} para ${owner!.name}`); for (const k of ['balance', 'pix']) qc.invalidateQueries({ queryKey: [k] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  const reset = () => { setKey(''); setOwner(null); setDone(null); setError(''); setIdem(crypto.randomUUID()) }
  if (done) return <Success title="Pix enviado ✓" detail={done} onAgain={reset} />
  return (
    <Card title="Enviar Pix" icon={Send}>
      {!owner ? (
        <>
          {!!contacts.data?.length && (
            <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 12 }}>
              {contacts.data.map((x) => (
                <Press key={x.key} accessibilityLabel={`Pix para ${x.nickname || x.name}`} onPress={() => { setKey(x.key); lookup.mutate(x.key) }}
                  style={{ alignItems: 'center', gap: 6, width: 68 }}>
                  <View>
                    <Avatar name={x.nickname || x.name || x.key} />
                    {x.favorite && <Star size={14} color={c.warning} fill={c.warning} style={{ position: 'absolute', right: -2, bottom: -2 }} />}
                  </View>
                  <Txt variant="caption" numberOfLines={1}>{x.nickname || x.name.split(' ')[0] || x.key}</Txt>
                </Press>
              ))}
            </ScrollView>
          )}
          <Field label="Chave Pix" autoCapitalize="none" placeholder="CPF, e-mail, celular ou aleatória" value={key} onChangeText={setKey} />
          <Notice>{error}</Notice>
          <Button title="Continuar" loading={lookup.isPending} onPress={() => lookup.mutate(key)} />
        </>
      ) : (
        <>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12, backgroundColor: c.surface2, borderRadius: 16, padding: 12 }}>
            <Avatar name={owner.name} />
            <View style={{ flex: 1 }}>
              <Txt variant="label" style={{ fontFamily: font.semibold }}>{owner.name}</Txt>
              <Txt variant="caption" tone="muted">{owner.document} · {owner.bank}</Txt>
            </View>
            <Press accessibilityLabel={favorite ? 'Remover dos favoritos' : 'Favoritar'} onPress={() => toggleFavorite.mutate()}>
              <Star size={22} color={favorite ? c.warning : c.subtle} fill={favorite ? c.warning : 'transparent'} />
            </Press>
          </View>
          <MoneyField label="Valor" onValue={setAmount} />
          <Notice>{error}</Notice>
          <Button title={`Enviar ${amount ? formatBRL(amount) : ''}`} icon={Send} disabled={!amount} loading={send.isPending} onPress={() => send.mutate()} />
          <Button title="Voltar" variant="ghost" onPress={reset} />
        </>
      )}
    </Card>
  )
}

function Receive({ accountId }: { accountId: string }) {
  const { c } = useTheme()
  const qc = useQueryClient()
  const keys = useQuery({
    queryKey: ['pix', accountId, 'keys'],
    queryFn: async () => (await api<{ keys: PixKey[] }>('GET', `/v1/accounts/${accountId}/pix/keys`)).keys,
  })
  const [email, setEmail] = useState('')
  const [amount, setAmount] = useState<string | null>(null)
  const [svg, setSvg] = useState<{ xml: string; payload: string } | null>(null)
  const [msg, setMsg] = useState('')
  const [error, setError] = useState('')
  const first = keys.data?.[0]
  const add = useMutation({
    mutationFn: (v: { type: string; value: string }) => api('POST', `/v1/accounts/${accountId}/pix/keys`, { body: v }),
    onSuccess: () => { setEmail(''); setError(''); qc.invalidateQueries({ queryKey: ['pix', accountId, 'keys'] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  const qr = useMutation({
    mutationFn: () => api<{ payload: string }>('POST', `/v1/accounts/${accountId}/pix/qr-codes`, {
      body: { key: first!.value, amount: amount ?? undefined, description: 'Cobrança', one_time: true, expires_in_seconds: 3600 } }),
    onSuccess: async (r) => setSvg({ payload: r.payload, xml: await QRCode.toString(r.payload, { type: 'svg', margin: 1 }) }),
    onError: (e) => setError(errorMessage(e)),
  })
  const pixIn = useMutation({
    mutationFn: () => api('POST', '/v1/sandbox/pix-in', { body: { key: first!.value, amount, payer_name: 'Empresa X Ltda' } }),
    onSuccess: () => setMsg(`${formatBRL(amount!)} a caminho.`), onError: (e) => setMsg(errorMessage(e)),
  })
  return (
    <>
      <Card title="Minhas chaves" icon={KeyRound}>
        {keys.data?.map((k) => <Row key={k.id} icon={keyIcons[k.type] ?? KeyRound} tone="brand" title={k.value}
          subtitle={{ EMAIL: 'E-mail', PHONE: 'Celular', CPF: 'CPF', EVP: 'Chave aleatória' }[k.type]} />)}
        <Field label="Nova chave (e-mail)" keyboardType="email-address" autoCapitalize="none" value={email} onChangeText={setEmail} />
        <View style={{ flexDirection: 'row', gap: 10 }}>
          <View style={{ flex: 1 }}><Button title="Cadastrar e-mail" variant="secondary" disabled={!email} onPress={() => add.mutate({ type: 'EMAIL', value: email.trim() })} /></View>
          <View style={{ flex: 1 }}><Button title="Chave aleatória" variant="secondary" onPress={() => add.mutate({ type: 'EVP', value: '' })} /></View>
        </View>
        <Notice>{error}</Notice>
      </Card>
      {first && (
        <Card title="Cobrar com QR Code" icon={QrCode} index={1}>
          <MoneyField label="Valor (opcional)" onValue={(v) => { setAmount(v); setSvg(null) }} />
          {svg ? (
            <View style={{ alignItems: 'center', gap: 12 }}>
              <Animated.View entering={ZoomIn.duration(350)} accessibilityLabel="QR Code Pix" style={{ backgroundColor: '#fff', padding: 14, borderRadius: 24, borderWidth: 1, borderColor: c.line }}>
                <SvgXml xml={svg.xml} width={210} height={210} />
              </Animated.View>
              <Button title="Compartilhar Pix copia e cola" icon={Share2} variant="secondary" onPress={() => Share.share({ message: svg.payload })} />
            </View>
          ) : <Button title="Gerar QR Code" icon={QrCode} loading={qr.isPending} onPress={() => qr.mutate()} />}
          <Sandbox>
            <Txt variant="label" tone="fg2">Receber um Pix de outro banco em {first.value}</Txt>
            <Button title={amount ? `Receber ${formatBRL(amount)}` : 'Informe o valor acima'} disabled={!amount} loading={pixIn.isPending} onPress={() => pixIn.mutate()} />
            {msg ? <Txt variant="label" tone="warning">{msg}</Txt> : null}
          </Sandbox>
        </Card>
      )}
    </>
  )
}

function PayQR({ accountId }: { accountId: string }) {
  const qc = useQueryClient()
  const [payload, setPayload] = useState('')
  const [info, setInfo] = useState<{ receiver: Party; amount?: { amount: string } } | null>(null)
  const [amount, setAmount] = useState<string | null>(null)
  const [result, setResult] = useState('')
  const [error, setError] = useState('')
  const decode = useMutation({
    mutationFn: () => api<typeof info>('POST', '/v1/pix/qr-codes/decode', { body: { payload: payload.trim() } }),
    onSuccess: (d) => { setInfo(d); setError('') }, onError: (e) => setError(errorMessage(e)),
  })
  const pay = useMutation({
    mutationFn: () => api('POST', `/v1/accounts/${accountId}/pix/qr-payments`, { body: { payload: payload.trim(), amount: info?.amount ? undefined : amount }, idempotent: true }),
    onSuccess: () => { setResult('Pago ✓'); qc.invalidateQueries({ queryKey: ['balance'] }) }, onError: (e) => setError(errorMessage(e)),
  })
  return (
    <Card title="Pagar Pix copia e cola" icon={QrCode}>
      <Field label="Código" multiline numberOfLines={4} value={payload} onChangeText={setPayload} style={{ minHeight: 100, textAlignVertical: 'top' }} />
      {!info ? <Button title="Ler código" loading={decode.isPending} onPress={() => decode.mutate()} /> : (
        <>
          <Row icon={Send} tone="brand" title={info.receiver.name} subtitle={info.receiver.bank} />
          {info.amount ? <Txt variant="title">{formatBRL(info.amount.amount)}</Txt> : <MoneyField label="Valor" onValue={setAmount} />}
          <Button title="Pagar" loading={pay.isPending} onPress={() => pay.mutate()} />
        </>
      )}
      <Notice tone="green">{result}</Notice>
      <Notice>{error}</Notice>
    </Card>
  )
}

export default function Pix() {
  const account = useAccount()
  const [tab, setTab] = useState<Tab>('enviar')
  if (!account) return null
  return (
    <Screen title="Pix" subtitle="Transferências instantâneas, 24 horas por dia.">
      <Segmented<Tab> value={tab} onChange={setTab} options={[['enviar', 'Enviar'], ['receber', 'Receber'], ['pagar', 'Copia e cola']]} />
      {tab === 'enviar' && <SendPix accountId={account.id} />}
      {tab === 'receber' && <Receive accountId={account.id} />}
      {tab === 'pagar' && <PayQR accountId={account.id} />}
      <Muted> </Muted>
    </Screen>
  )
}
