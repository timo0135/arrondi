// État d'une connexion bancaire : libellé et couleurs (jamais la couleur seule, toujours un texte).
export const bankStatus: Record<string, { label: string; cls: string }> = {
  active: { label: 'Connexion active', cls: 'bg-primary-soft text-ink' },
  expiring_soon: { label: 'Expire bientôt', cls: 'bg-warning-soft text-warning' },
  expired: { label: 'Expirée', cls: 'bg-danger-soft text-danger' },
  error: { label: 'Erreur', cls: 'bg-danger-soft text-danger' },
  pending: { label: 'À autoriser', cls: 'bg-warning-soft text-warning' },
  none: { label: 'Aucune connexion', cls: 'bg-surface-sunken text-ink-muted' },
}

/** Banques de la version 1 (D-17) ; seules celles dont la fiche de mapping est validée sont ouvertes (D-18). */
export const banks = [
  { code: 'boursobank', name: 'BoursoBank', available: true },
  { code: 'credit-agricole', name: 'Crédit Agricole', available: false },
  { code: 'credit-mutuel', name: 'Crédit Mutuel', available: false },
  { code: 'banque-postale', name: 'La Banque Postale', available: false },
] as const

export const bankName = (code: string) => banks.find((b) => b.code === code)?.name ?? code
