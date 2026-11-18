import { describe, expect, it } from 'vitest'
import { addBRL, compareBRL, formatBRL, parseBRL } from './money'

describe('money', () => {
  it('formats Brazilian style', () => {
    expect(formatBRL('1234.5')).toBe('R$ 1.234,50')
    expect(formatBRL({ amount: '1000000.00', currency: 'BRL' })).toBe('R$ 1.000.000,00')
    expect(formatBRL('-15.00')).toBe('-R$ 15,00')
    expect(formatBRL('0.05')).toBe('R$ 0,05')
  })
  it('parses what people type', () => {
    expect(parseBRL('1.234,56')).toBe('1234.56')
    expect(parseBRL('10')).toBe('10.00')
    expect(parseBRL('0,5')).toBe('0.50')
    expect(parseBRL('R$ 99,90')).toBe('99.90')
    expect(parseBRL('12.5')).toBe('12.50')
    expect(parseBRL('0')).toBeNull()
    expect(parseBRL('abc')).toBeNull()
    expect(parseBRL('1,234')).toBeNull()
  })
  it('adds exactly', () => {
    expect(addBRL('0.10', '0.20')).toBe('0.30')
    expect(addBRL('1.5', '2.05', '-0.55')).toBe('3.00')
  })
  it('compares exactly', () => {
    expect(compareBRL('10.00', '9.99')).toBe(1)
    expect(compareBRL('0.10', '0.10')).toBe(0)
  })
})
