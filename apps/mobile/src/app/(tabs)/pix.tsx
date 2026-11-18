import { errorMessage, formatBRL, type Party, type PixKey } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import QRCode from 'qrcode'
import { useState } from 'react'
import { Share, Text, View } from 'react-native'
import { SvgXml } from 'react-native-svg'
import { Button, Card, Field, MoneyField, Muted, Notice, Row, Sandbox, Screen, Segmented } from '../../components/ui'
import { api } from '../../lib/client'
import { useAccount } from '../../lib/queries'
import { colors } from '../../lib/theme'

type Tab = 'enviar' | 'receber' | 'pagar'

function Send({ accountId }: { accountId: string }) {
  const qc = useQueryClient()
  const [key, setKey] = useState('')
  const [owner, setOwner] = useState<Party | null>(null)
  const [amount, setAmount] = useState<string | null>(null)
  const [done, setDone] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [idem, setIdem] = useState(() => crypto.randomUUID())
  const lookup = useMutation({
    mutationFn: () => api<{ owner: Party }>('GET', `/v1/pix/keys/${encodeURIComponent(key.trim())}`),
    onSuccess: (r) => { setOwner(r.owner); setError('') }, onError: (e) => setError(errorMessage(e)),
  })
  const send = useMutation({
    mutationFn: () => api('POST', `/v1/accounts/${accountId}/pix/transfers`, { body: { key: key.trim(), amount, description: '' }, idempotent: idem }),
    onSuccess: () => { setDone(`${formatBRL(amount!)} para ${owner!.name}`); qc.invalidateQueries({ queryKey: ['balance'] }) },
    onError: (e) => setError(errorMessage(e)),
  })
  const reset = () => { setKey(''); setOwner(null); setDone(null); setError(''); setIdem(crypto.randomUUID()) }
  if (done) return <Card title="Pix enviado ✓"><Text style={{ fontSize: 18 }}>{done}</Text><Button title="Novo Pix" variant="secondary" onPress={reset} /></Card>
  return (
    <Card title="Enviar Pix">
      {!owner ? (
        <>
          <Field label="Chave Pix" autoCapitalize="none" placeholder="CPF, e-mail, celular ou aleatória" value={key} onChangeText={setKey} />
          <Notice>{error}</Notice>
          <Button title="Continuar" loading={lookup.isPending} onPress={() => lookup.mutate()} />
        </>
      ) : (
        <>
          <View style={{ backgroundColor: colors.bg, borderRadius: 12, padding: 12 }}>
            <Text style={{ fontWeight: '600' }}>{owner.name}</Text>
            <Muted>{owner.document} · {owner.bank}</Muted>
          </View>
          <MoneyField label="Valor" onValue={setAmount} />
          <Notice>{error}</Notice>
          <Button title={`Enviar ${amount ? formatBRL(amount) : ''}`} disabled={!amount} loading={send.isPending} onPress={() => send.mutate()} />
          <Button title="Voltar" variant="ghost" onPress={reset} />
        </>
      )}
    </Card>
  )
}

function Receive({ accountId }: { accountId: string }) {
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
      <Card title="Minhas chaves">
        {keys.data?.map((k) => <Row key={k.id} title={k.value} subtitle={k.type} />)}
        <Field label="Nova chave (e-mail)" keyboardType="email-address" autoCapitalize="none" value={email} onChangeText={setEmail} />
        <View style={{ flexDirection: 'row', gap: 10 }}>
          <View style={{ flex: 1 }}><Button title="Cadastrar e-mail" variant="secondary" disabled={!email} onPress={() => add.mutate({ type: 'EMAIL', value: email.trim() })} /></View>
          <View style={{ flex: 1 }}><Button title="Chave aleatória" variant="secondary" onPress={() => add.mutate({ type: 'EVP', value: '' })} /></View>
        </View>
        <Notice>{error}</Notice>
      </Card>
      {first && (
        <Card title="Cobrar com QR Code">
          <MoneyField label="Valor (opcional)" onValue={(v) => { setAmount(v); setSvg(null) }} />
          {svg ? (
            <View style={{ alignItems: 'center', gap: 10 }}>
              <View accessibilityLabel="QR Code Pix"><SvgXml xml={svg.xml} width={220} height={220} /></View>
              <Button title="Compartilhar Pix copia e cola" variant="secondary" onPress={() => Share.share({ message: svg.payload })} />
            </View>
          ) : <Button title="Gerar QR Code" loading={qr.isPending} onPress={() => qr.mutate()} />}
          <Sandbox>
            <Text>Receber um Pix de outro banco em {first.value}</Text>
            <Button title={amount ? `Receber ${formatBRL(amount)}` : 'Informe o valor acima'} disabled={!amount} loading={pixIn.isPending} onPress={() => pixIn.mutate()} />
            {msg ? <Text style={{ color: colors.amber }}>{msg}</Text> : null}
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
    <Card title="Pagar Pix copia e cola">
      <Field label="Código" multiline numberOfLines={4} value={payload} onChangeText={setPayload} />
      {!info ? <Button title="Ler código" loading={decode.isPending} onPress={() => decode.mutate()} /> : (
        <>
          <Text style={{ fontWeight: '600' }}>{info.receiver.name} · {info.receiver.bank}</Text>
          {info.amount ? <Text style={{ fontSize: 22, fontWeight: '700' }}>{formatBRL(info.amount.amount)}</Text> : <MoneyField label="Valor" onValue={setAmount} />}
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
    <Screen title="Pix">
      <Segmented<Tab> value={tab} onChange={setTab} options={[['enviar', 'Enviar'], ['receber', 'Receber'], ['pagar', 'Copia e cola']]} />
      {tab === 'enviar' && <Send accountId={account.id} />}
      {tab === 'receber' && <Receive accountId={account.id} />}
      {tab === 'pagar' && <PayQR accountId={account.id} />}
    </Screen>
  )
}
