<script setup lang="ts">
// Détail d'une opération et calcul pas à pas de son arrondi (EF-40), avec les réglages
// en vigueur au moment du calcul (D-08 : changer les réglages ne refait pas le passé).
import { computed } from 'vue'
import type { Schemas } from '@/api/client'
import { formatCents, formatDate } from '@/lib/format'
import { exclusionLabels, operationKind } from '@/lib/labels'
import RoundingPill from './RoundingPill.vue'

const props = defineProps<{ tx: Schemas['TransactionOut'] }>()
const r = computed(() => props.tx.rounding)
const steps = computed(() => {
  const x = r.value
  if (!x) return []
  const rounded = props.tx.amount_cents + x.base_cents
  const out: [string, string][] = [
    ['Montant', formatCents(props.tx.amount_cents)],
    [`Palier ${formatCents(x.step_cents).replace(',00', '')} → ${formatCents(rounded)}`, formatCents(x.base_cents, { sign: true })],
    [`Multiplicateur × ${x.multiplier}`, formatCents(x.gross_cents, { sign: true })],
  ]
  if (x.limited_by === 'tx_cap') out.push(['Plafond par opération', `limité à ${formatCents(x.final_cents)}`])
  else if (x.limited_by === 'monthly') out.push(['Plafond mensuel', `limité à ${formatCents(x.final_cents)}`])
  else out.push(['Plafonds', 'non atteints'])
  return out
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <div>
      <p class="text-label text-ink-muted">{{ operationKind(tx.label, tx.direction) }}</p>
      <h2 class="text-heading">{{ tx.merchant }}</h2>
      <p class="text-caption text-ink-muted">{{ formatDate(tx.booked_on) }}</p>
    </div>
    <div class="flex items-center justify-between rounded-[var(--radius-md)] bg-surface p-4">
      <span class="amount text-title">{{ formatCents(tx.direction === 'debit' ? -tx.amount_cents : tx.amount_cents, { sign: true }) }}</span>
      <RoundingPill v-if="r" :cents="r.final_cents" />
    </div>
    <div v-if="r">
      <h3 class="text-label text-ink-muted">Calcul de l’arrondi</h3>
      <dl>
        <div v-for="[k, v] in steps" :key="k" class="flex justify-between gap-3 border-b border-surface-sunken py-2">
          <dt>{{ k }}</dt>
          <dd class="amount whitespace-nowrap">{{ v }}</dd>
        </div>
        <div class="flex justify-between gap-3 py-2.5">
          <dt class="font-semibold">Arrondi gardé</dt>
          <dd class="amount font-bold text-primary">{{ formatCents(r.final_cents, { sign: true }) }}</dd>
        </div>
      </dl>
    </div>
    <p v-else-if="tx.exclusion_reason" class="rounded-[var(--radius-md)] bg-surface p-4 text-ink-muted">
      Pas d’arrondi : {{ exclusionLabels[tx.exclusion_reason] ?? tx.exclusion_reason }}.
    </p>
    <div class="flex flex-col gap-1">
      <h3 class="text-label text-ink-muted">Libellé de la banque</h3>
      <p class="font-mono text-label font-normal break-all text-ink-muted">{{ tx.label }}</p>
    </div>
    <p v-if="r" class="text-caption text-ink-muted">Calculé avec les réglages en vigueur à ce moment-là : changer les réglages ne modifie pas les arrondis passés.</p>
  </div>
</template>
