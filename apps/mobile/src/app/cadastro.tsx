import { errorMessage } from '@bank/core'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Redirect } from 'expo-router'
import { useState } from 'react'
import { Check, IdCard, Landmark, LogOut, ScanFace, ShieldCheck } from 'lucide-react-native'
import { View } from 'react-native'
import Animated, { FadeInDown } from 'react-native-reanimated'
import { Button, Card, Field, IconTile, Loading, Muted, Notice, Sandbox, Screen, Txt } from '../components/ui'
import { font, useTheme } from '../lib/theme'
import { api, client } from '../lib/client'
import { useAccounts, useProfile } from '../lib/queries'

const maskCPF = (v: string) =>
  v.replace(/\D/g, '').slice(0, 11).replace(/(\d{3})(\d)/, '$1.$2').replace(/(\d{3})(\d)/, '$1.$2').replace(/(\d{3})(\d{1,2})$/, '$1-$2')
const maskDate = (v: string) => v.replace(/\D/g, '').slice(0, 8).replace(/(\d{2})(\d)/, '$1/$2').replace(/(\d{2})(\d)/, '$1/$2')
const isoDate = (br: string) => { const [d, m, y] = br.split('/'); return y?.length === 4 ? `${y}-${m}-${d}` : '' }
const reasons: Record<string, string> = { UNDERAGE: 'é preciso ter 18 anos ou mais', CPF_IRREGULAR: 'CPF com situação irregular', PEP: 'pessoa politicamente exposta' }

function Steps({ step }: { step: number }) {
  const { c } = useTheme()
  const steps = [['Dados', IdCard], ['Verificação', ScanFace], ['Conta', Landmark]] as const
  return (
    <Animated.View entering={FadeInDown.duration(400)} style={{ flexDirection: 'row', gap: 8 }}>
      {steps.map(([name, Icon], i) => (
        <View key={name} style={{ flex: 1, gap: 6 }}>
          <View style={{ flexDirection: 'row', alignItems: 'center', gap: 6 }}>
            <View style={{ width: 32, height: 32, borderRadius: 16, alignItems: 'center', justifyContent: 'center', borderWidth: 2,
              borderColor: i <= step ? c.brand : c.lineStrong, backgroundColor: i < step ? c.brand : i === step ? c.brandSoft : c.surface }}>
              {i < step ? <Check size={16} color={c.brandFg} strokeWidth={3} /> : <Icon size={15} color={i === step ? c.brandStrong : c.subtle} />}
            </View>
            {i < 2 && <View style={{ flex: 1, height: 2, borderRadius: 1, backgroundColor: i < step ? c.brand : c.line }} />}
          </View>
          <Txt variant="caption" tone={i <= step ? 'fg' : 'subtle'} style={{ fontFamily: font.semibold }}>{name}</Txt>
        </View>
      ))}
    </Animated.View>
  )
}

