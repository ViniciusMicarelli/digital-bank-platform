import { useState } from 'react'
import { parseBRL } from '@bank/core'
import { Field } from './ui'

/** An amount input in Brazilian format; onValue gets "10.50" or null. */
export function MoneyField({ label, onValue, hint, optional }: { label: string; onValue: (v: string | null) => void; hint?: string; optional?: boolean }) {
  const [text, setText] = useState('')
  const invalid = text !== '' && parseBRL(text) === null
  return (
    <Field label={label} inputMode="decimal" placeholder="0,00" value={text} required={!optional} hint={hint}
      error={invalid ? 'Valor inválido' : undefined}
      onChange={(e) => {
        setText(e.target.value)
        onValue(parseBRL(e.target.value))
      }} />
  )
}
