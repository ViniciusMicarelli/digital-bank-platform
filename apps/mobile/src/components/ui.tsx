import { useState, type ReactNode } from 'react'
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Switch, Text, TextInput, View, type TextInputProps } from 'react-native'
import { SafeAreaView } from 'react-native-safe-area-context'
import { formatBRL, parseBRL, type Money } from '@bank/core'
import { colors, radius } from '../lib/theme'

export function Screen({ children, title, refreshing, scroll = true }: { children: ReactNode; title?: string; refreshing?: ReactNode; scroll?: boolean }) {
  const body = (
    <>
      {title && <Text style={s.title} accessibilityRole="header">{title}</Text>}
      {children}
    </>
  )
  return (
    <SafeAreaView style={s.screen} edges={['top']}>
      {scroll ? <ScrollView contentContainerStyle={s.content} keyboardShouldPersistTaps="handled" refreshControl={refreshing as never}>{body}</ScrollView>
        : <View style={s.content}>{body}</View>}
    </SafeAreaView>
  )
}

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost'

export function Button({ title, onPress, variant = 'primary', loading, disabled }: { title: string; onPress: () => void; variant?: Variant; loading?: boolean; disabled?: boolean }) {
  const off = disabled || loading
  return (
    <Pressable accessibilityRole="button" accessibilityState={{ disabled: !!off }} onPress={off ? undefined : onPress}
      style={({ pressed }) => [s.button, s[`button_${variant}`], (pressed || off) && { opacity: off ? 0.5 : 0.85 }]}>
      {loading ? <ActivityIndicator color={variant === 'primary' ? '#fff' : colors.brand} /> :
        <Text style={[s.buttonText, s[`buttonText_${variant}`]]}>{title}</Text>}
    </Pressable>
  )
}

export function Field({ label, hint, error, ...rest }: TextInputProps & { label: string; hint?: string; error?: string }) {
  return (
    <View style={s.field}>
      <Text style={s.label}>{label}</Text>
      <TextInput accessibilityLabel={label} placeholderTextColor="#94a3b8" {...rest} style={[s.input, error && { borderColor: colors.red }]} />
      {(error || hint) ? <Text style={[s.hint, error && { color: colors.red }]}>{error ?? hint}</Text> : null}
    </View>
  )
}

/** An amount in Brazilian format; onValue gets "10.50" or null. */
export function MoneyField({ label, onValue, hint }: { label: string; onValue: (v: string | null) => void; hint?: string }) {
  const [text, setText] = useState('')
  return (
    <Field label={label} keyboardType="decimal-pad" placeholder="0,00" value={text} hint={hint}
      error={text && !parseBRL(text) ? 'Valor inválido' : undefined}
      onChangeText={(t) => { setText(t); onValue(parseBRL(t)) }} />
  )
}

export function Card({ children, title, style }: { children: ReactNode; title?: string; style?: object }) {
  return (
    <View style={[s.card, style]}>
      {title && <Text style={s.cardTitle}>{title}</Text>}
      {children}
    </View>
  )
}

export function Toggle({ label, value, onChange }: { label: string; value: boolean; onChange: (v: boolean) => void }) {
  return (
    <View style={s.toggle}>
      <Text style={s.toggleLabel}>{label}</Text>
      <Switch accessibilityLabel={label} value={value} onValueChange={onChange} trackColor={{ true: colors.brand }} />
    </View>
  )
}

export function Notice({ tone = 'red', children }: { tone?: 'red' | 'green' | 'amber' | 'brand'; children?: ReactNode }) {
  if (!children) return null
  const bg = { red: colors.redSoft, green: colors.greenSoft, amber: colors.amberSoft, brand: colors.brandSoft }[tone]
  const fg = { red: colors.red, green: colors.green, amber: colors.amber, brand: colors.brand }[tone]
  return <View accessibilityRole={tone === 'red' ? 'alert' : undefined} style={[s.notice, { backgroundColor: bg }]}><Text style={{ color: fg }}>{children}</Text></View>
}

export function Amount({ value, direction, style }: { value?: Money | string; direction?: 'DEBIT' | 'CREDIT'; style?: object }) {
  return (
    <Text style={[s.amount, direction === 'CREDIT' && { color: colors.green }, style]}>
      {direction === 'DEBIT' ? '− ' : direction === 'CREDIT' ? '+ ' : ''}{formatBRL(value)}
    </Text>
  )
}

