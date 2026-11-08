import { describe, expect, it } from 'vitest'
import { displayLabel, operationKind } from './labels'
import { formatDayHeading, formatSyncTime, shiftMonth } from './format'

describe('libellés', () => {
  it.each([
    ['CARTE 04/11/26 TOTAL 4 CB*6804', 'TOTAL 4'],
    ['CARTE 02/11/26 LECLERC LANDERNEA CB*6804', 'LECLERC LANDERNEA'],
    ['CARTE 31/10/26 UBER * EATS PEN CB*6804', 'UBER * EATS PEN'],
    ['CARTE 02/11/26 E.LECLERC', 'E.LECLERC'],
    ['PRLV SEPA ORANGE SA', 'ORANGE SA'],
    ['VIR SEPA CAF DU FINISTERE', 'CAF DU FINISTERE'],
    ['100% COIFFURE', '100% COIFFURE'],
  ])('%s → %s', (raw, want) => expect(displayLabel(raw)).toBe(want))

  it('nature', () => {
    expect(operationKind('CARTE 04/11/26 TOTAL', 'debit')).toBe('Carte')
    expect(operationKind('PRLV SEPA MACIF', 'debit')).toBe('Prélèvement')
    expect(operationKind('VIR SEPA CAF', 'credit')).toBe('Virement reçu')
    expect(operationKind('VIR INST DUPONT', 'debit')).toBe('Virement')
  })
})

describe('dates relatives', () => {
  it('formatDayHeading', () => {
    expect(formatDayHeading('2026-11-08', '2026-11-08')).toBe('Aujourd’hui')
    expect(formatDayHeading('2026-11-07', '2026-11-08')).toBe('Hier')
    expect(formatDayHeading('2026-11-05', '2026-11-08')).toBe('jeudi 5 nov.')
    expect(formatDayHeading('2025-12-31', '2026-11-08')).toBe('mercredi 31 déc. 2025')
  })
  it('formatSyncTime en heure de Paris', () => {
    const now = new Date('2026-11-08T10:00:00Z')
    expect(formatSyncTime('2026-11-08T05:30:12Z', now)).toBe('aujourd’hui à 06:30')
    expect(formatSyncTime('2026-11-07T05:30:12Z', now)).toBe('hier à 06:30')
    expect(formatSyncTime('2026-11-03T05:30:12Z', now)).toBe('le 3 nov. 2026 à 06:30')
  })
  it('shiftMonth', () => {
    expect(shiftMonth('2026-11', 1)).toBe('2026-12')
    expect(shiftMonth('2026-12', 1)).toBe('2027-01')
    expect(shiftMonth('2026-01', -1)).toBe('2025-12')
  })
})
