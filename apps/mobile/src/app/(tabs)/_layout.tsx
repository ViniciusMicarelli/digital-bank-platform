import { Redirect, Tabs } from 'expo-router'
import { ArrowLeftRight, CreditCard, House, Menu, type LucideIcon } from 'lucide-react-native'
import { StyleSheet, type ColorValue } from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { Glass } from '../../components/Glass'
import { Loading, Screen } from '../../components/ui'
import { useAccounts, useProfile } from '../../lib/queries'
import { font, useTheme } from '../../lib/theme'

function icon(Icon: LucideIcon) {
  const TabIcon = ({ color, focused }: { color: ColorValue; focused: boolean }) => <Icon size={22} color={color as string} strokeWidth={focused ? 2.4 : 2} />
  return TabIcon
}

export default function TabsLayout() {
  const { c } = useTheme()
  const insets = useSafeAreaInsets()
  const profile = useProfile()
  const accounts = useAccounts()
  if (profile.isLoading || accounts.isLoading) return <Screen><Loading rows={5} /></Screen>
  // No verified identity or no account yet: onboarding first.
  if (profile.data?.kyc_status !== 'APPROVED' || !accounts.data?.length) return <Redirect href="/cadastro" />
  return (
    <Tabs safeAreaInsets={{ bottom: 0 }} screenOptions={{
      headerShown: false, tabBarActiveTintColor: c.brandStrong, tabBarInactiveTintColor: c.muted,
      // A floating Liquid Glass capsule, as in iOS 26.
      tabBarStyle: {
        position: 'absolute', bottom: Math.max(insets.bottom, 12), marginHorizontal: 20, height: 66, borderRadius: 33,
        borderTopWidth: 0, backgroundColor: 'transparent', elevation: 0, paddingBottom: 8, paddingTop: 8,
        shadowColor: c.shadow, shadowOpacity: 0.18, shadowRadius: 24, shadowOffset: { width: 0, height: 10 },
      },
      tabBarBackground: () => <Glass strong radius={33} style={StyleSheet.absoluteFill} />,
      tabBarItemStyle: { borderRadius: 26 },
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
