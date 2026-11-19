import { disputeReasons, errorMessage, formatBRL, type Card as CardT, type CardPurchase, type DisputeReason } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Receipt, ShieldAlert } from 'lucide-react-native'
import { useState } from 'react'
import { View } from 'react-native'
import { api } from '../lib/client'
import { dateTime } from '../lib/queries'
import { useTheme } from '../lib/theme'
import { Button, Card, Field, Muted, Notice, Press, Row, Txt } from './ui'

const statusLabel = { OPEN: 'Contestação em análise', WON: 'Contestação aceita', LOST: 'Contestação recusada' } as const

function DisputeForm({ base, purchase, onDone }: { base: string; purchase: CardPurchase; onDone: () => void }) {
  const { c } = useTheme()
  const [reason, setReason] = useState<DisputeReason>('NOT_RECEIVED')
  const [description, setDescription] = useState('')
  const [error, setError] = useState('')
  const open = useMutation({
    mutationFn: () => api('POST', `${base}/purchases/${purchase.id}/disputes`, { body: { reason, description } }),
    onSuccess: onDone,
    onError: (e) => setError(errorMessage(e)),
  })
  return (
    <View style={{ gap: 10, paddingVertical: 10 }}>
      <Txt variant="label" tone="muted">O que aconteceu?</Txt>
      {(Object.entries(disputeReasons) as [DisputeReason, string][]).map(([k, v]) => (
        <Press key={k} onPress={() => setReason(k)} accessibilityLabel={v}>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 10, paddingVertical: 6 }}>
            <View style={{ width: 22, height: 22, borderRadius: 11, borderWidth: 2, borderColor: reason === k ? c.brand : c.line,
              backgroundColor: reason === k ? c.brand : 'transparent', alignItems: 'center', justifyContent: 'center' }}>
              {reason === k && <Check size={13} color="#fff" strokeWidth={3} />}
            </View>
            <Txt variant="label">{v}</Txt>
          </View>
        </Press>
      ))}
      {reason === 'NOT_RECOGNIZED' && <Notice tone="amber">Por segurança, o cartão será bloqueado assim que você enviar.</Notice>}
      <Field label="Conte o que houve (opcional)" value={description} maxLength={500} onChangeText={setDescription} />
      <Notice>{error}</Notice>
      <Button title="Enviar contestação" icon={ShieldAlert} loading={open.isPending} onPress={() => open.mutate()} />
    </View>
  )
}

/** A card's purchases, with a dispute for each cleared one. */
export function Purchases({ accountId, card }: { accountId: string; card: CardT }) {
  const qc = useQueryClient()
  const base = `/v1/accounts/${accountId}/cards/${card.id}`
  const [disputing, setDisputing] = useState<string | null>(null)
  const purchases = useQuery({
    queryKey: ['purchases', card.id],
    queryFn: async () => (await api<{ purchases: CardPurchase[] }>('GET', `${base}/purchases`)).purchases,
  })
  const list = (purchases.data ?? []).filter((p) => p.status === 'CLEARED' || p.status === 'APPROVED')
  return (
    <Card title="Compras" icon={Receipt} index={3}>
      {!list.length ? <Muted>As compras com este cartão aparecem aqui.</Muted> : list.map((p) => {
        const disputable = p.status === 'CLEARED' && !p.dispute && Number(p.refunded_amount.amount) < Number(p.cleared_amount.amount)
        return (
          <View key={p.id}>
            <Row title={p.merchant_name}
              subtitle={`${p.function === 'CREDIT' ? 'Crédito' : 'Débito'} · ${dateTime(p.created_at)}${p.dispute ? ` · ${statusLabel[p.dispute.status]}` : ''}`}
              onPress={disputable ? () => setDisputing(disputing === p.id ? null : p.id) : undefined}
              right={<View style={{ alignItems: 'flex-end' }}>
                <Txt variant="label" style={{ fontVariant: ['tabular-nums'] }}>{formatBRL(p.status === 'CLEARED' ? p.cleared_amount : p.amount)}</Txt>
                {disputable && <Txt variant="caption" tone="brandStrong">Contestar</Txt>}
              </View>} />
            {disputing === p.id && <DisputeForm base={base} purchase={p} onDone={() => {
              setDisputing(null)
              qc.invalidateQueries({ queryKey: ['purchases', card.id] })
              qc.invalidateQueries({ queryKey: ['cards', accountId] })
            }} />}
          </View>
        )
      })}
    </Card>
  )
}