export default function Onboarding() {
  const qc = useQueryClient()
  const profile = useProfile()
  const accounts = useAccounts()
  const [f, setF] = useState({ name: '', cpf: '', birth: '', cep: '', street: '', number: '', city: '', state: 'SP' })
  const [error, setError] = useState('')
  const refresh = () => qc.invalidateQueries({ queryKey: ['profile'] })
  const fail = (e: unknown) => setError(errorMessage(e))
  const save = useMutation({
    mutationFn: () => api('PUT', '/v1/me/profile', { body: { full_name: f.name, cpf: f.cpf, birth_date: isoDate(f.birth),
      address: { postal_code: f.cep, street: f.street, number: f.number, city: f.city, state: f.state } } }),
    onSuccess: () => { setError(''); refresh() }, onError: fail,
  })
  const kyc = useMutation({ mutationFn: () => api('POST', '/v1/me/kyc'), onSuccess: refresh, onError: fail })
  const review = useMutation({ mutationFn: () => api('POST', '/v1/sandbox/kyc/review', { body: { approve: true } }), onSuccess: refresh })
  const open = useMutation({
    mutationFn: () => api('POST', '/v1/accounts', { body: {}, idempotent: true }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['accounts'] }), onError: fail,
  })
  if (profile.isLoading || accounts.isLoading) return <Screen><Loading rows={4} /></Screen>
  const p = profile.data
  if (p?.kyc_status === 'APPROVED' && accounts.data?.length) return <Redirect href="/" />
  const set = (k: keyof typeof f, mask?: (v: string) => string) => (v: string) => setF({ ...f, [k]: mask ? mask(v) : v })

  return (
    <Screen title="Vamos abrir sua conta" subtitle="Leva dois minutos. Seus dados são verificados e protegidos com criptografia.">
      <Steps step={!p || p.kyc_status === 'REJECTED' ? 0 : p.kyc_status === 'APPROVED' ? 2 : 1} />
      {(!p || p.kyc_status === 'REJECTED') && (
        <Card title="Seus dados" icon={IdCard}>
          {p?.kyc_status === 'REJECTED' && <Notice tone="amber">Não conseguimos verificar: {reasons[p.kyc_reason] ?? p.kyc_reason}. Revise e envie de novo.</Notice>}
          <Field label="Nome completo" value={f.name} onChangeText={set('name')} />
          <Field label="CPF" keyboardType="number-pad" value={f.cpf} onChangeText={set('cpf', maskCPF)} />
          <Field label="Data de nascimento" placeholder="dd/mm/aaaa" keyboardType="number-pad" value={f.birth} onChangeText={set('birth', maskDate)} />
          <Field label="CEP" keyboardType="number-pad" value={f.cep} onChangeText={set('cep')} />
          <Field label="Rua" value={f.street} onChangeText={set('street')} />
          <View style={{ flexDirection: 'row', gap: 10 }}>
            <View style={{ flex: 1 }}><Field label="Número" value={f.number} onChangeText={set('number')} /></View>
            <View style={{ width: 80 }}><Field label="UF" autoCapitalize="characters" maxLength={2} value={f.state} onChangeText={set('state')} /></View>
          </View>
          <Field label="Cidade" value={f.city} onChangeText={set('city')} />
          <Notice>{error}</Notice>
          <Button title="Salvar e continuar" loading={save.isPending} onPress={() => save.mutate()} />
        </Card>
      )}
      {p && p.kyc_status !== 'REJECTED' && p.kyc_status !== 'APPROVED' && (
        <Card title="Verificação de identidade" icon={ScanFace}>
          <Txt variant="label" style={{ fontFamily: font.semibold }}>{p.full_name} · {p.cpf}</Txt>
          {p.kyc_status === 'NOT_STARTED' && <Button title="Enviar para verificação" loading={kyc.isPending} onPress={() => kyc.mutate()} />}
          {p.kyc_status === 'PENDING' && <Notice tone="brand">Verificando com o bureau…</Notice>}
          {p.kyc_status === 'MANUAL_REVIEW' && (
            <>
              <Notice tone="amber">Seu cadastro está em análise manual. Avisamos assim que terminar.</Notice>
              <Sandbox><Button title="Aprovar como analista" loading={review.isPending} onPress={() => review.mutate()} /></Sandbox>
            </>
          )}
          <Notice>{error}</Notice>
        </Card>
      )}
      {p?.kyc_status === 'APPROVED' && (
        <Card title="Identidade verificada" icon={ShieldCheck}>
          <View style={{ alignItems: 'center', paddingVertical: 6 }}><IconTile icon={Check} tone="brand" size={56} /></View>
          <Muted>Tudo certo, {p.full_name.split(' ')[0]}. A conta será aberta com o nome e o CPF verificados.</Muted>
          <Notice>{error}</Notice>
          <Button title="Abrir minha conta" loading={open.isPending} onPress={() => open.mutate()} />
        </Card>
      )}
      <Button title="Sair" variant="ghost" icon={LogOut} onPress={() => client.logout()} />
    </Screen>
  )
}
