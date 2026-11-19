import { LinearGradient } from 'expo-linear-gradient'
import { Nfc } from 'lucide-react-native'
import { useEffect } from 'react'
import { StyleSheet, View } from 'react-native'
import Animated, { interpolate, useAnimatedStyle, useSharedValue, withTiming, Easing } from 'react-native-reanimated'
import { font } from '../lib/theme'
import { Txt } from './ui'

export type CardSkin = 'debit' | 'credit' | 'blocked'

const skins: Record<CardSkin, [string, string, string]> = {
  debit: ['#0e7a5f', '#0a4d3c', '#07362a'],
  credit: ['#4a37a8', '#2d2075', '#1c1450'],
  blocked: ['#667085', '#475467', '#1d2939'],
}

function Chip() {
  return (
    <LinearGradient colors={['#f6e7b0', '#d4b062', '#f3dc96']} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={st.chip}>
      <View style={[st.chipLine, { top: '33%' }]} />
      <View style={[st.chipLine, { top: '66%' }]} />
      <View style={[st.chipLineV, { left: '33%' }]} />
      <View style={[st.chipLineV, { left: '66%' }]} />
    </LinearGradient>
  )
}

/** A payment card that flips to its back (number and CVV) when revealed. */
export function BankCard({ holder, last4, expiry, skin = 'debit', label = 'Débito', revealed }: {
  holder: string; last4: string; expiry: string; skin?: CardSkin; label?: string; revealed?: { pan: string; cvv: string } | null
}) {
  const flip = useSharedValue(0)
  useEffect(() => {
    flip.set(withTiming(revealed ? 1 : 0, { duration: 700, easing: Easing.inOut(Easing.cubic) }))
  }, [revealed, flip])
  const front = useAnimatedStyle(() => ({
    transform: [{ perspective: 1200 }, { rotateY: `${interpolate(flip.get(), [0, 1], [0, 180])}deg` }],
    opacity: flip.get() < 0.5 ? 1 : 0,
  }))
  const back = useAnimatedStyle(() => ({
    transform: [{ perspective: 1200 }, { rotateY: `${interpolate(flip.get(), [0, 1], [180, 360])}deg` }],
    opacity: flip.get() >= 0.5 ? 1 : 0,
  }))
  const colors = skins[skin]
  return (
    <View accessibilityRole="image" accessibilityLabel={`Cartão ${label.toLowerCase()} final ${last4}`} style={st.wrap}>
      <Animated.View style={[StyleSheet.absoluteFill, front]}>
        <LinearGradient colors={colors} start={{ x: 1, y: 0 }} end={{ x: 0, y: 1 }} style={st.face}>
          <View style={st.between}>
            <Txt variant="heading" tone="white" style={{ fontFamily: font.bold }}>Digital Bank</Txt>
            <View style={st.pillLabel}><Txt variant="caption" tone="white" style={{ fontFamily: font.semibold, letterSpacing: 1 }}>{label.toUpperCase()}</Txt></View>
          </View>
          <View style={{ gap: 14 }}>
            <View style={[st.between, { justifyContent: 'flex-start', gap: 10 }]}><Chip /><Nfc size={22} color="#ffffffcc" style={{ transform: [{ rotate: '90deg' }] }} /></View>
            <Txt tone="white" style={st.pan}>•••• •••• •••• {last4}</Txt>
          </View>
          <View style={st.between}>
            <View style={{ flex: 1 }}>
              <Txt variant="caption" style={st.small}>TITULAR</Txt>
              <Txt variant="label" tone="white" numberOfLines={1} style={{ fontFamily: font.semibold }}>{holder}</Txt>
            </View>
            <View style={{ marginRight: 14 }}>
              <Txt variant="caption" style={st.small}>VALIDADE</Txt>
              <Txt variant="label" tone="white" style={{ fontFamily: font.semibold }}>{expiry}</Txt>
            </View>
            <View style={{ flexDirection: 'row' }}>
              <View style={[st.dot, { backgroundColor: '#ffffffd9' }]} />
              <View style={[st.dot, { backgroundColor: '#ffffff73', marginLeft: -10 }]} />
            </View>
          </View>
        </LinearGradient>
      </Animated.View>
      <Animated.View style={[StyleSheet.absoluteFill, back]} accessibilityElementsHidden={!revealed} importantForAccessibility={revealed ? 'auto' : 'no-hide-descendants'}>
        <LinearGradient colors={colors} start={{ x: 1, y: 0 }} end={{ x: 0, y: 1 }} style={[st.face, { justifyContent: 'flex-start', gap: 18 }]}>
          {revealed && <>
            <View style={st.stripe} />
            <Txt tone="white" style={st.pan}>{revealed.pan.replace(/(\d{4})(?=\d)/g, '$1 ')}</Txt>
            <View style={{ flexDirection: 'row', gap: 28 }}>
              <View><Txt variant="caption" style={st.small}>VALIDADE</Txt><Txt variant="label" tone="white" style={{ fontFamily: font.semibold }}>{expiry}</Txt></View>
              <View><Txt variant="caption" style={st.small}>CVV</Txt><View style={st.cvv}><Txt variant="label" style={{ color: '#0b1220', fontFamily: font.bold }}>{revealed.cvv}</Txt></View></View>
            </View>
          </>}
        </LinearGradient>
      </Animated.View>
    </View>
  )
}

const st = StyleSheet.create({
  wrap: { width: '100%', aspectRatio: 1.586 },
  face: { flex: 1, borderRadius: 22, padding: 20, justifyContent: 'space-between', overflow: 'hidden', borderWidth: 1, borderColor: '#ffffff1a' },
  between: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  pillLabel: { backgroundColor: '#ffffff26', borderRadius: 999, paddingHorizontal: 10, paddingVertical: 3 },
  pan: { fontSize: 19, letterSpacing: 2.5, fontVariant: ['tabular-nums'], fontFamily: 'Courier' },
  small: { color: '#ffffff99', fontSize: 10, letterSpacing: 1 },
  chip: { width: 42, height: 31, borderRadius: 6 },
  chipLine: { position: 'absolute', left: 0, right: 0, height: 1, backgroundColor: '#8a6d2c66' },
  chipLineV: { position: 'absolute', top: 0, bottom: 0, width: 1, backgroundColor: '#8a6d2c66' },
  dot: { width: 26, height: 26, borderRadius: 13 },
  stripe: { height: 40, backgroundColor: '#000000b3', marginHorizontal: -20, marginTop: 4 },
  cvv: { backgroundColor: '#fff', borderRadius: 4, paddingHorizontal: 8, marginTop: 2 },
})
