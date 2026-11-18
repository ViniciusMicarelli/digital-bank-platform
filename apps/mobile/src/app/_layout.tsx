import { ApiError } from '@bank/core'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Stack } from 'expo-router'
import { StatusBar } from 'expo-status-bar'
import { useEffect, useState } from 'react'
import { SafeAreaProvider } from 'react-native-safe-area-context'
import { Live } from '../components/Live'
import { Loading } from '../components/ui'
import { client, restoreSession } from '../lib/client'
import { SessionContext } from '../lib/queries'

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 15_000, retry: (n, e) => !(e instanceof ApiError && e.status < 500) && n < 2 } },
})

export default function RootLayout() {
  const [restored, setRestored] = useState(false)
  const [signedIn, setSignedIn] = useState(false)
  useEffect(() => {
    const off = client.onSessionChange((v) => {
      setSignedIn(v)
      if (!v) queryClient.clear()
    })
    restoreSession().finally(() => {
      setSignedIn(client.hasSession())
      setRestored(true)
    })
    return off
  }, [])
  if (!restored) return <Loading />
  const stack = (
    <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: '#f8fafc' } }}>
      <Stack.Protected guard={!signedIn}>
        <Stack.Screen name="entrar" />
        <Stack.Screen name="criar-conta" />
      </Stack.Protected>
      <Stack.Protected guard={signedIn}>
        <Stack.Screen name="(tabs)" />
        <Stack.Screen name="cadastro" />
        <Stack.Screen name="notificacoes" options={{ headerShown: true, title: 'Notificações', headerBackTitle: 'Voltar' }} />
        <Stack.Screen name="boletos" options={{ headerShown: true, title: 'Boletos', headerBackTitle: 'Voltar' }} />
        <Stack.Screen name="credito" options={{ headerShown: true, title: 'Crédito', headerBackTitle: 'Voltar' }} />
      </Stack.Protected>
    </Stack>
  )
  return (
    <SafeAreaProvider>
      <QueryClientProvider client={queryClient}>
        <SessionContext.Provider value={signedIn}>
          {signedIn ? <Live>{stack}</Live> : stack}
        </SessionContext.Provider>
      </QueryClientProvider>
      <StatusBar style="dark" />
    </SafeAreaProvider>
  )
}
