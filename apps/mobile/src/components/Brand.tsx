import { Text, View } from 'react-native'
import { colors } from '../lib/theme'

export function Brand() {
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center', gap: 10, marginVertical: 24 }}>
      <View style={{ width: 40, height: 40, borderRadius: 12, backgroundColor: colors.brand, alignItems: 'center', justifyContent: 'center' }}>
        <Text style={{ color: '#fff', fontWeight: '700', fontSize: 20 }}>D</Text>
      </View>
      <Text style={{ fontSize: 22, fontWeight: '700', color: colors.text }}>Digital Bank</Text>
    </View>
  )
}

