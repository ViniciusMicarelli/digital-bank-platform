import { errorMessage } from '@bank/core'
import { Link } from 'expo-router'
import { useState } from 'react'
import { KeyboardAvoidingView, Platform, ScrollView, View } from 'react-native'
import Animated, { FadeInDown } from 'react-native-reanimated'
import { AuthHero } from '../components/AuthHero'
import { AuroraBackground, Glass } from '../components/Glass'
import { Button, Field, Notice, Txt } from '../components/ui'
import { client } from '../lib/client'
import { font, useTheme } from '../lib/theme'

export default function Register() {
  const { c } = useTheme()
  const [f, setF] = useState({ name: '', email: '', password: '' })
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true)
    setError('')
    try {
      await client.register(f.email.trim(), f.password, f.name)
      await client.login(f.email.trim(), f.password)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }
  const strength = Math.min(4, Math.floor(f.password.length / 5))
  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1, backgroundColor: c.bg }}>
      <AuroraBackground />
      <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={{ paddingBottom: 40 }}>
        <AuthHero title="Abra sua conta" subtitle="Sem tarifa de manutenção. Leva dois minutos." />
        <Animated.View entering={FadeInDown.delay(500).duration(500)} style={{ marginTop: -26, marginHorizontal: 14 }}>
         <Glass strong radius={30} style={{ padding: 20, gap: 14 }}>
          <Field label="Nome completo" autoComplete="name" value={f.name} onChangeText={(name) => setF({ ...f, name })} />
          <Field label="E-mail" keyboardType="email-address" autoCapitalize="none" value={f.email} onChangeText={(email) => setF({ ...f, email })} />
          <Field label="Senha" secureTextEntry hint="De 10 a 128 caracteres. Uma frase é ótima." value={f.password} onChangeText={(password) => setF({ ...f, password })} />
          <View style={{ flexDirection: 'row', gap: 4 }}>
            {[0, 1, 2, 3].map((i) => <View key={i} style={{ flex: 1, height: 4, borderRadius: 2, backgroundColor: i < strength ? (strength < 2 ? c.warning : c.brand) : c.surface3 }} />)}
          </View>
          <Notice>{error}</Notice>
          <Button title="Criar conta" loading={busy} onPress={submit} />
          <View style={{ flexDirection: 'row', justifyContent: 'center', gap: 6, marginTop: 6 }}>
            <Txt variant="label" tone="muted">Já tem conta?</Txt>
            <Link href="/entrar"><Txt variant="label" tone="brandStrong" style={{ fontFamily: font.semibold }}>Entrar</Txt></Link>
          </View>
         </Glass>
        </Animated.View>
      </ScrollView>
    </KeyboardAvoidingView>
  )
}
