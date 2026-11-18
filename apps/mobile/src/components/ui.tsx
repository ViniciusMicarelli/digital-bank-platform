import { formatBRL, parseBRL, type Money } from '@bank/core'
import { AlertCircle, CheckCircle2, FlaskConical, Info, TriangleAlert, type LucideIcon } from 'lucide-react-native'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  ActivityIndicator, Pressable, ScrollView, StyleSheet, Switch, Text, TextInput, View,
  type LayoutChangeEvent, type StyleProp, type TextInputProps, type TextProps, type TextStyle, type ViewStyle,
} from 'react-native'
import Animated, { FadeInDown, useAnimatedStyle, useSharedValue, withRepeat, withSequence, withSpring, withTiming } from 'react-native-reanimated'
import { SafeAreaView } from 'react-native-safe-area-context'
import { font, radius, useTheme, type Colors } from '../lib/theme'

/** Themed text: variant sets size and weight. */
export function Txt({ variant = 'body', tone = 'fg', style, ...rest }: TextProps & {
  variant?: 'display' | 'title' | 'heading' | 'body' | 'label' | 'caption'
  tone?: keyof Colors | 'white'
}) {
  const { c } = useTheme()
  const v: Record<string, TextStyle> = {
    display: { fontSize: 36, fontFamily: font.bold, letterSpacing: -1 },
    title: { fontSize: 28, fontFamily: font.bold, letterSpacing: -0.6 },
    heading: { fontSize: 16, fontFamily: font.semibold, letterSpacing: -0.2 },
    body: { fontSize: 15, fontFamily: font.regular },
    label: { fontSize: 14, fontFamily: font.medium },
    caption: { fontSize: 12, fontFamily: font.regular },
  }
  return <Text {...rest} style={[v[variant], { color: tone === 'white' ? '#fff' : c[tone] }, style]} />
}

export function Screen({ children, title, subtitle, refreshing, scroll = true }: { children: ReactNode; title?: string; subtitle?: string; refreshing?: ReactNode; scroll?: boolean }) {
  const { c } = useTheme()
  const body = (
    <>
      {title && (
        <Animated.View entering={FadeInDown.duration(400)} style={{ marginBottom: 6 }}>
          <Txt variant="title" accessibilityRole="header">{title}</Txt>
          {subtitle && <Txt variant="label" tone="muted" style={{ marginTop: 2 }}>{subtitle}</Txt>}
        </Animated.View>
      )}
      {children}
    </>
  )
  return (
    <SafeAreaView style={{ flex: 1, backgroundColor: c.bg }} edges={['top']}>
      {scroll ? (
        <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled" refreshControl={refreshing as never}>{body}</ScrollView>
      ) : <View style={styles.content}>{body}</View>}
    </SafeAreaView>
  )
}

const AnimatedPressable = Animated.createAnimatedComponent(Pressable)

/** Pressable that springs down a little when touched. */
export function Press({ children, onPress, disabled, style, accessibilityLabel, accessibilityRole = 'button' }: {
  children: ReactNode; onPress?: () => void; disabled?: boolean; style?: StyleProp<ViewStyle>; accessibilityLabel?: string; accessibilityRole?: 'button' | 'tab' | 'link'
}) {
  const scale = useSharedValue(1)
  const anim = useAnimatedStyle(() => ({ transform: [{ scale: scale.get() }] }))
  return (
    <AnimatedPressable accessibilityRole={accessibilityRole} accessibilityLabel={accessibilityLabel} accessibilityState={{ disabled: !!disabled }}
      onPress={disabled ? undefined : onPress}
      onPressIn={() => { scale.set(withSpring(0.96, { damping: 15, stiffness: 400 })) }}
      onPressOut={() => { scale.set(withSpring(1, { damping: 12, stiffness: 300 })) }}
      style={[style, anim]}>
      {children}
    </AnimatedPressable>
  )
}

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost' | 'credit'

