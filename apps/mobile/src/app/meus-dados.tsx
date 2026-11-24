import { errorMessage } from '@bank/core'
import { useMutation } from '@tanstack/react-query'
import { FileDown } from 'lucide-react-native'
import { useState } from 'react'
import { Share } from 'react-native'
import { useStepUp } from '../components/StepUp'
import { Button, Card, Muted, Notice, Screen } from '../components/ui'
import { api } from '../lib/client'

/** LGPD: everything the bank keeps about the customer, shared as JSON. */
export default function MyData() {
  const { confirm, modal } = useStepUp()
  const [error, setError] = useState('')
  const download = useMutation({
    mutationFn: async () => api<unknown>('POST', '/v1/me/data-export', { stepUp: await confirm() }),
    onSuccess: async (data) => {
      setError('')
      await Share.share({ title: 'Meus dados — Digital Bank', message: JSON.stringify(data, null, 2) })
    },
    onError: (e) => { if (!(e instanceof Error && e.message === 'cancelled')) setError(errorMessage(e)) },
  })
  return (
    <Screen>
      <Card title="Seus dados" icon={FileDown}>
        <Muted>
          Tudo o que guardamos sobre você: cadastro, contas, chaves Pix, cartões (só os 4 últimos dígitos), caixinhas, extrato e acessos.
          Registros de conta e de transações ficam guardados pelo prazo exigido por lei, mesmo depois de encerrar a conta.
        </Muted>
        <Notice>{error}</Notice>
        <Button title="Baixar meus dados" icon={FileDown} variant="secondary" loading={download.isPending} onPress={() => download.mutate()} />
      </Card>
      {modal}
    </Screen>
  )
}
