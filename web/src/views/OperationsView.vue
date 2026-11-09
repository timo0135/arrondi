<script setup lang="ts">
// E-02 Opérations : transactions d'un mois et leur arrondi, groupées par jour.
// D-02 (ordinateur) : totaux du mois, filtre, recherche, tableau et détail du calcul.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { formatCents, formatDayHeading, formatMonth, shiftMonth, todayParis } from '@/lib/format'
import { displayLabel, exclusionLabels, operationKind } from '@/lib/labels'
import { useDesktop, useWide } from '@/lib/useDesktop'
import AppIcon from '@/components/AppIcon.vue'
import BottomSheet from '@/components/BottomSheet.vue'
import PageTitle from '@/components/PageTitle.vue'
import RoundingPill from '@/components/RoundingPill.vue'
import TxDetail from '@/components/TxDetail.vue'

type Tx = Schemas['TransactionOut']
type Status = 'all' | 'rounded' | 'excluded'

const today = todayParis()
const currentMonth = today.slice(0, 7)
const month = ref(currentMonth)
const items = ref<Tx[]>([])
const cursor = ref<string | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)
const isDesktop = useDesktop()
const isWide = useWide()

// Filtre et recherche (EF-39) ; la recherche part 250 ms après la frappe, dès 2 caractères.
const status = ref<Status>('all')
const search = ref('')
const q = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
watch(search, (v) => {
  clearTimeout(timer)
  timer = setTimeout(() => (q.value = v.trim().length >= 2 ? v.trim() : ''), 250)
})

