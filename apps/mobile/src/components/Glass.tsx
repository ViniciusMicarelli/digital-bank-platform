import { GlassView, isGlassEffectAPIAvailable, isLiquidGlassAvailable } from 'expo-glass-effect'
import type { ReactNode } from 'react'
import { Platform, StyleSheet, View, type StyleProp, type ViewStyle } from 'react-native'
import { useTheme } from '../lib/theme'

const liquid = Platform.OS === 'ios' && isLiquidGlassAvailable() && isGlassEffectAPIAvailable()

/**
 * A surface. Solid with a hairline border: cheap to draw, so scrolling and
 * taps stay smooth on any phone (a blur per card was not). Only the tab bar
 * (strong) uses Apple's Liquid Glass, on iOS 26 where it is native.
 */
export function Glass({ children, style, radius = 24, strong }: {
  children?: ReactNode; style?: StyleProp<ViewStyle>; radius?: number; strong?: boolean; interactive?: boolean
}) {
  const { c, dark } = useTheme()
  const shape: ViewStyle = { borderRadius: radius, overflow: 'hidden' }
  if (strong && liquid) {
    return <GlassView glassEffectStyle="regular" colorScheme={dark ? 'dark' : 'light'} style={[shape, style]}>{children}</GlassView>
  }
  return (
    <View style={[shape, { backgroundColor: c.surface, borderWidth: StyleSheet.hairlineWidth, borderColor: strong ? c.lineStrong : c.line }, style]}>
      {children}
    </View>
  )
}

/** The screen background: plain, the same color the system bars get. */
export function AuroraBackground() {
  const { c } = useTheme()
  return <View pointerEvents="none" style={[StyleSheet.absoluteFill, { backgroundColor: c.bg }]} />
}
