import { errorMessage, formatBRL, type Money } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import { FileText, Share2 } from 'lucide-react-native'
import { useState } from 'react'
import { Share } from 'react-native'
import { Button, Card, Loading, Muted, Notice, Row, Screen, Segmented, Txt } from '../components/ui'
import { api } from '../lib/client'
import { useAccounts } from '../lib/queries'

interface Report {
  year: number
  complete: boolean
  holder: { name: string; document: string }
  account: { branch: string; number: string }
  checking: { balance_start: Money; balance_end: Money }
  savings: { boxes: { name: string; status: string; balance_start: Money; balance_end: Money; yielded: Money }[]; yielded: Money }
}

const currentYear = () => new Date(Date.now() - 3 * 3600_000).getUTCFullYear()

/** The annual income report ("informe de rendimentos"), to share or keep. */
export default function TaxReport() {
  const accounts = useAccounts().data ?? []
  const account = accounts.find((a) => a.status === 'ACTIVE') ?? accounts[0]
  const [thisYear] = useState(currentYear)
  const [year, setYear] = useState(String(thisYear - 1))
  const report = useQuery({
    queryKey: ['tax-report', account?.id, year], enabled: !!account,
    queryFn: () => api<Report>('GET', `/v1/accounts/${account!.id}/tax-report?year=${year}`),
  })
  const r = report.data
  const share = () => r && Share.share({
    message: [
      `Informe de rendimentos ${r.year} — Digital Bank`,
      `${r.holder.name} · CPF ${r.holder.document} · Ag. ${r.account.branch} Conta ${r.account.number}`,
      `Conta corrente: 31/12/${r.year - 1} ${formatBRL(r.checking.balance_start)} · 31/12/${r.year} ${formatBRL(r.checking.balance_end)}`,
      ...r.savings.boxes.map((b) => `Caixinha ${b.name}: ${formatBRL(b.balance_start)} → ${formatBRL(b.balance_end)}; rendeu ${formatBRL(b.yielded)}`),
      `Rendimentos no ano: ${formatBRL(r.savings.yielded)}`,
    ].join('\n'),
  })
  return (
    <Screen>
      <Muted>Para a declaração do Imposto de Renda: saldos em 31/12 e rendimentos das caixinhas no ano.</Muted>
      <Segmented value={year} onChange={setYear} options={[String(thisYear - 2), String(thisYear - 1), String(thisYear)].map((y) => [y, y] as [string, string])} />
      {!account ? <Muted>Você ainda não tem conta.</Muted> : report.isLoading ? <Loading /> : report.error ? <Notice>{errorMessage(report.error)}</Notice> : r && (
        <>
          {!r.complete && <Notice tone="amber">O ano ainda não terminou: os valores de 31/12/{r.year} são os de hoje.</Notice>}
          <Card title={`Saldos em 31/12/${r.year}`} icon={FileText} index={1}>
            <Row title="Conta corrente" subtitle={`31/12/${r.year - 1}: ${formatBRL(r.checking.balance_start)}`}
              right={<Txt variant="label">{formatBRL(r.checking.balance_end)}</Txt>} />
            {r.savings.boxes.map((b) => (
              <Row key={b.name + b.balance_start.amount} title={`Caixinha ${b.name}${b.status === 'CLOSED' ? ' (encerrada)' : ''}`}
                subtitle={`31/12/${r.year - 1}: ${formatBRL(b.balance_start)} · rendeu ${formatBRL(b.yielded)}`}
                right={<Txt variant="label">{formatBRL(b.balance_end)}</Txt>} />
            ))}
          </Card>
          <Card title="Rendimentos no ano" index={2}>
            <Txt variant="heading">{formatBRL(r.savings.yielded)}</Txt>
            <Muted>Ambiente de demonstração: sem imposto retido na fonte.</Muted>
          </Card>
          <Button title="Compartilhar" icon={Share2} variant="secondary" onPress={() => void share()} />
        </>
      )}
    </Screen>
  )
}
