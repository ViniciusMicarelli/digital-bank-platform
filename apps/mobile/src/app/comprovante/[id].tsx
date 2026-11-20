import { errorMessage, formatBRL, type Receipt } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import { useLocalSearchParams } from 'expo-router'
import { Share2 } from 'lucide-react-native'
import { Share, View } from 'react-native'
import { Button, Card, Loading, Notice, Screen, Txt } from '../../components/ui'
import { api, apiURL } from '../../lib/client'
import { typeLabels, useAccount } from '../../lib/queries'
import { font, useTheme } from '../../lib/theme'

const fullDate = (iso: string) => new Date(iso).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'medium' })
const account = (branch: string, number: string) => (number ? `Ag ${branch} · Conta ${number}` : '')

export default function ReceiptScreen() {
  const { c } = useTheme()
  const { id } = useLocalSearchParams<{ id: string }>()
  const acc = useAccount()
  const receipt = useQuery({
    queryKey: ['receipt', id], enabled: !!acc,
    queryFn: () => api<Receipt>('GET', `/v1/accounts/${acc!.id}/statement/${id}/receipt`),
  })
  if (receipt.isError) return <Screen><Notice>{errorMessage(receipt.error)}</Notice></Screen>
  const r = receipt.data
  if (!r) return <Screen><Loading /></Screen>
  const debit = r.item.direction === 'DEBIT'
  const me = { name: r.holder_name, detail: account(r.branch, r.number) }
  const them = { name: r.item.counterparty_name || r.item.description || '—', detail: account(r.counterparty_branch, r.counterparty_number) }
  const [from, to] = debit ? [me, them] : [them, me]
  const rows: [string, string][] = [
    ['Tipo', typeLabels[r.item.type] ?? r.item.type],
    ['Data e hora', fullDate(r.item.occurred_at)],
    ...(r.item.description ? [['Descrição', r.item.description] as [string, string]] : []),
    ...(r.item.end_to_end_id ? [['ID Pix', r.item.end_to_end_id] as [string, string]] : []),
  ]
  // The web app serves the public check page; in development it sits next to the API.
  const checkURL = `${apiURL().replace(/:\d+$/, ':8081')}/comprovante/${r.authentication_code}`
  const share = () => Share.share({ message: [
    'Comprovante Digital Bank', `${debit ? 'Enviado' : 'Recebido'}: ${formatBRL(r.item.amount)}`, `De: ${from.name}`, `Para: ${to.name}`,
    ...rows.map(([k, v]) => `${k}: ${v}`), `Autenticação: ${r.authentication_code}`, `Confira: ${checkURL}`,
  ].join('\n') })
  return (
    <Screen>
      <Card>
        <Txt variant="caption" tone="muted">{debit ? 'Você enviou' : 'Você recebeu'}</Txt>
        <Txt variant="title" style={{ fontSize: 32 }}>{formatBRL(r.item.amount)}</Txt>
        <View style={{ backgroundColor: c.surface2, borderRadius: 16, padding: 14, gap: 10 }}>
          {[['De', from], ['Para', to]].map(([label, p]) => {
            const party = p as { name: string; detail: string }
            return (
              <View key={label as string}>
                <Txt variant="caption" tone="muted">{label as string}</Txt>
                <Txt variant="label" style={{ fontFamily: font.semibold }}>{party.name}</Txt>
                {party.detail ? <Txt variant="caption" tone="muted">{party.detail}</Txt> : null}
              </View>
            )
          })}
        </View>
        {rows.map(([k, v]) => (
          <View key={k} style={{ flexDirection: 'row', justifyContent: 'space-between', gap: 12 }}>
            <Txt variant="label" tone="muted">{k}</Txt>
            <Txt variant="label" style={{ flexShrink: 1, textAlign: 'right' }}>{v}</Txt>
          </View>
        ))}
        <View style={{ borderTopWidth: 1, borderColor: c.line, borderStyle: 'dashed', paddingTop: 12 }}>
          <Txt variant="caption" tone="muted">Código de autenticação</Txt>
          <Txt variant="caption" selectable style={{ fontFamily: 'Courier', marginTop: 4 }}>{r.authentication_code}</Txt>
        </View>
      </Card>
      <Button title="Compartilhar comprovante" icon={Share2} onPress={() => void share()} />
    </Screen>
  )
}
