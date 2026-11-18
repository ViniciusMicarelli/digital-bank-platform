import { errorMessage, formatBRL, type Card as CardT, type CardDetails } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { StyleSheet, Text, View } from 'react-native'
import { useStepUp } from '../../components/StepUp'
import { Button, Card, Loading, MoneyField, Muted, Notice, Sandbox, Screen, Segmented, Toggle } from '../../components/ui'
import { api } from '../../lib/client'
import { useAccount } from '../../lib/queries'
import { colors } from '../../lib/theme'

function CardArt({ card, details }: { card: CardT; details: CardDetails | null }) {
  return (
    <View style={[st.art, card.status === 'BLOCKED' && { backgroundColor: '#64748b' }]} accessibilityLabel={`Cartão final ${card.last4}`}>
      <View style={st.artTop}><Text style={st.artText}>Digital Bank</Text><Text style={st.artSmall}>{card.kind === 'VIRTUAL' ? 'VIRTUAL' : 'FÍSICO'}</Text></View>
      <Text style={st.pan}>{details ? details.pan.replace(/(\d{4})(?=\d)/g, '$1 ') : `•••• •••• •••• ${card.last4}`}</Text>
      <View style={st.artTop}>
        <View><Text style={st.artSmall}>Titular</Text><Text style={st.artText}>{card.holder_name}</Text></View>
        <View><Text style={st.artSmall}>Validade</Text><Text style={st.artText}>{String(card.expiry_month).padStart(2, '0')}/{String(card.expiry_year).slice(2)}</Text></View>
        {details && <View><Text style={st.artSmall}>CVV</Text><Text style={st.artText}>{details.cvv}</Text></View>}
      </View>
      {card.status === 'BLOCKED' && <Text style={[st.artText, { textAlign: 'center' }]}>BLOQUEADO</Text>}
    </View>
  )
}

export default function Cards() {
  const account = useAccount()
  const qc = useQueryClient()
  const { confirm, modal } = useStepUp()
  const [details, setDetails] = useState<CardDetails | null>(null)
  const [error, setError] = useState('')
  const [amount, setAmount] = useState<string | null>(null)
  const [fn, setFn] = useState<'DEBIT' | 'CREDIT'>('DEBIT')
  const [result, setResult] = useState('')
  const key = ['cards', account?.id]
  const cards = useQuery({ queryKey: key, enabled: !!account, queryFn: async () => (await api<{ cards: CardT[] }>('GET', `/v1/accounts/${account!.id}/cards`)).cards })
  const card = cards.data?.[0]
  const base = `/v1/accounts/${account?.id}/cards/${card?.id}`
  const refresh = () => qc.invalidateQueries({ queryKey: key })
  const cancelled = (e: unknown) => e instanceof Error && e.message === 'cancelled'
  const issue = useMutation({ mutationFn: () => api('POST', `/v1/accounts/${account!.id}/cards`, { body: { kind: 'VIRTUAL' }, idempotent: true }), onSuccess: refresh })
  const reveal = async () => {
    if (details) return details
    const d = await api<CardDetails>('POST', `${base}/reveal`, { stepUp: await confirm() })
    setDetails(d)
    return d
  }
  const show = useMutation({ mutationFn: reveal, onError: (e) => !cancelled(e) && setError(errorMessage(e)) })
  const block = useMutation({
    mutationFn: () => card!.status === 'ACTIVE' ? api('POST', `${base}/block`, { body: { reason: 'Bloqueado pelo cliente' } }) : api('POST', `${base}/unblock`),
    onSuccess: refresh,
  })
  const controls = useMutation({ mutationFn: (c: CardT['controls']) => api('PUT', `${base}/controls`, { body: c }), onSuccess: refresh })
  const buy = useMutation({
    mutationFn: async () => {
      const d = await reveal()
      const a = await api<{ id: string; status: string; response_code: string; reason: string }>('POST', '/v1/sandbox/card-network/authorizations', {
        body: { pan: d.pan, expiry_month: d.expiry_month, expiry_year: d.expiry_year, cvv: d.cvv, amount, merchant_name: 'MERCADO DO BAIRRO', mcc: '5411', country: 'BR', channel: 'ONLINE', function: fn } })
      if (a.status === 'APPROVED') await api('POST', `/v1/sandbox/card-network/authorizations/${a.id}/clear`, { body: {} })
      return a
    },
    onSuccess: (a) => setResult(a.status === 'APPROVED' ? `Aprovada: ${formatBRL(amount!)}` : `Recusada (${a.response_code}): ${a.reason}`),
    onError: (e) => !cancelled(e) && setResult(errorMessage(e)),
  })

  if (!account || cards.isLoading) return <Loading />
  if (!card) {
    return (
      <Screen title="Cartões">
        <Card><Muted>Você ainda não tem cartão.</Muted><Button title="Criar cartão virtual" loading={issue.isPending} onPress={() => issue.mutate()} /></Card>
      </Screen>
    )
  }
  const set = (k: keyof CardT['controls']) => (v: boolean) => controls.mutate({ ...card.controls, [k]: v })
  return (
    <Screen title="Cartões">
      <CardArt card={card} details={details} />
      <View style={{ flexDirection: 'row', gap: 10 }}>
        <View style={{ flex: 1 }}>
          {details ? <Button title="Ocultar dados" variant="secondary" onPress={() => setDetails(null)} />
            : <Button title="Ver dados" variant="secondary" disabled={card.status !== 'ACTIVE'} loading={show.isPending} onPress={() => show.mutate()} />}
        </View>
        <View style={{ flex: 1 }}>
          <Button title={card.status === 'ACTIVE' ? 'Bloquear' : 'Desbloquear'} variant={card.status === 'ACTIVE' ? 'danger' : 'primary'} loading={block.isPending} onPress={() => block.mutate()} />
        </View>
      </View>
      <Notice>{error}</Notice>
      <Card title="Controles">
        <Toggle label="Compras online" value={card.controls.online} onChange={set('online')} />
        <Toggle label="Compras internacionais" value={card.controls.international} onChange={set('international')} />
        <Toggle label="Aproximação" value={card.controls.contactless} onChange={set('contactless')} />
      </Card>
      {card.status === 'ACTIVE' && (
        <Sandbox>
          <MoneyField label="Simular compra de" onValue={setAmount} />
          <Segmented value={fn} onChange={setFn} options={[['DEBIT', 'Débito'], ['CREDIT', 'Crédito']]} />
          <Button title="Passar o cartão" disabled={!amount} loading={buy.isPending} onPress={() => buy.mutate()} />
          {result ? <Text style={{ color: colors.amber }}>{result}</Text> : null}
        </Sandbox>
      )}
      {modal}
    </Screen>
  )
}

const st = StyleSheet.create({
  art: { backgroundColor: colors.brandDark, borderRadius: 20, padding: 20, gap: 22, aspectRatio: 1.586 },
  artTop: { flexDirection: 'row', justifyContent: 'space-between' },
  artText: { color: '#fff', fontWeight: '600' },
  artSmall: { color: '#c7d2fe', fontSize: 11 },
  pan: { color: '#fff', fontSize: 20, letterSpacing: 2, fontVariant: ['tabular-nums'], fontFamily: 'Courier' },
})
