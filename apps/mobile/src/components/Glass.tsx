import { BlurView } from 'expo-blur'
import { GlassView, isGlassEffectAPIAvailable, isLiquidGlassAvailable } from 'expo-glass-effect'
import { LinearGradient } from 'expo-linear-gradient'
import type { ReactNode } from 'react'
import { Platform, StyleSheet, View, type StyleProp, type ViewStyle } from 'react-native'
import { aurora, useTheme } from '../lib/theme'

const liquid = Platform.OS === 'ios' && isLiquidGlassAvailable() && isGlassEffectAPIAvailable()

/**
 * A glass surface: Apple's Liquid Glass on iOS 26, a system material blur on
 * older iOS and on the web, and a translucent fill on Android (whose blur
 * needs a blur target per screen).
 */
export function Glass({ children, style, radius = 24, strong, interactive }: {
  children?: ReactNode; style?: StyleProp<ViewStyle>; radius?: number; strong?: boolean; interactive?: boolean
}) {
  const { c, dark } = useTheme()
  const shape: ViewStyle = { borderRadius: radius, overflow: 'hidden' }
  if (liquid) {
    return <GlassView glassEffectStyle="regular" isInteractive={interactive} colorScheme={dark ? 'dark' : 'light'} style={[shape, style]}>{children}</GlassView>
  }
  const rim: ViewStyle = { borderWidth: StyleSheet.hairlineWidth * 2, borderColor: c.glassRim }
  if (Platform.OS === 'android') {
    return <View style={[shape, rim, { backgroundColor: strong ? c.glassStrong : c.glass }, style]}>{children}</View>
  }
  return (
    <BlurView intensity={strong ? 80 : 55} tint={dark ? 'systemChromeMaterialDark' : 'systemChromeMaterialLight'} style={[shape, rim, style]}>
      <View pointerEvents="none" style={[StyleSheet.absoluteFill, { backgroundColor: strong ? c.glassStrong : c.glass }]} />
      {children}
    </BlurView>
  )
}

/** The soft color field behind every screen, which the glass refracts. */
export function AuroraBackground() {
  const { dark } = useTheme()
  return (
    <View pointerEvents="none" style={StyleSheet.absoluteFill}>
      <LinearGradient colors={[...(dark ? aurora.dark : aurora.light)]} start={{ x: 0, y: 0 }} end={{ x: 1, y: 1 }} style={StyleSheet.absoluteFill} />
      <View style={[st.blob, { top: -120, left: -100, backgroundColor: dark ? '#2ed3a01f' : '#2ed3a033' }]} />
      <View style={[st.blob, { bottom: 80, right: -140, backgroundColor: dark ? '#7a5af824' : '#7a5af81f' }]} />
    </View>
  )
}

const st = StyleSheet.create({
  blob: { position: 'absolute', width: 360, height: 360, borderRadius: 180 },
})
