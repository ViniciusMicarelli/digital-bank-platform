import { LinearGradient } from 'expo-linear-gradient'
import { useEffect } from 'react'
import { StyleSheet, View } from 'react-native'
import Animated, { Easing, FadeInDown, useAnimatedStyle, useSharedValue, withRepeat, withTiming } from 'react-native-reanimated'
import { heroGradient } from '../lib/theme'
import { BankCard } from './BankCard'
import { Logo } from './Brand'
import { Txt } from './ui'

/** The top of the sign-in screens: ink with jade light and floating cards. */
export function AuthHero({ title, subtitle }: { title: string; subtitle: string }) {
  const float = useSharedValue(0)
  useEffect(() => {
    float.set(withRepeat(withTiming(1, { duration: 3200, easing: Easing.inOut(Easing.sin) }), -1, true))
  }, [float])
  const back = useAnimatedStyle(() => ({ transform: [{ translateY: -6 * float.get() }, { rotate: '-8deg' }] }))
  const front = useAnimatedStyle(() => ({ transform: [{ translateY: 6 * float.get() }, { rotate: '4deg' }] }))
  return (
    <LinearGradient colors={[...heroGradient]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={st.hero}>
      <View style={st.glow} />
      <Animated.View entering={FadeInDown.duration(500)} style={{ flexDirection: 'row', alignItems: 'center', gap: 10 }}>
        <Logo size={36} />
        <Txt variant="heading" tone="white" style={{ fontSize: 18 }}>Digital Bank</Txt>
      </Animated.View>
      <View style={st.cards}>
        <Animated.View entering={FadeInDown.delay(150).duration(700)} style={[st.cardBack, back]}>
          <BankCard holder="ANA SOUZA" last4="4821" expiry="11/31" skin="credit" label="Crédito" />
        </Animated.View>
        <Animated.View entering={FadeInDown.delay(300).duration(700)} style={[st.cardFront, front]}>
          <BankCard holder="ANA SOUZA" last4="0938" expiry="11/31" />
        </Animated.View>
      </View>
      <Animated.View entering={FadeInDown.delay(400).duration(500)}>
        <Txt variant="title" tone="white">{title}</Txt>
        <Txt variant="label" style={{ color: '#ffffffb3', marginTop: 4 }}>{subtitle}</Txt>
      </Animated.View>
    </LinearGradient>
  )
}

const st = StyleSheet.create({
  hero: { paddingHorizontal: 22, paddingTop: 64, paddingBottom: 28, borderBottomLeftRadius: 32, borderBottomRightRadius: 32, overflow: 'hidden', gap: 18 },
  glow: { position: 'absolute', width: 320, height: 320, borderRadius: 160, top: -140, right: -120, backgroundColor: '#2ed3a0', opacity: 0.22 },
  cards: { height: 170, marginVertical: 6 },
  cardBack: { position: 'absolute', width: '68%', right: 0, top: 0, opacity: 0.9 },
  cardFront: { position: 'absolute', width: '72%', left: 0, top: 28 },
})
