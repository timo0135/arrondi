<script setup lang="ts">
// E-02 Opérations : transactions d'un mois et leur arrondi, groupées par jour.
import { computed, ref, watch } from 'vue'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { formatCents, formatDayHeading, formatMonth, shiftMonth, todayParis } from '@/lib/format'
import { displayLabel, exclusionLabels, operationKind } from '@/lib/labels'
import AppIcon from '@/components/AppIcon.vue'
import PageTitle from '@/components/PageTitle.vue'
import RoundingPill from '@/components/RoundingPill.vue'

type Tx = Schemas['TransactionOut']

const today = todayParis()
const currentMonth = today.slice(0, 7)
const month = ref(currentMonth)
const items = ref<Tx[]>([])
const cursor = ref<string | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

async function load(reset: boolean) {
  loading.value = true
  error.value = null
  try {
    const page = await call(
      api.GET('/transactions', {
        params: { query: { month: month.value, limit: 50, ...(reset || !cursor.value ? {} : { cursor: cursor.value }) } },
      }),
    )
    items.value = reset ? (page.items ?? []) : [...items.value, ...(page.items ?? [])]
    cursor.value = page.next_cursor ?? null
  } catch (e) {
    error.value = e instanceof ApiError ? e.detail : 'Impossible de joindre le serveur.'
  } finally {
    loading.value = false
  }
}
watch(month, () => load(true), { immediate: true })

const groups = computed(() => {
  const out: { day: string; heading: string; items: Tx[] }[] = []
  for (const t of items.value) {
    let g = out[out.length - 1]
    if (!g || g.day !== t.booked_on) {
      g = { day: t.booked_on, heading: formatDayHeading(t.booked_on, today), items: [] }
      out.push(g)
    }
    g.items.push(t)
  }
  return out
})

const monthTotal = computed(() => items.value.reduce((s, t) => s + (t.rounding?.final_cents ?? 0), 0))

function signed(t: Tx) {
  return formatCents(t.direction === 'debit' ? -t.amount_cents : t.amount_cents, { sign: true })
}
</script>

<template>
  <PageTitle title="Opérations" />

  <div class="card mx-4 flex items-center justify-between p-1">
    <button class="flex size-11 items-center justify-center text-ink" aria-label="Mois précédent" @click="month = shiftMonth(month, -1)">
      <AppIcon name="chevron" class="rotate-180" />
    </button>
    <p class="text-heading">{{ formatMonth(month) }}</p>
    <button
      class="flex size-11 items-center justify-center text-ink disabled:opacity-30"
      aria-label="Mois suivant"
      :disabled="month >= currentMonth"
      @click="month = shiftMonth(month, 1)"
    >
      <AppIcon name="chevron" />
    </button>
  </div>

  <p v-if="items.length && monthTotal > 0" class="mx-4 mt-3 text-ink-muted">
    Arrondis du mois : <span class="amount font-semibold">{{ formatCents(monthTotal) }}</span>
  </p>

  <div v-if="error" class="card mx-4 mt-4 flex flex-col items-start gap-3 bg-danger-soft text-danger" role="alert">
    {{ error }}
    <button class="btn-secondary" @click="load(true)">Réessayer</button>
  </div>

  <p v-else-if="!loading && !items.length" class="mx-4 mt-6 text-ink-muted">Aucune opération en {{ formatMonth(month) }}.</p>

  <section v-for="g in groups" :key="g.day" class="mx-4 mt-4">
    <h2 class="mb-1 text-label text-ink-muted first-letter:uppercase">{{ g.heading }}</h2>
    <ul class="divide-y divide-surface-sunken">
      <li v-for="t in g.items" :key="t.id" class="flex justify-between gap-3 py-3">
        <div class="min-w-0">
          <p class="truncate" :class="t.rounding ? 'text-ink' : 'text-ink-muted'">{{ displayLabel(t.label) }}</p>
          <p class="text-caption text-ink-muted">{{ operationKind(t.label, t.direction) }}</p>
        </div>
        <div class="flex shrink-0 flex-col items-end gap-1">
          <p class="tabular-nums" :class="t.rounding ? 'text-ink' : 'text-ink-muted'">{{ signed(t) }}</p>
          <template v-if="t.rounding">
            <RoundingPill :cents="t.rounding.final_cents" />
            <p v-if="t.rounding.limited_by" class="text-caption text-ink-muted">
              {{ t.rounding.limited_by === 'monthly' ? 'Plafond mensuel atteint' : 'Plafonné' }}
            </p>
          </template>
          <p v-else-if="t.exclusion_reason" class="text-caption text-ink-muted">
            Exclu : {{ exclusionLabels[t.exclusion_reason] ?? t.exclusion_reason }}
          </p>
        </div>
      </li>
    </ul>
  </section>

  <div class="mx-4 mt-4">
    <p v-if="loading" class="text-ink-muted" aria-live="polite">Chargement…</p>
    <button v-else-if="cursor" class="btn-secondary w-full" @click="load(false)">Afficher plus</button>
  </div>
</template>
