import { Redirect, Tabs } from 'expo-router'
import { ArrowLeftRight, CreditCard, House, Menu, type LucideIcon } from 'lucide-react-native'
import type { ColorValue } from 'react-native'
import { Loading, Screen } from '../../components/ui'
import { useAccounts, useProfile } from '../../lib/queries'
import { font, useTheme } from '../../lib/theme'

function icon(Icon: LucideIcon) {
  const TabIcon = ({ color, focused }: { color: ColorValue; focused: boolean }) => <Icon size={22} color={color as string} strokeWidth={focused ? 2.4 : 2} />
  return TabIcon
}

export default function TabsLayout() {
  const { c } = useTheme()
  const profile = useProfile()
  const accounts = useAccounts()
  if (profile.isLoading || accounts.isLoading) return <Screen><Loading rows={5} /></Screen>
  // No verified identity or no account yet: onboarding first.
  if (profile.data?.kyc_status !== 'APPROVED' || !accounts.data?.length) return <Redirect href="/cadastro" />
  return (
    <Tabs screenOptions={{
      headerShown: false, tabBarActiveTintColor: c.brandStrong, tabBarInactiveTintColor: c.muted,
      tabBarStyle: { backgroundColor: c.surface, borderTopColor: c.line, height: 64, paddingTop: 6 },
      tabBarLabelStyle: { fontFamily: font.semibold, fontSize: 11 },
      sceneStyle: { backgroundColor: c.bg },
    }}>
      <Tabs.Screen name="index" options={{ title: 'Início', tabBarIcon: icon(House) }} />
      <Tabs.Screen name="pix" options={{ title: 'Pix', tabBarIcon: icon(ArrowLeftRight) }} />
      <Tabs.Screen name="cartoes" options={{ title: 'Cartões', tabBarIcon: icon(CreditCard) }} />
      <Tabs.Screen name="mais" options={{ title: 'Mais', tabBarIcon: icon(Menu) }} />
    </Tabs>
  )
}