export function Row({ title, subtitle, right, onPress }: { title: string; subtitle?: string; right?: ReactNode; onPress?: () => void }) {
  return (
    <Pressable onPress={onPress} disabled={!onPress} style={({ pressed }) => [s.row, pressed && { backgroundColor: colors.bg }]}>
      <View style={{ flex: 1 }}>
        <Text style={s.rowTitle} numberOfLines={1}>{title}</Text>
        {subtitle ? <Text style={s.rowSub} numberOfLines={2}>{subtitle}</Text> : null}
      </View>
      {right}
    </Pressable>
  )
}

export function Segmented<T extends string>({ value, onChange, options }: { value: T; onChange: (v: T) => void; options: [T, string][] }) {
  return (
    <View style={s.segmented} accessibilityRole="tablist">
      {options.map(([id, label]) => (
        <Pressable key={id} accessibilityRole="tab" accessibilityState={{ selected: value === id }} onPress={() => onChange(id)}
          style={[s.segment, value === id && s.segmentOn]}>
          <Text style={[s.segmentText, value === id && { color: colors.text }]}>{label}</Text>
        </Pressable>
      ))}
    </View>
  )
}

export function Sandbox({ children }: { children: ReactNode }) {
  return (
    <View style={s.sandbox}>
      <Text style={s.sandboxTitle}>AMBIENTE DE TESTES</Text>
      {children}
    </View>
  )
}

export const Muted = ({ children }: { children: ReactNode }) => <Text style={s.muted}>{children}</Text>

export const Loading = () => <ActivityIndicator style={{ marginVertical: 32 }} color={colors.brand} />

export const s = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  content: { padding: 16, paddingBottom: 48, gap: 14 },
  title: { fontSize: 26, fontWeight: '700', color: colors.text, marginBottom: 4 },
  button: { borderRadius: radius.md, paddingVertical: 14, paddingHorizontal: 18, alignItems: 'center' },
  button_primary: { backgroundColor: colors.brand },
  button_secondary: { backgroundColor: colors.card, borderWidth: 1, borderColor: colors.border },
  button_danger: { backgroundColor: colors.card, borderWidth: 1, borderColor: '#fecaca' },
  button_ghost: { backgroundColor: 'transparent', paddingVertical: 10 },
  buttonText: { fontSize: 15, fontWeight: '600' },
  buttonText_primary: { color: '#fff' },
  buttonText_secondary: { color: colors.text },
  buttonText_danger: { color: colors.red },
  buttonText_ghost: { color: colors.brand },
  field: { gap: 6 },
  label: { fontSize: 14, fontWeight: '500', color: '#334155' },
  input: { backgroundColor: colors.card, borderWidth: 1, borderColor: colors.border, borderRadius: radius.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 16, color: colors.text },
  hint: { fontSize: 12, color: colors.muted },
  card: { backgroundColor: colors.card, borderRadius: radius.lg, padding: 16, gap: 12, borderWidth: 1, borderColor: '#f1f5f9' },
  cardTitle: { fontSize: 16, fontWeight: '600', color: colors.text },
  toggle: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingVertical: 4 },
  toggleLabel: { fontSize: 15, color: colors.text, flex: 1 },
  notice: { borderRadius: radius.md, padding: 12 },
  amount: { fontSize: 15, fontWeight: '600', color: colors.text, fontVariant: ['tabular-nums'] },
  row: { flexDirection: 'row', alignItems: 'center', gap: 12, paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: colors.border },
  rowTitle: { fontSize: 15, fontWeight: '500', color: colors.text },
  rowSub: { fontSize: 12, color: colors.muted, marginTop: 2 },
  segmented: { flexDirection: 'row', backgroundColor: '#f1f5f9', borderRadius: radius.md, padding: 4, gap: 4 },
  segment: { flex: 1, paddingVertical: 8, borderRadius: radius.sm, alignItems: 'center' },
  segmentOn: { backgroundColor: colors.card },
  segmentText: { fontSize: 14, fontWeight: '500', color: colors.muted },
  sandbox: { borderWidth: 1, borderStyle: 'dashed', borderColor: '#fcd34d', backgroundColor: '#fffbeb99', borderRadius: radius.lg, padding: 14, gap: 10 },
  sandboxTitle: { fontSize: 11, fontWeight: '700', letterSpacing: 1, color: colors.amber },
  muted: { fontSize: 14, color: colors.muted },
})
