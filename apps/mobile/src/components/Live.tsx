import { openStream, type Notification } from '@bank/core'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { StyleSheet, Text, View } from 'react-native'
import { useSafeAreaInsets } from 'react-native-safe-area-context'
import { client, streamTransport } from '../lib/client'

/** Live notifications: a banner on top, and fresh data behind it. */
export function Live({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const insets = useSafeAreaInsets()
  const [banner, setBanner] = useState<Notification | null>(null)
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined
    const stop = openStream({
      url: client.url('/v1/notifications/stream'),
      token: client.ensureAccess,
      transport: streamTransport,
      onEvent: (ev) => {
        if (ev.event !== 'notification') return
        setBanner(JSON.parse(ev.data))
        clearTimeout(timer)
        timer = setTimeout(() => setBanner(null), 5000)
        qc.invalidateQueries()
        // The statement is projected by another service: look again shortly.
        setTimeout(() => qc.invalidateQueries({ queryKey: ['statement'] }), 1500)
      },
    })
    return () => {
      stop()
      clearTimeout(timer)
    }
  }, [qc])
  return (
    <View style={{ flex: 1 }}>
      {children}
      {banner && (
        <View accessibilityLiveRegion="polite" style={[st.banner, { top: insets.top + 8 }]}>
          <Text style={st.title}>{banner.title}</Text>
          <Text style={st.body}>{banner.body}</Text>
        </View>
      )}
    </View>
  )
}

const st = StyleSheet.create({
  banner: { position: 'absolute', left: 12, right: 12, backgroundColor: '#0f172a', borderRadius: 16, padding: 14, shadowColor: '#000', shadowOpacity: 0.2, shadowRadius: 12, elevation: 6 },
  title: { color: '#fff', fontWeight: '600', fontSize: 15 },
  body: { color: '#cbd5e1', fontSize: 14, marginTop: 2 },
})
