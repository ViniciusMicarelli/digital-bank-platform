import { openStream, type Notification } from '@bank/core'
import { useQueryClient } from '@tanstack/react-query'
import { ArrowLeftRight, Bell, CreditCard, FileText, ShieldCheck, Wallet, type LucideIcon } from 'lucide-react-native'
import { useEffect, useState, type ReactNode } from 'react'
import { StyleSheet, View } from 'react-native'
import Animated, { FadeOutUp, SlideInUp } from 'react-native-reanimated'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { client, streamTransport } from '../lib/client'
import { font } from '../lib/theme'
import { Txt } from './ui'

const icons: Record<string, LucideIcon> = { PIX: ArrowLeftRight, TRANSFER: ArrowLeftRight, CARD: CreditCard, CREDIT: Wallet, BOLETO: FileText, ACCOUNT: ShieldCheck, PROFILE: ShieldCheck }

/** Live notifications: a banner that slides in on top, and fresh data behind it. */
export function Live({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const insets = useSafeAreaInsets()
  const [banner, setBanner] = useState<(Notification & { key: number }) | null>(null)
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined
    let n = 0
    const stop = openStream({
      url: client.url('/v1/notifications/stream'),
      token: client.ensureAccess,
      transport: streamTransport,
      onEvent: (ev) => {
        if (ev.event !== 'notification') return
        setBanner({ ...JSON.parse(ev.data), key: ++n })
        clearTimeout(timer)
        timer = setTimeout(() => setBanner(null), 5000)
        qc.invalidateQueries()
        // The statement is projected by another service: look again shortly.
        setTimeout(() => { for (const k of ['statement', 'spending', 'budgets']) qc.invalidateQueries({ queryKey: [k] }) }, 1500)
      },
    })
    return () => {
      stop()
      clearTimeout(timer)
    }
  }, [qc])
  const Icon = (banner && icons[banner.category]) || Bell
  return (
    <View style={{ flex: 1 }}>
      {children}
      {banner && (
        <Animated.View key={banner.key} entering={SlideInUp.springify().damping(16)} exiting={FadeOutUp}
          accessibilityLiveRegion="polite" style={[st.banner, { top: insets.top + 8 }]}>
          <View style={st.icon}><Icon size={18} color="#2ed3a0" /></View>
          <View style={{ flex: 1 }}>
            <Txt tone="white" style={{ fontFamily: font.semibold }}>{banner.title}</Txt>
            <Txt variant="label" style={{ color: '#ffffffb3', marginTop: 2 }}>{banner.body}</Txt>
          </View>
        </Animated.View>
      )}
    </View>
  )
}

const st = StyleSheet.create({
  banner: {
    position: 'absolute', left: 12, right: 12, flexDirection: 'row', gap: 12, alignItems: 'flex-start',
    backgroundColor: '#0b1220f2', borderRadius: 20, padding: 14, borderWidth: 1, borderColor: '#ffffff1a',
    shadowColor: '#000', shadowOpacity: 0.3, shadowRadius: 16, shadowOffset: { width: 0, height: 8 }, elevation: 8,
  },
  icon: { width: 36, height: 36, borderRadius: 12, backgroundColor: '#2ed3a026', alignItems: 'center', justifyContent: 'center' },
})
