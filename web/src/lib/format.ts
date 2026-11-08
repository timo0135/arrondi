// Formats de la charte : « 1 284,50 € » (espace fine pour les milliers,
// espace insécable avant l'euro), « +0,50 € » pour un arrondi, « 7 nov. 2026 ».

const NBSP = ' '
const NARROW_NBSP = ' '

/** Montant en centimes → « 1 284,50 € ». Calcul entier, aucun float. */
export function formatCents(cents: number, opts: { sign?: boolean } = {}): string {
  if (!Number.isInteger(cents)) throw new Error(`montant non entier : ${cents}`)
  const negative = cents < 0
  const abs = Math.abs(cents)
  const euros = Math.floor(abs / 100)
  const rest = String(abs % 100).padStart(2, '0')
  const grouped = String(euros).replace(/\B(?=(\d{3})+(?!\d))/g, NARROW_NBSP)
  const sign = negative ? '−' : opts.sign && abs > 0 ? '+' : ''
  return `${sign}${grouped},${rest}${NBSP}€`
}

const MONTHS = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.']
const MONTHS_LONG = ['janvier', 'février', 'mars', 'avril', 'mai', 'juin', 'juillet', 'août', 'septembre', 'octobre', 'novembre', 'décembre']

/** Date de calendrier « AAAA-MM-JJ » → « 7 nov. 2026 » (sans passer par un fuseau). */
export function formatDate(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso)
  if (!m) return iso
  return `${Number(m[3])} ${MONTHS[Number(m[2]) - 1]} ${m[1]}`
}

/** Mois « AAAA-MM » → « novembre 2026 ». */
export function formatMonth(ym: string): string {
  const m = /^(\d{4})-(\d{2})/.exec(ym)
  if (!m) return ym
  return `${MONTHS_LONG[Number(m[2]) - 1]} ${m[1]}`
}

/** Saisie utilisateur « 12,50 » ou « 12.5 » → centimes, ou null si invalide. */
export function parseEuros(input: string): number | null {
  const s = input.replace(/[\s  €]/g, '').replace(',', '.')
  const m = /^(\d{1,7})(?:\.(\d{1,2}))?$/.exec(s)
  if (!m) return null
  return Number(m[1]) * 100 + Number((m[2] ?? '').padEnd(2, '0'))
}

/** Date du jour en Europe/Paris, au format AAAA-MM-JJ. */
export function todayParis(now = new Date()): string {
  return new Intl.DateTimeFormat('fr-CA', { timeZone: 'Europe/Paris', year: 'numeric', month: '2-digit', day: '2-digit' }).format(now)
}
