import { ApiError, errorMessage } from '@bank/core'
import { Link } from 'expo-router'
import { ArrowRight } from 'lucide-react-native'
import { useState } from 'react'
import { KeyboardAvoidingView, Platform, ScrollView, View } from 'react-native'
import Animated, { FadeInDown } from 'react-native-reanimated'
import { AuthHero } from '../components/AuthHero'
import { AuroraBackground, Glass } from '../components/Glass'
import { Button, Field, Notice, Txt } from '../components/ui'
import { client } from '../lib/client'
import { font, useTheme } from '../lib/theme'

export default function Login() {
  const { c } = useTheme()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [totp, setTotp] = useState('')
  const [needsTotp, setNeedsTotp] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true)
    setError('')
    try {
      await client.login(email.trim(), password, totp)
    } catch (e) {
      if (e instanceof ApiError && e.code === 'MFA_REQUIRED') setNeedsTotp(true)
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1, backgroundColor: c.bg }}>
      <AuroraBackground />
      <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={{ paddingBottom: 40 }}>
        <AuthHero title="Bem-vindo de volta" subtitle="Entre para ver seu saldo e movimentar sua conta." />
        <Animated.View entering={FadeInDown.delay(500).duration(500)} style={{ marginTop: -26, marginHorizontal: 14 }}>
         <Glass strong radius={30} style={{ padding: 20, gap: 14 }}>
          <Field label="E-mail" keyboardType="email-address" autoCapitalize="none" autoComplete="email" value={email} onChangeText={setEmail} />
          <Field label="Senha" secureTextEntry autoComplete="current-password" value={password} onChangeText={setPassword} onSubmitEditing={submit} />
          {needsTotp && <Field label="Código do autenticador" keyboardType="number-pad" maxLength={6} value={totp} onChangeText={setTotp} />}
          <Notice>{error}</Notice>
          <Button title="Entrar" icon={ArrowRight} loading={busy} onPress={submit} />
          <View style={{ flexDirection: 'row', justifyContent: 'center', gap: 6, marginTop: 6 }}>
            <Txt variant="label" tone="muted">Ainda não tem conta?</Txt>
            <Link href="/criar-conta"><Txt variant="label" tone="brandStrong" style={{ fontFamily: font.semibold }}>Criar conta</Txt></Link>
          </View>
         </Glass>
        </Animated.View>
      </ScrollView>
    </KeyboardAvoidingView>
  )
}
