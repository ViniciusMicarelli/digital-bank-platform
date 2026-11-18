import { errorMessage } from '@bank/core'
import { Link } from 'expo-router'
import { useState } from 'react'
import { Text } from 'react-native'
import { Button, Field, Notice, Screen, s } from '../components/ui'
import { client } from '../lib/client'
import { colors } from '../lib/theme'
import { Brand } from '../components/Brand'

export default function Register() {
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
  return (
    <Screen>
      <Brand />
      <Text style={s.title}>Criar conta</Text>
      <Field label="Nome completo" autoComplete="name" value={f.name} onChangeText={(name) => setF({ ...f, name })} />
      <Field label="E-mail" keyboardType="email-address" autoCapitalize="none" value={f.email} onChangeText={(email) => setF({ ...f, email })} />
      <Field label="Senha" secureTextEntry hint="De 10 a 128 caracteres. Uma frase é ótima." value={f.password} onChangeText={(password) => setF({ ...f, password })} />
      <Notice>{error}</Notice>
      <Button title="Criar conta" loading={busy} onPress={submit} />
      <Link href="/entrar" style={{ textAlign: 'center', color: colors.brand, fontWeight: '600', marginTop: 8 }}>Já tenho conta</Link>
    </Screen>
  )
}
