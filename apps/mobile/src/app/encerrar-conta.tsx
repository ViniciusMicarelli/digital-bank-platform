import { closureBlockerText, errorMessage, type ClosureCheck } from '@bank/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { router, type Href } from 'expo-router'
import { DoorClosed } from 'lucide-react-native'
import { useState } from 'react'
import { useStepUp } from '../components/StepUp'
import { Button, Card, Field, Loading, Muted, Notice, Row, Screen } from '../components/ui'
import { api } from '../lib/client'
import { useAccount } from '../lib/queries'

/** Closing the account: what is in the way, what will be cancelled, then the password again. */
export default function CloseAccount() {
  const qc = useQueryClient()
  const account = useAccount()
  const { confirm, modal } = useStepUp()
  const [reason, setReason] = useState('')
  const [error, setError] = useState('')
  const check = useQuery({
    queryKey: ['closure', account?.id], enabled: account?.status === 'ACTIVE',
    queryFn: () => api<ClosureCheck>('GET', `/v1/accounts/${account!.id}/closure`),
  })
  const close = useMutation({
    mutationFn: async () => api('POST', `/v1/accounts/${account!.id}/close`, { body: { reason }, stepUp: await confirm() }),
    onSuccess: () => { qc.invalidateQueries(); router.replace('/') },
    onError: (e) => { if (!(e instanceof Error && e.message === 'cancelled')) setError(errorMessage(e)); check.refetch() },
  })
  if (!account || check.isLoading) return <Screen><Loading /></Screen>
  const c = check.data
  if (!c) return <Screen><Muted>Nenhuma conta ativa para encerrar.</Muted></Screen>
  return (
    <Screen>
      <Muted>Você pode encerrar sua conta quando quiser. O extrato e os comprovantes continuam disponíveis.</Muted>
      {c.blockers.length > 0 ? (
        <Card title="Antes de encerrar, resolva" icon={DoorClosed}>
          {c.blockers.map((b) => {
            const t = closureBlockerText(b)
            return <Row key={b.code} title={t.title} subtitle={t.action} onPress={() => router.push(t.route as Href)} />
          })}
        </Card>
      ) : (
        <Card title="Encerrar conta" icon={DoorClosed}>
          <Muted>Seus cartões serão cancelados e suas chaves Pix, removidas. Agendamentos e Pix Automático também são cancelados. Não dá para desfazer; você pode abrir uma conta nova depois.</Muted>
          <Field label="Por que você está saindo? (opcional)" maxLength={200} value={reason} onChangeText={setReason} />
          <Notice>{error}</Notice>
          <Button title="Encerrar definitivamente" variant="danger" loading={close.isPending} onPress={() => { setError(''); close.mutate() }} />
        </Card>
      )}
      {modal}
    </Screen>
  )
}
