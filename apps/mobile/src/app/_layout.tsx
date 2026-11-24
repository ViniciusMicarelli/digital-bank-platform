import { ApiError } from '@bank/core'
import { Inter_400Regular, Inter_500Medium, Inter_600SemiBold, Inter_700Bold, useFonts } from '@expo-google-fonts/inter'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Stack } from 'expo-router'
import { StatusBar } from 'expo-status-bar'
import { useEffect, useState } from 'react'
import { ActivityIndicator, View } from 'react-native'
import { SafeAreaProvider } from 'react-native-safe-area-context'
import { Live } from '../components/Live'
import { client, restoreSession } from '../lib/client'
import { SessionContext } from '../lib/queries'
import { font, ThemeProvider, useTheme } from '../lib/theme'

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 15_000, retry: (n, e) => !(e instanceof ApiError && e.status < 500) && n < 2 } },
})

function Navigator({ signedIn }: { signedIn: boolean }) {
  const { c, dark } = useTheme()
  const header = {
    headerShown: true, headerBackTitle: 'Voltar', headerShadowVisible: false,
    headerStyle: { backgroundColor: c.bg }, headerTintColor: c.fg, headerTitleStyle: { fontFamily: font.semibold },
  }
  const stack = (
    <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: c.bg } }}>
      <Stack.Protected guard={!signedIn}>
        <Stack.Screen name="entrar" />
        <Stack.Screen name="criar-conta" />
      </Stack.Protected>
      <Stack.Protected guard={signedIn}>
        <Stack.Screen name="(tabs)" />
        <Stack.Screen name="cadastro" />
        <Stack.Screen name="notificacoes" options={{ ...header, title: 'Notificações' }} />
        <Stack.Screen name="boletos" options={{ ...header, title: 'Boletos' }} />
        <Stack.Screen name="credito" options={{ ...header, title: 'Crédito' }} />
        <Stack.Screen name="caixinhas" options={{ ...header, title: 'Caixinhas' }} />
        <Stack.Screen name="gastos" options={{ ...header, title: 'Gastos' }} />
        <Stack.Screen name="dispositivos" options={{ ...header, title: 'Dispositivos conectados' }} />
        <Stack.Screen name="encerrar-conta" options={{ ...header, title: 'Encerrar conta' }} />
        <Stack.Screen name="informe" options={{ ...header, title: 'Informe de rendimentos' }} />
        <Stack.Screen name="meus-dados" options={{ ...header, title: 'Seus dados' }} />
        <Stack.Screen name="comprovante/[id]" options={{ ...header, title: 'Comprovante' }} />
      </Stack.Protected>
    </Stack>
  )
  return (
    <View style={{ flex: 1, backgroundColor: c.bg }}>
      {signedIn ? <Live>{stack}</Live> : stack}
      <StatusBar style={dark ? 'light' : 'dark'} />
    </View>
  )
}

export default function RootLayout() {
  const [restored, setRestored] = useState(false)
  const [signedIn, setSignedIn] = useState(false)
  const [fontsLoaded] = useFonts({ Inter_400Regular, Inter_500Medium, Inter_600SemiBold, Inter_700Bold })
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
  return (
    <SafeAreaProvider>
      <ThemeProvider>
        <QueryClientProvider client={queryClient}>
          <SessionContext.Provider value={signedIn}>
            {restored && fontsLoaded ? <Navigator signedIn={signedIn} /> : <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center' }}><ActivityIndicator /></View>}
          </SessionContext.Provider>
        </QueryClientProvider>
      </ThemeProvider>
    </SafeAreaProvider>
  )
}
