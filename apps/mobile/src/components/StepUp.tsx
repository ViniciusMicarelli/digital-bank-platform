import { errorMessage } from '@bank/core'
import { ShieldCheck } from 'lucide-react-native'
import { useRef, useState } from 'react'
import { KeyboardAvoidingView, Modal, Platform, StyleSheet, View } from 'react-native'
import { client } from '../lib/client'
import { useTheme } from '../lib/theme'
import { Button, Field, IconTile, Muted, Notice, Txt } from './ui'

let cached: { token: string; until: number } | null = null

/** confirm() resolves with a step-up token, asking for the password when needed. */
export function useStepUp() {
  const { c } = useTheme()
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
        <View style={[st.sheet, { backgroundColor: c.surface, borderColor: c.line }]} accessibilityViewIsModal>
          <View style={[st.grip, { backgroundColor: c.lineStrong }]} />
          <IconTile icon={ShieldCheck} tone="brand" size={48} />
          <Txt variant="heading" style={{ fontSize: 20 }}>Confirme que é você</Txt>
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
  backdrop: { flex: 1, justifyContent: 'flex-end', backgroundColor: '#00000080' },
  sheet: { borderTopLeftRadius: 28, borderTopRightRadius: 28, padding: 22, paddingBottom: 38, gap: 12, borderWidth: 1 },
  grip: { alignSelf: 'center', width: 40, height: 5, borderRadius: 3, marginBottom: 4 },
})
