import { errorMessage, formatBRL, type Card as CardT, type CardDetails } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, EyeOff, Lock, LockOpen, Plus, ShoppingCart, Timer, Wifi } from 'lucide-react-native'
import { useState } from 'react'
import { ScrollView, View } from 'react-native'
import Animated, { FadeInDown } from 'react-native-reanimated'
import { BankCard } from '../../components/BankCard'
import { Purchases } from '../../components/Purchases'
import { useStepUp } from '../../components/StepUp'
import { Button, Card, IconTile, Loading, MoneyField, Muted, Notice, Press, Sandbox, Screen, Segmented, Toggle, Txt } from '../../components/ui'
import { api } from '../../lib/client'
import { useAccount } from '../../lib/queries'

export default function Cards() {
  const account = useAccount()
  const qc = useQueryClient()
  const { confirm, modal } = useStepUp()
  const [details, setDetails] = useState<CardDetails | null>(null)
  const [error, setError] = useState('')
  const [amount, setAmount] = useState<string | null>(null)
  const [fn, setFn] = useState<'DEBIT' | 'CREDIT'>('DEBIT')
  const [installments, setInstallments] = useState<'1' | '3' | '6' | '10'>('1')
  const [result, setResult] = useState('')
  const key = ['cards', account?.id]
  const cards = useQuery({ queryKey: key, enabled: !!account, queryFn: async () => (await api<{ cards: CardT[] }>('GET', `/v1/accounts/${account!.id}/cards`)).cards })
  const [selected, setSelected] = useState<string | null>(null)
  const card = cards.data?.find((x) => x.id === selected) ?? cards.data?.[0]
  const base = `/v1/accounts/${account?.id}/cards/${card?.id}`
  const refresh = () => qc.invalidateQueries({ queryKey: key })
  const cancelled = (e: unknown) => e instanceof Error && e.message === 'cancelled'
  const issue = useMutation({
    mutationFn: (kind: CardT['kind'] = 'VIRTUAL') => api<CardT>('POST', `/v1/accounts/${account!.id}/cards`, { body: { kind }, idempotent: true }),
    onSuccess: (c) => { setSelected(c.id); setDetails(null); setError(''); refresh() },
    onError: (e) => setError(errorMessage(e)),
  })
  const reveal = async () => {
    if (details) return details
    const d = await api<CardDetails>('POST', `${base}/reveal`, { stepUp: await confirm() })
    setDetails(d)
    return d
  }
  const show = useMutation({ mutationFn: reveal, onError: (e) => { if (!cancelled(e)) setError(errorMessage(e)) } })
  const block = useMutation({
    mutationFn: () => card!.status === 'ACTIVE' ? api('POST', `${base}/block`, { body: { reason: 'Bloqueado pelo cliente' } }) : api('POST', `${base}/unblock`),
    onSuccess: refresh,
  })
  const controls = useMutation({ mutationFn: (c: CardT['controls']) => api('PUT', `${base}/controls`, { body: c }), onSuccess: refresh })
  const buy = useMutation({
    mutationFn: async () => {
      const d = await reveal()
      const a = await api<{ id: string; status: string; response_code: string; reason: string }>('POST', '/v1/sandbox/card-network/authorizations', {
        body: { pan: d.pan, expiry_month: d.expiry_month, expiry_year: d.expiry_year, cvv: d.cvv, amount, merchant_name: 'MERCADO DO BAIRRO', mcc: '5411', country: 'BR', channel: 'ONLINE', function: fn, installments: fn === 'CREDIT' ? Number(installments) : 1 } })
      if (a.status === 'APPROVED') await api('POST', `/v1/sandbox/card-network/authorizations/${a.id}/clear`, { body: {} })
      return a
    },
    onSuccess: (a) => {
      setResult(a.status === 'APPROVED' ? `Aprovada: ${formatBRL(amount!)}` : `Recusada (${a.response_code}): ${a.reason}`)
      qc.invalidateQueries({ queryKey: ['purchases', card?.id] })
    },
    onError: (e) => { if (!cancelled(e)) setResult(errorMessage(e)) },
  })

  if (!account || cards.isLoading) return <Screen title="Cartões"><Loading /></Screen>
  if (!card) {
    return (
      <Screen title="Cartões" subtitle="Débito na conta e, com o crédito ativo, também no crédito.">
        <Card>
          <View style={{ alignItems: 'center', gap: 10, paddingVertical: 8 }}>
            <IconTile icon={Wifi} size={56} />
            <Muted>Você ainda não tem cartão. O virtual fica pronto na hora.</Muted>
          </View>
          <Button title="Criar cartão virtual" icon={Plus} loading={issue.isPending} onPress={() => issue.mutate('VIRTUAL')} />
        </Card>
      </Screen>
    )
  }
  const set = (k: keyof CardT['controls']) => (v: boolean) => controls.mutate({ ...card.controls, [k]: v })
  const temporary = card.kind === 'TEMPORARY'
  const spent = temporary && card.status !== 'ACTIVE'
  const compromised = card.status_reason === 'FRAUD_SUSPECTED'
  return (
    <Screen title="Cartões" subtitle="Débito na conta e, com o crédito ativo, também no crédito.">
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 8 }}>
        {cards.data!.map((x) => (
          <Press key={x.id} accessibilityLabel={`Cartão final ${x.last4}`} onPress={() => { setSelected(x.id); setDetails(null) }}>
            <Txt variant="label" tone={x.id === card.id ? 'fg' : 'muted'} style={{ paddingHorizontal: 12, paddingVertical: 8, borderRadius: 999, overflow: 'hidden', fontWeight: '600' }}>
              {x.kind === 'TEMPORARY' ? 'Temporário' : x.kind === 'VIRTUAL' ? 'Virtual' : 'Físico'} •••• {x.last4}{x.status === 'CANCELLED' ? ' · usado' : ''}
            </Txt>
          </Press>
        ))}
        <Press accessibilityLabel="Novo cartão temporário" onPress={() => issue.mutate('TEMPORARY')}>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4, paddingHorizontal: 12, paddingVertical: 8 }}>
            <Timer size={16} color="#b45309" /><Txt variant="label" style={{ color: '#b45309', fontWeight: '600' }}>Temporário</Txt>
          </View>
        </Press>
      </ScrollView>
      <Animated.View entering={FadeInDown.delay(60).duration(500)}>
        <BankCard holder={card.holder_name} last4={card.last4}
          expiry={`${String(card.expiry_month).padStart(2, '0')}/${String(card.expiry_year).slice(2)}`}
          skin={card.status !== 'ACTIVE' ? 'blocked' : temporary ? 'temporary' : 'debit'}
          label={temporary ? 'Temporário' : card.kind === 'VIRTUAL' ? 'Virtual' : 'Físico'} revealed={details} />
      </Animated.View>
      {temporary && (spent
        ? <Notice tone="amber">Este cartão temporário já foi usado ou expirou.</Notice>
        : <Notice tone="brand">Uso único: cancelado depois da primeira compra. Válido por 24 horas.</Notice>)}
      <View style={{ flexDirection: 'row', gap: 10 }}>
        <View style={{ flex: 1 }}>
          {details ? <Button title="Ocultar dados" icon={EyeOff} variant="secondary" onPress={() => setDetails(null)} />
            : <Button title="Ver dados" icon={Eye} variant="secondary" disabled={card.status !== 'ACTIVE'} loading={show.isPending} onPress={() => show.mutate()} />}
        </View>
{card.status !== 'CANCELLED' && (
        <View style={{ flex: 1 }}>
          <Button title={card.status === 'ACTIVE' ? 'Bloquear' : 'Desbloquear'} icon={card.status === 'ACTIVE' ? Lock : LockOpen}
            variant={card.status === 'ACTIVE' ? 'danger' : 'primary'} loading={block.isPending} disabled={compromised || spent} onPress={() => block.mutate()} />
        </View>
        )}
      </View>
      {compromised
        ? <Notice tone="amber">Bloqueamos este cartão por segurança depois da sua contestação. Peça um novo cartão virtual.</Notice>
        : card.status === 'BLOCKED' && <Notice tone="amber">Cartão bloqueado: compras são recusadas até você desbloquear.</Notice>}
      <Notice>{error}</Notice>
      <Card title="Controles" icon={Lock} index={2}>
        <Toggle label="Compras online" description="Sites e aplicativos" value={card.controls.online} onChange={set('online')} />
        <Toggle label="Compras internacionais" description="Fora do Brasil" value={card.controls.international} onChange={set('international')} />
        <Toggle label="Aproximação" description="Encostar o cartão na maquininha" value={card.controls.contactless} onChange={set('contactless')} />
      </Card>
      <Purchases accountId={account.id} card={card} />
      {card.status === 'ACTIVE' && (
        <Sandbox>
          <MoneyField label="Simular compra de" onValue={setAmount} />
          <Segmented value={fn} onChange={setFn} options={[['DEBIT', 'Débito'], ['CREDIT', 'Crédito']]} />
          {fn === 'CREDIT' && <Segmented value={installments} onChange={setInstallments} options={[['1', 'À vista'], ['3', '3x'], ['6', '6x'], ['10', '10x']]} />}
          <Button title="Passar o cartão" icon={ShoppingCart} disabled={!amount} loading={buy.isPending} onPress={() => buy.mutate()} />
          {result ? <Txt variant="label" tone="warning">{result}</Txt> : null}
        </Sandbox>
      )}
      {modal}
    </Screen>
  )
}
