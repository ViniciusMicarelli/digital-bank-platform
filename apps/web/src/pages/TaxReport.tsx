import { errorMessage, formatBRL, type Money } from '@bank/core'
import { useQuery } from '@tanstack/react-query'
import { FileText, Printer } from 'lucide-react'
import { useState } from 'react'
import { useAccounts } from '../app/queries'
import { Brand } from '../components/Brand'
import { Alert, Button, Empty, Loading, PageTitle, Panel } from '../components/ui'
import { api } from '../lib/api'
import { dateTimeBR } from '../lib/format'

interface Report {
  year: number
  complete: boolean
  generated_at: string
  holder: { name: string; document: string }
  account: { branch: string; number: string }
  checking: { balance_start: Money; balance_end: Money }
  savings: {
    boxes: { name: string; status: string; balance_start: Money; balance_end: Money; yielded: Money }[]
    balance_start: Money; balance_end: Money; yielded: Money
  }
}

/** The current year in Brasília (the bank's calendar). */
const currentYear = () => new Date(Date.now() - 3 * 3600_000).getUTCFullYear()

function Row({ label, prev, cur }: { label: string; prev: Money; cur: Money }) {
  return (
    <tr className="border-t border-line">
      <td className="py-2 pr-3">{label}</td>
      <td className="tabular py-2 pr-3 text-right">{formatBRL(prev)}</td>
      <td className="tabular py-2 text-right">{formatBRL(cur)}</td>
    </tr>
  )
}

/** The annual income report ("informe de rendimentos"): what the account
 * and its boxes held on 31/12 and what the boxes yielded, ready to print. */
export function TaxReportPage() {
  const accounts = useAccounts().data ?? []
  const [thisYear] = useState(currentYear)
  const years = [thisYear - 1, thisYear - 2, thisYear - 3, thisYear]
  const [year, setYear] = useState(thisYear - 1)
  const [accountId, setAccountId] = useState<string | null>(null)
  const account = accounts.find((a) => a.id === accountId) ?? accounts.find((a) => a.status === 'ACTIVE') ?? accounts[0]
  const report = useQuery({
    queryKey: ['tax-report', account?.id, year],
    enabled: !!account,
    queryFn: () => api<Report>('GET', `/v1/accounts/${account!.id}/tax-report?year=${year}`),
  })
  const r = report.data
  return (
    <div className="space-y-6">
      <PageTitle subtitle="Para a declaração do Imposto de Renda: saldos em 31/12 e rendimentos das caixinhas no ano."
        action={r && <Button variant="secondary" icon={Printer} onClick={() => window.print()}>Imprimir ou salvar PDF</Button>}>
        Informe de rendimentos
      </PageTitle>
      <div data-reveal className="flex flex-wrap gap-2 print:hidden">
        <select aria-label="Ano" value={year} onChange={(e) => setYear(Number(e.target.value))}
          className="rounded-full bg-surface-2 px-4 py-2.5 text-sm font-medium text-fg">
          {years.map((y) => <option key={y} value={y}>Ano-calendário {y}</option>)}
        </select>
        {accounts.length > 1 && (
          <select aria-label="Conta" value={account?.id} onChange={(e) => setAccountId(e.target.value)}
            className="rounded-full bg-surface-2 px-4 py-2.5 text-sm font-medium text-fg">
            {accounts.map((a) => <option key={a.id} value={a.id}>Conta {a.number}{a.status === 'CLOSED' ? ' (encerrada)' : ''}</option>)}
          </select>
        )}
      </div>
      {!account ? <Panel><Empty icon={FileText}>Você ainda não tem conta.</Empty></Panel> : report.isLoading ? <Panel><Loading rows={5} /></Panel>
        : report.error ? <Alert>{errorMessage(report.error)}</Alert> : r && (
          <Panel className="receipt">
            <div className="space-y-5 text-sm text-fg">
              <div className="flex items-start justify-between gap-4">
                <Brand />
                <div className="text-right">
                  <p className="font-semibold">Informe de rendimentos</p>
                  <p className="text-xs text-muted">Ano-calendário {r.year}</p>
                </div>
              </div>
              {!r.complete && <Alert tone="amber">O ano ainda não terminou: os valores de 31/12/{r.year} são os de hoje e podem mudar.</Alert>}
              <dl className="grid gap-1 sm:grid-cols-2">
                <div><dt className="text-xs text-muted">Titular</dt><dd className="font-medium">{r.holder.name} · CPF {r.holder.document}</dd></div>
                <div><dt className="text-xs text-muted">Conta</dt><dd className="font-medium">Ag. {r.account.branch} · Conta {r.account.number}</dd></div>
              </dl>
              <section>
                <h2 className="mb-1 font-semibold">Saldos em 31/12 (bens e direitos)</h2>
                <table className="w-full">
                  <thead>
                    <tr className="text-xs text-muted">
                      <th className="py-1 text-left font-medium">Aplicação</th>
                      <th className="py-1 text-right font-medium">31/12/{r.year - 1}</th>
                      <th className="py-1 text-right font-medium">31/12/{r.year}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <Row label="Conta corrente" prev={r.checking.balance_start} cur={r.checking.balance_end} />
                    {r.savings.boxes.map((b) => (
                      <Row key={b.name + b.balance_start.amount} label={`Caixinha ${b.name}${b.status === 'CLOSED' ? ' (encerrada)' : ''}`}
                        prev={b.balance_start} cur={b.balance_end} />
                    ))}
                  </tbody>
                </table>
              </section>
              <section>
                <h2 className="mb-1 font-semibold">Rendimentos no ano</h2>
                {r.savings.boxes.length === 0 ? <p className="text-muted">Nenhum rendimento em {r.year}.</p> : (
                  <table className="w-full">
                    <tbody>
                      {r.savings.boxes.map((b) => (
                        <tr key={b.name + b.yielded.amount} className="border-t border-line">
                          <td className="py-2 pr-3">Caixinha {b.name}</td>
                          <td className="tabular py-2 text-right">{formatBRL(b.yielded)}</td>
                        </tr>
                      ))}
                      <tr className="border-t border-line font-semibold">
                        <td className="py-2 pr-3">Total</td>
                        <td className="tabular py-2 text-right">{formatBRL(r.savings.yielded)}</td>
                      </tr>
                    </tbody>
                  </table>
                )}
              </section>
              <p className="text-xs text-muted">
                Ambiente de demonstração: sem imposto retido na fonte. Gerado em {dateTimeBR(r.generated_at)}.
              </p>
            </div>
          </Panel>
        )}
    </div>
  )
}