async function load(reset: boolean) {
  loading.value = true
  error.value = null
  try {
    const page = await call(
      api.GET('/transactions', {
        params: {
          query: {
            month: month.value,
            limit: 50,
            ...(status.value !== 'all' ? { status: status.value } : {}),
            ...(q.value ? { q: q.value } : {}),
            ...(reset || !cursor.value ? {} : { cursor: cursor.value }),
          },
        },
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
watch([month, status, q], () => load(true), { immediate: true })

// Totaux du mois (EF-38), sur ordinateur seulement.
const stats = ref<Schemas['MonthStatsOut'] | null>(null)
async function loadStats() {
  if (!isDesktop.value) return
  try {
    stats.value = await call(api.GET('/stats/month', { params: { query: { month: month.value } } }))
  } catch {
    stats.value = null
  }
}
watch([month, isDesktop], loadStats, { immediate: true })

const segments = computed<{ key: Status; label: string; n?: number }[]>(() => [
  { key: 'all', label: 'Toutes', n: stats.value?.transactions_count },
  { key: 'rounded', label: 'Arrondies', n: stats.value?.card_payments_count },
  { key: 'excluded', label: 'Exclues', n: stats.value?.excluded_count },
])
const rate = computed(() => {
  const s = stats.value
  if (!s || s.card_spending_cents <= 0) return '–'
  return `${((s.saved_cents * 1000) / s.card_spending_cents / 10).toFixed(1).replace('.', ',')} %`
})
const exportUrl = computed(() => `/api/v1/transactions/export?month=${month.value}`)

// Détail : panneau à droite dès 1 280 px, boîte de dialogue en dessous (section 8).
const selected = ref<Tx | null>(null)
watch([month, status, q], () => (selected.value = null))
const searchInput = ref<HTMLInputElement>()
function onKey(e: KeyboardEvent) {
  const typing = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement
  if (e.key === '/' && !typing && isDesktop.value) {
    e.preventDefault()
    searchInput.value?.focus()
  } else if (e.key === 'Escape' && selected.value && isWide.value) {
    selected.value = null
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

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
  <!-- ====== Ordinateur (D-02) ====== -->
  <template v-if="isDesktop">
    <PageTitle title="Opérations" subtitle="Tout ce que ta banque a comptabilisé, et l’arrondi de chaque paiement.">
      <template #actions>
        <a :href="exportUrl" download class="btn-secondary">Exporter le mois (CSV)</a>
      </template>
    </PageTitle>

    <div class="grid grid-cols-2 gap-4 xl:grid-cols-4">
      <div class="card flex flex-col gap-1 p-4">
        <span class="text-label text-ink-muted">Arrondis du mois</span>
        <span class="amount text-title">{{ stats ? formatCents(stats.saved_cents, { sign: true }) : '–' }}</span>
      </div>
      <div class="card flex flex-col gap-1 p-4">
        <span class="text-label text-ink-muted">Dépenses carte</span>
        <span class="amount text-title">{{ stats ? formatCents(stats.card_spending_cents) : '–' }}</span>
      </div>
      <div class="card flex flex-col gap-1 p-4">
        <span class="text-label text-ink-muted">Paiements arrondis</span>
        <span class="amount text-title">{{ stats ? `${stats.rounded_count} sur ${stats.card_payments_count}` : '–' }}</span>
      </div>
      <div class="card flex flex-col gap-1 p-4">
        <span class="text-label text-ink-muted">Taux d’épargne sur la carte</span>
        <span class="amount text-title">{{ rate }}</span>
      </div>
    </div>

    <div class="mt-6 flex flex-wrap items-center gap-3">
      <div class="card flex items-center gap-1 p-1">
        <button class="flex size-9 items-center justify-center rounded-[var(--radius-sm)] hover:bg-surface" aria-label="Mois précédent" @click="month = shiftMonth(month, -1)">
          <AppIcon name="chevron" class="rotate-180" :size="20" />
        </button>
        <span class="min-w-36 text-center font-semibold">{{ formatMonth(month) }}</span>
        <button
          class="flex size-9 items-center justify-center rounded-[var(--radius-sm)] hover:bg-surface disabled:opacity-30"
          aria-label="Mois suivant"
          :disabled="month >= currentMonth"
          @click="month = shiftMonth(month, 1)"
        >
          <AppIcon name="chevron" :size="20" />
        </button>
      </div>
      <div role="group" aria-label="Filtrer les opérations" class="flex gap-0.5 rounded-[12px] bg-surface-sunken p-1">
        <button
          v-for="sg in segments"
          :key="sg.key"
          class="min-h-9 rounded-[var(--radius-sm)] px-3.5"
          :class="status === sg.key ? 'bg-surface-raised font-semibold shadow-sm' : 'text-ink-muted hover:text-ink'"
          :aria-pressed="status === sg.key"
          @click="status = sg.key"
        >
          {{ sg.label }}<template v-if="sg.n != null"> · {{ sg.n }}</template>
        </button>
      </div>
      <label class="flex min-h-11 min-w-56 flex-1 items-center gap-2 rounded-[var(--radius-md)] border border-line bg-surface-raised px-3">
        <span class="sr-only">Rechercher dans les libellés</span>
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" class="shrink-0 text-ink-muted" aria-hidden="true">
          <circle cx="11" cy="11" r="7" /><path d="m20 20-4-4" />
        </svg>
        <input ref="searchInput" v-model="search" type="search" placeholder="Rechercher un libellé" class="min-w-0 flex-1 bg-transparent outline-none" />
        <kbd class="rounded border border-surface-sunken px-1.5 text-caption text-ink-muted" aria-hidden="true">/</kbd>
      </label>
    </div>

    <div class="mt-4 flex items-start gap-4">
      <section class="card min-w-0 flex-1 p-2">
        <div v-if="error" class="m-2 flex flex-col items-start gap-3 rounded-[var(--radius-md)] bg-danger-soft p-4 text-danger" role="alert">
          {{ error }}
          <button class="btn-secondary" @click="load(true)">Réessayer</button>
        </div>
        <p v-else-if="!loading && !items.length" class="p-4 text-ink-muted">
          {{ q || status !== 'all' ? 'Aucune opération ne correspond.' : `Aucune opération en ${formatMonth(month)}.` }}
        </p>
        <div v-else class="overflow-x-auto">
          <table class="w-full border-collapse">
            <caption class="sr-only">Opérations de {{ formatMonth(month) }}</caption>
            <thead>
              <tr class="border-b border-line text-left text-label text-ink-muted">
                <th scope="col" class="px-3 py-2.5">Date</th>
                <th scope="col" class="px-3 py-2.5">Libellé</th>
                <th scope="col" class="px-3 py-2.5">Type</th>
                <th scope="col" class="px-3 py-2.5 text-right">Montant</th>
                <th scope="col" class="px-3 py-2.5 text-right">Arrondi</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="t in items"
                :key="t.id"
                class="cursor-pointer border-b border-surface-sunken last:border-0"
                :class="selected?.id === t.id ? 'bg-primary-soft' : 'hover:bg-surface'"
                @click="selected = t"
              >
                <td class="px-3 py-3 whitespace-nowrap first-letter:uppercase" :class="t.rounding ? '' : 'text-ink-muted'">
                  {{ formatDayHeading(t.booked_on, today) }}
                </td>
                <td class="px-3 py-3" :class="t.rounding ? '' : 'text-ink-muted'">
                  <button class="text-left hover:underline" :aria-label="`Détail de l’opération ${t.merchant}`" @click.stop="selected = t">
                    {{ t.merchant }}
                  </button>
                </td>
                <td class="px-3 py-3 text-caption text-ink-muted">{{ operationKind(t.label, t.direction) }}</td>
                <td class="px-3 py-3 text-right whitespace-nowrap tabular-nums" :class="t.rounding ? '' : 'text-ink-muted'">{{ signed(t) }}</td>
                <td class="px-3 py-3 text-right">
                  <RoundingPill v-if="t.rounding" :cents="t.rounding.final_cents" />
                  <span v-else-if="t.exclusion_reason" class="text-caption text-ink-muted">Exclu : {{ exclusionLabels[t.exclusion_reason] }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="flex items-center justify-between px-3 pt-3 pb-1">
          <span class="text-caption text-ink-muted" aria-live="polite">
            {{ loading ? 'Chargement…' : `${items.length} opération${items.length > 1 ? 's' : ''} affichée${items.length > 1 ? 's' : ''}` }}
          </span>
          <button v-if="cursor && !loading" class="btn-secondary" @click="load(false)">Afficher plus</button>
        </div>
      </section>

      <aside v-if="isWide && selected" class="card sticky top-8 w-[340px] shrink-0 p-5" aria-label="Détail de l’opération">
        <div class="-mt-2 -mr-2 flex justify-end">
          <button class="min-h-9 px-2 text-primary" @click="selected = null">Fermer</button>
        </div>
        <TxDetail :tx="selected" />
      </aside>
    </div>

    <BottomSheet :open="!isWide && !!selected" title="Détail de l’opération" @close="selected = null">
      <TxDetail v-if="selected" :tx="selected" />
    </BottomSheet>
  </template>

  <!-- ====== Téléphone et tablette (E-02) ====== -->
  <template v-else>
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
</template>