export function Button({ title, onPress, variant = 'primary', loading, disabled, icon: Icon }: {
  title: string; onPress: () => void; variant?: Variant; loading?: boolean; disabled?: boolean; icon?: LucideIcon
}) {
  const { c } = useTheme()
  const off = disabled || loading
  const bg = { primary: c.brand, secondary: c.surface, danger: c.surface, ghost: 'transparent', credit: c.credit }[variant]
  const fg = { primary: c.brandFg, secondary: c.fg, danger: c.danger, ghost: c.brandStrong, credit: '#fff' }[variant]
  const border = variant === 'secondary' ? c.lineStrong : variant === 'danger' ? c.dangerSoft : 'transparent'
  return (
    <Press onPress={onPress} disabled={off} accessibilityLabel={title}
      style={[styles.button, { backgroundColor: bg, borderColor: border, opacity: off ? 0.5 : 1 },
        variant === 'primary' && { shadowColor: c.brand, shadowOpacity: 0.35, shadowRadius: 12, shadowOffset: { width: 0, height: 6 }, elevation: 3 }]}>
      {loading ? <ActivityIndicator color={fg} /> : (
        <View style={styles.row}>
          {Icon && <Icon size={18} color={fg} strokeWidth={2.2} />}
          <Txt style={{ color: fg, fontFamily: font.semibold, fontSize: 15 }}>{title}</Txt>
        </View>
      )}
    </Press>
  )
}

