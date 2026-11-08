import { describe, expect, it } from 'vitest'
import { formatCents, formatDate, formatMonth, parseEuros, todayParis } from './format'

const n = (s: string) => s.replace(/ /g, '·').replace(/ /g, '_')

describe('formatCents', () => {
  it.each([
    [128450, '1_284,50·€'],
    [50, '0,50·€'],
    [0, '0,00·€'],
    [100000000, '1_000_000,00·€'],
    [-1500, '−15,00·€'],
  ])('%i → %s', (cents, want) => expect(n(formatCents(cents))).toBe(want))

  it('signe + pour un arrondi', () => expect(n(formatCents(50, { sign: true }))).toBe('+0,50·€'))
  it('refuse un montant non entier', () => expect(() => formatCents(1.5)).toThrow())
})

describe('dates', () => {
  it('formatDate', () => {
    expect(formatDate('2026-11-07')).toBe('7 nov. 2026')
    expect(formatDate('2027-02-01')).toBe('1 févr. 2027')
  })
  it('formatMonth', () => expect(formatMonth('2026-11')).toBe('novembre 2026'))
  it('todayParis passe minuit en heure de Paris', () => {
    expect(todayParis(new Date('2026-10-31T23:30:00Z'))).toBe('2026-11-01')
  })
})

describe('parseEuros', () => {
  it.each([
    ['12,50', 1250],
    ['12.5', 1250],
    ['12', 1200],
    ['1 284,50 €', 128450],
    ['0,01', 1],
  ])('%s → %i', (s, want) => expect(parseEuros(s)).toBe(want))
  it.each(['', 'abc', '12,345', '-3', '1,2,3'])('refuse %s', (s) => expect(parseEuros(s)).toBeNull())
})
