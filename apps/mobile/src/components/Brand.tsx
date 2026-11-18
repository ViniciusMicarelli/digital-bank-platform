import { LinearGradient } from 'expo-linear-gradient'
import { View } from 'react-native'
import { font } from '../lib/theme'
import { Txt } from './ui'

export function Logo({ size = 40 }: { size?: number }) {
  return (
    <View style={{ width: size, height: size, borderRadius: size * 0.28, backgroundColor: '#0b1220', alignItems: 'center', justifyContent: 'center' }}>
      <LinearGradient colors={['#2ed3a0', '#00805f']} style={{ width: size * 0.42, height: size * 0.46, borderTopRightRadius: size * 0.24, borderBottomRightRadius: size * 0.24, padding: size * 0.08 }}>
        <View style={{ flex: 1, backgroundColor: '#0b1220', borderTopRightRadius: size * 0.16, borderBottomRightRadius: size * 0.16 }} />
      </LinearGradient>
    </View>
  )
}

export function Brand() {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', gap: 10 }}>
      <Logo />
      <Txt variant="heading" style={{ fontSize: 20, fontFamily: font.bold }}>Digital Bank</Txt>
    </View>
  )
}