export function Field({ label, hint, error, ...rest }: TextInputProps & { label: string; hint?: string; error?: string }) {
  const { c } = useTheme()
  const [focused, setFocused] = useState(false)
  return (
    <View style={{ gap: 6 }}>
      <Txt variant="label" tone="fg2">{label}</Txt>
      <TextInput accessibilityLabel={label} placeholderTextColor={c.subtle} {...rest}
        onFocus={(e) => { setFocused(true); rest.onFocus?.(e) }} onBlur={(e) => { setFocused(false); rest.onBlur?.(e) }}
        style={[styles.input, { backgroundColor: c.surface, color: c.fg, borderColor: error ? c.danger : focused ? c.brand : c.lineStrong, borderWidth: focused ? 2 : 1 }]} />
      {(error || hint) ? <Txt variant="caption" tone={error ? 'danger' : 'muted'}>{error ?? hint}</Txt> : null}
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

/** A surface; index staggers its entrance. */
export function Card({ children, title, icon: Icon, action, index = 0, style }: {
  children: ReactNode; title?: string; icon?: LucideIcon; action?: ReactNode; index?: number; style?: StyleProp<ViewStyle>
}) {
  const { c, dark } = useTheme()
  return (
    <Animated.View entering={FadeInDown.delay(80 + index * 70).duration(450)}
      style={[styles.card, { backgroundColor: c.surface, borderColor: c.line, shadowColor: c.shadow, shadowOpacity: dark ? 0 : 0.06 }, style]}>
      {(title || action) && (
        <View style={[styles.row, { justifyContent: 'space-between' }]}>
          <View style={styles.row}>
            {Icon && <View style={[styles.tileSm, { backgroundColor: c.surface2 }]}><Icon size={16} color={c.fg2} /></View>}
            {title && <Txt variant="heading">{title}</Txt>}
          </View>
          {action}
        </View>
      )}
      {children}
    </Animated.View>
  )
}

export function Toggle({ label, value, onChange, description }: { label: string; value: boolean; onChange: (v: boolean) => void; description?: string }) {
  const { c } = useTheme()
  return (
    <View style={[styles.row, { justifyContent: 'space-between', paddingVertical: 6 }]}>
      <View style={{ flex: 1 }}>
        <Txt variant="label">{label}</Txt>
        {description && <Txt variant="caption" tone="muted">{description}</Txt>}
      </View>
      <Switch accessibilityLabel={label} value={value} onValueChange={onChange} trackColor={{ true: c.brand, false: c.surface3 }} thumbColor="#fff" {...{ activeThumbColor: '#fff' }} />
    </View>
  )
}

export function Notice({ tone = 'red', children }: { tone?: 'red' | 'green' | 'amber' | 'brand'; children?: ReactNode }) {
  const { c } = useTheme()
  if (!children) return null
  const map = {
    red: [c.dangerSoft, c.danger, AlertCircle], green: [c.positiveSoft, c.positive, CheckCircle2],
    amber: [c.warningSoft, c.warning, TriangleAlert], brand: [c.brandSoft, c.brandStrong, Info],
  } as const
  const [bg, fg, Icon] = map[tone]
  return (
    <Animated.View entering={FadeInDown.duration(300)} accessibilityRole={tone === 'red' ? 'alert' : undefined} style={[styles.notice, { backgroundColor: bg }]}>
      <Icon size={16} color={fg} />
      <Txt variant="label" style={{ color: fg, flex: 1 }}>{children}</Txt>
    </Animated.View>
  )
}

export function Amount({ value, direction, style }: { value?: Money | string; direction?: 'DEBIT' | 'CREDIT'; style?: TextStyle }) {
  return (
    <Txt tone={direction === 'CREDIT' ? 'positive' : 'fg'} style={[{ fontFamily: font.semibold, fontVariant: ['tabular-nums'] }, style]}>
      {direction === 'DEBIT' ? '− ' : direction === 'CREDIT' ? '+ ' : ''}{formatBRL(value)}
    </Txt>
  )
}

const tileTones = (c: Colors) => ({
  slate: [c.surface2, c.fg2], brand: [c.brandSoft, c.brandStrong], green: [c.positiveSoft, c.positive],
  credit: [c.creditSoft, c.credit], amber: [c.warningSoft, c.warning], red: [c.dangerSoft, c.danger],
}) as const

export type Tone = keyof ReturnType<typeof tileTones>

export function IconTile({ icon: Icon, tone = 'slate', size = 40 }: { icon: LucideIcon; tone?: Tone; size?: number }) {
  const { c } = useTheme()
  const [bg, fg] = tileTones(c)[tone]
  return <View style={{ width: size, height: size, borderRadius: size * 0.36, backgroundColor: bg, alignItems: 'center', justifyContent: 'center' }}><Icon size={size * 0.45} color={fg} strokeWidth={2.2} /></View>
}

export function Row({ title, subtitle, right, onPress, icon, tone }: { title: string; subtitle?: string; right?: ReactNode; onPress?: () => void; icon?: LucideIcon; tone?: Tone }) {
  const { c } = useTheme()
  const content = (
    <View style={[styles.row, { paddingVertical: 12, gap: 12, borderBottomWidth: StyleSheet.hairlineWidth, borderColor: c.line }]}>
      {icon && <IconTile icon={icon} tone={tone} />}
      <View style={{ flex: 1 }}>
        <Txt variant="label" numberOfLines={1} style={{ fontFamily: font.semibold }}>{title}</Txt>
        {subtitle ? <Txt variant="caption" tone="muted" numberOfLines={2} style={{ marginTop: 2 }}>{subtitle}</Txt> : null}
      </View>
      {right}
    </View>
  )
  return onPress ? <Press onPress={onPress} accessibilityLabel={title}>{content}</Press> : content
}

/** Segmented control with a sliding indicator. */
export function Segmented<T extends string>({ value, onChange, options }: { value: T; onChange: (v: T) => void; options: [T, string][] }) {
  const { c } = useTheme()
  const [width, setWidth] = useState(0)
  const index = Math.max(0, options.findIndex(([id]) => id === value))
  const x = useSharedValue(0)
  const seg = width ? (width - 8) / options.length : 0
  useEffect(() => { x.set(withSpring(index * seg, { damping: 18, stiffness: 180 })) }, [index, seg, x])
  const pill = useAnimatedStyle(() => ({ transform: [{ translateX: x.get() }] }))
  return (
    <View accessibilityRole="tablist" onLayout={(e: LayoutChangeEvent) => setWidth(e.nativeEvent.layout.width)}
      style={[styles.segmented, { backgroundColor: c.surface2 }]}>
      {seg > 0 && <Animated.View style={[styles.pill, { width: seg, backgroundColor: c.surface, shadowColor: c.shadow }, pill]} />}
      {options.map(([id, label]) => (
        <Pressable key={id} accessibilityRole="tab" accessibilityState={{ selected: value === id }} onPress={() => onChange(id)} style={styles.segment}>
          <Txt variant="label" tone={value === id ? 'fg' : 'muted'} style={{ fontFamily: font.semibold }}>{label}</Txt>
        </Pressable>
      ))}
    </View>
  )
}

export function Sandbox({ children }: { children: ReactNode }) {
  const { c } = useTheme()
  return (
    <Animated.View entering={FadeInDown.delay(200).duration(450)} style={[styles.sandbox, { borderColor: c.warning + '66', backgroundColor: c.warningSoft }]}>
      <View style={styles.row}><FlaskConical size={14} color={c.warning} /><Txt variant="caption" tone="warning" style={{ fontFamily: font.bold, letterSpacing: 1 }}>AMBIENTE DE TESTES</Txt></View>
      {children}
    </Animated.View>
  )
}

export const Muted = ({ children }: { children: ReactNode }) => <Txt variant="label" tone="muted">{children}</Txt>

/** Pulsing placeholder rows while data loads. */
export function Loading({ rows = 3 }: { rows?: number }) {
  const { c } = useTheme()
  const o = useSharedValue(0.5)
  useEffect(() => { o.set(withRepeat(withSequence(withTiming(1, { duration: 700 }), withTiming(0.5, { duration: 700 })), -1)) }, [o])
  const pulse = useAnimatedStyle(() => ({ opacity: o.get() }))
  const bar = (w: number | `${number}%`, h = 12) => <View style={{ width: w, height: h, borderRadius: 6, backgroundColor: c.surface3 }} />
  return (
    <Animated.View accessibilityLabel="Carregando" style={[{ gap: 16, paddingVertical: 8 }, pulse]}>
      {Array.from({ length: rows }, (_, i) => (
        <View key={i} style={styles.row}>
          <View style={{ width: 40, height: 40, borderRadius: 20, backgroundColor: c.surface3 }} />
          <View style={{ flex: 1, gap: 8 }}>{bar('65%')}{bar('35%')}</View>
          {bar(64, 14)}
        </View>
      ))}
    </Animated.View>
  )
}

export function useStyles<T>(make: (c: Colors) => T): T {
  const { c } = useTheme()
  return useMemo(() => make(c), [c, make])
}

export const styles = StyleSheet.create({
  content: { padding: 16, paddingBottom: 56, gap: 14 },
  row: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  button: { borderRadius: radius.md, paddingVertical: 15, paddingHorizontal: 18, alignItems: 'center', borderWidth: 1 },
  input: { borderRadius: radius.md, paddingHorizontal: 14, paddingVertical: 13, fontSize: 16, fontFamily: font.regular },
  card: { borderRadius: radius.lg, padding: 18, gap: 12, borderWidth: 1, shadowRadius: 16, shadowOffset: { width: 0, height: 4 }, elevation: 1 },
  tileSm: { width: 30, height: 30, borderRadius: 9, alignItems: 'center', justifyContent: 'center' },
  notice: { borderRadius: radius.md, padding: 12, flexDirection: 'row', gap: 8, alignItems: 'flex-start' },
  segmented: { flexDirection: 'row', borderRadius: radius.md, padding: 4 },
  pill: { position: 'absolute', top: 4, bottom: 4, left: 4, borderRadius: 10, shadowOpacity: 0.08, shadowRadius: 6, elevation: 1 },
  segment: { flex: 1, paddingVertical: 9, alignItems: 'center' },
  sandbox: { borderWidth: 1, borderStyle: 'dashed', borderRadius: radius.lg, padding: 14, gap: 10 },
})
