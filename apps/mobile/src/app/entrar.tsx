import { ApiError, errorMessage } from '@bank/core'
import { Link } from 'expo-router'
import { useState } from 'react'
import { Text } from 'react-native'
import { Brand } from '../components/Brand'
import { Button, Field, Notice, Screen, s } from '../components/ui'
import { client } from '../lib/client'
import { colors } from '../lib/theme'

export default function Login() {
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
    <Screen>
      <Brand />
      <Text style={s.title}>Entrar</Text>
      <Field label="E-mail" keyboardType="email-address" autoCapitalize="none" autoComplete="email" value={email} onChangeText={setEmail} />
      <Field label="Senha" secureTextEntry autoComplete="current-password" value={password} onChangeText={setPassword} onSubmitEditing={submit} />
      {needsTotp && <Field label="Código do autenticador" keyboardType="number-pad" maxLength={6} value={totp} onChangeText={setTotp} />}
      <Notice>{error}</Notice>
      <Button title="Entrar" loading={busy} onPress={submit} />
      <Link href="/criar-conta" style={{ textAlign: 'center', color: colors.brand, fontWeight: '600', marginTop: 8 }}>Criar conta</Link>
    </Screen>
  )
}
