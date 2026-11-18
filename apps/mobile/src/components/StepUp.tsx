import { errorMessage } from '@bank/core'
import { useRef, useState } from 'react'
import { KeyboardAvoidingView, Modal, Platform, StyleSheet, Text, View } from 'react-native'
import { client } from '../lib/client'
import { Button, Field, Muted, Notice } from './ui'

let cached: { token: string; until: number } | null = null

/** confirm() resolves with a step-up token, asking for the password when needed. */
export function useStepUp() {
  const [open, setOpen] = useState(false)
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const pending = useRef<{ resolve: (t: string) => void; reject: (e: Error) => void } | null>(null)

  const confirm = (): Promise<string> => {
    if (cached && cached.until > Date.now()) return Promise.resolve(cached.token)
    setPassword('')
    setError('')
    setOpen(true)
    return new Promise((resolve, reject) => { pending.current = { resolve, reject } })
  }
  async function submit() {
    setBusy(true)
    try {
      const token = await client.stepUp(password)
      cached = { token, until: Date.now() + 4 * 60_000 }
      setOpen(false)
      pending.current?.resolve(token)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }
  const close = () => {
    setOpen(false)
    pending.current?.reject(new Error('cancelled'))
  }
  const modal = (
    <Modal visible={open} transparent animationType="slide" onRequestClose={close}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={st.backdrop}>
        <View style={st.sheet} accessibilityViewIsModal>
          <Text style={st.title}>Confirme que é você</Text>
          <Muted>Por segurança, digite sua senha para continuar.</Muted>
          <Field label="Senha" secureTextEntry autoFocus value={password} onChangeText={setPassword} onSubmitEditing={submit} />
          <Notice>{error}</Notice>
          <Button title="Confirmar" loading={busy} onPress={submit} />
          <Button title="Cancelar" variant="ghost" onPress={close} />
        </View>
      </KeyboardAvoidingView>
    </Modal>
  )
  return { confirm, modal }
}

const st = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: 'flex-end', backgroundColor: '#0f172a66' },
  sheet: { backgroundColor: '#fff', borderTopLeftRadius: 24, borderTopRightRadius: 24, padding: 20, paddingBottom: 36, gap: 12 },
  title: { fontSize: 18, fontWeight: '700' },
})
