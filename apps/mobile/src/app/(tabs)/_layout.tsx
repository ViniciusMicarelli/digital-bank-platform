import { Redirect, Tabs } from 'expo-router'
import { Text, type ColorValue } from 'react-native'
import { Loading } from '../../components/ui'
import { useAccounts, useProfile } from '../../lib/queries'
import { colors } from '../../lib/theme'

function icon(glyph: string) {
  const TabIcon = ({ color }: { color: ColorValue }) => <Text style={{ color, fontSize: 20 }}>{glyph}</Text>
  return TabIcon
}

export default function TabsLayout() {
  const profile = useProfile()
  const accounts = useAccounts()
  if (profile.isLoading || accounts.isLoading) return <Loading />
  // No verified identity or no account yet: onboarding first.
  if (profile.data?.kyc_status !== 'APPROVED' || !accounts.data?.length) return <Redirect href="/cadastro" />
  return (
    <Tabs screenOptions={{ headerShown: false, tabBarActiveTintColor: colors.brand, tabBarInactiveTintColor: colors.muted }}>
      <Tabs.Screen name="index" options={{ title: 'Início', tabBarIcon: icon('⌂') }} />
      <Tabs.Screen name="pix" options={{ title: 'Pix', tabBarIcon: icon('◇') }} />
      <Tabs.Screen name="cartoes" options={{ title: 'Cartões', tabBarIcon: icon('▭') }} />
      <Tabs.Screen name="mais" options={{ title: 'Mais', tabBarIcon: icon('⋯') }} />
    </Tabs>
  )
}
