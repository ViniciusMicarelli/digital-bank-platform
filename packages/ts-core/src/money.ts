// Money travels as decimal strings ("1234.50"): never floats.

export interface Money {
  amount: string
  currency: string
}

/** "1234.5" → "R$ 1.234,50" */
export function formatBRL(amount: string | Money | undefined): string {
  if (amount === undefined) return '—'
  const s = typeof amount === 'string' ? amount : amount.amount
  const negative = s.startsWith('-')
  const [units, cents = ''] = (negative ? s.slice(1) : s).split('.')
  const grouped = units.replace(/\B(?=(\d{3})+(?!\d))/g, '.')
  return `${negative ? '-' : ''}R$ ${grouped},${cents.padEnd(2, '0').slice(0, 2)}`
}

/** What a person types ("1.234,5", "10", "0,99") → the API's "1234.50"; null if invalid. */
export function parseBRL(input: string): string | null {
  const s = input.replace(/\s|R\$/g, '')
  if (!/^\d{1,3}(\.\d{3})*(,\d{0,2})?$|^\d+(,\d{0,2})?$|^\d+(\.\d{1,2})?$/.test(s)) return null
  let units: string, cents: string
  if (s.includes(',')) {
    ;[units, cents] = s.split(',')
    units = units.replaceAll('.', '')
  } else if (/^\d+\.\d{1,2}$/.test(s)) {
    ;[units, cents] = s.split('.')
  } else {
    units = s.replaceAll('.', '')
    cents = ''
  }
  const value = `${BigInt(units || '0')}.${cents.padEnd(2, '0')}`
  return value === '0.00' ? null : value
}

/** Compares two decimal strings with 2 places: <0, 0, >0. */
export function compareBRL(a: string, b: string): number {
  const cents = (v: string) => BigInt(v.replace('.', ''))
  const d = cents(a) - cents(b)
  return d < 0n ? -1 : d > 0n ? 1 : 0
}

/** Adds decimal strings exactly (centavos as BigInt). */
export function addBRL(...values: string[]): string {
  const total = values.reduce((sum, v) => {
    const [u, c = ''] = v.split('.')
    const sign = u.startsWith('-') ? -1n : 1n
    return sum + sign * (BigInt(u.replace('-', '')) * 100n + BigInt(c.padEnd(2, '0').slice(0, 2)))
  }, 0n)
  const abs = total < 0n ? -total : total
  return `${total < 0n ? '-' : ''}${abs / 100n}.${String(abs % 100n).padStart(2, '0')}`
}
