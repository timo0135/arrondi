// Libellés bancaires rendus lisibles. Les libellés bruts restent intacts en base.

/** « CARTE 04/11/26 TOTAL 4 CB*6804 » → « TOTAL 4 » ; les autres libellés sont gardés tels quels. */
export function displayLabel(raw: string): string {
  const card = /^CARTE \d{2}\/\d{2}\/\d{2} (.+?)(?: CB\*\d{4})?$/.exec(raw.trim())
  if (card?.[1]) return card[1]
  return raw.replace(/^(PRLV SEPA|VIR SEPA|VIR INST|VIR) /, '').trim() || raw
}

/** Nature de l'opération, affichée sous le libellé. */
export function operationKind(raw: string, direction: string): string {
  const s = raw.trim().toUpperCase()
  if (s.startsWith('CARTE ')) return 'Carte'
  if (s.startsWith('PRLV')) return 'Prélèvement'
  if (s.startsWith('VIR')) return direction === 'credit' ? 'Virement reçu' : 'Virement'
  if (s.startsWith('RETRAIT')) return 'Retrait'
  return direction === 'credit' ? 'Crédit' : 'Débit'
}

/** Motif d'exclusion (section 4.2) en français. */
export const exclusionLabels: Record<string, string> = {
  credit: 'crédit',
  foreign_currency: 'devise étrangère',
  zero_amount: 'montant nul',
  before_activation: 'avant l’activation',
  not_card: 'pas un paiement par carte',
  cash_withdrawal: 'retrait d’espèces',
  untracked_account: 'compte non suivi',
}
