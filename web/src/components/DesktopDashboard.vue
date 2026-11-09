<script setup lang="ts">
// D-01 Tableau de bord, version ordinateur : indicateurs, épargne par mois, chiffres du mois,
// dernières opérations et commerçants (D-20, D-21 : tous les chiffres viennent de l'API).
import { computed, onMounted, ref } from 'vue'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { formatCents, formatDate, formatMonth, formatSyncTime } from '@/lib/format'
import { exclusionLabels } from '@/lib/labels'
import { useToast } from '@/stores/toast'
import AppIcon from './AppIcon.vue'
import BarChart from './BarChart.vue'
import ProgressBar from './ProgressBar.vue'
import RoundingPill from './RoundingPill.vue'

const props = defineProps<{ dash: Schemas['DashboardOut']; transferring: boolean }>()
const emit = defineEmits<{ transferred: []; refresh: [] }>()
const toast = useToast()

const stats = ref<Schemas['MonthStatsOut'] | null>(null)
const months = ref<Schemas['MonthOut'][]>([])
const recent = ref<Schemas['TransactionOut'][]>([])
const error = ref<string | null>(null)

async function load() {
  error.value = null
  try {
    const [s, m, t] = await Promise.all([
      call(api.GET('/stats/month')),
      call(api.GET('/months', { params: { query: { limit: 12 } } })),
      call(api.GET('/transactions', { params: { query: { limit: 5 } } })),
    ])
    stats.value = s
    months.value = [...(m ?? [])].reverse()
    recent.value = t.items ?? []
  } catch (e) {
    error.value = e instanceof ApiError ? e.detail : 'Impossible de charger les statistiques.'
  }
}
onMounted(load)

const SHORT = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.']
const short = (ym: string) => SHORT[Number(ym.slice(5, 7)) - 1] ?? ym
const chartLabels = computed(() => months.value.map((m) => short(m.month)))
const chartValues = computed(() => months.value.map((m) => m.saved_cents))
const isCurrent = computed(() => months.value.at(-1)?.month === props.dash.month)
const chartSummary = computed(() => {
  if (!months.value.length) return 'Aucune épargne pour l’instant'
  const best = months.value.reduce((a, b) => (b.saved_cents > a.saved_cents ? b : a))
  return `Épargne par mois sur ${months.value.length} mois ; meilleur mois : ${formatMonth(best.month)}, ${formatCents(best.saved_cents)}`
})

const prevMonthName = computed(() => {
  const [y, m] = props.dash.month.split('-').map(Number) as [number, number]
  return formatMonth(`${m === 1 ? y - 1 : y}-${String(m === 1 ? 12 : m - 1).padStart(2, '0')}`).split(' ')[0]
})
const monthName = computed(() => formatMonth(props.dash.month).split(' ')[0])
const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

const figures = computed(() => {
  const s = stats.value
  if (!s) return []
  const rate = s.card_spending_cents > 0 ? ((s.saved_cents * 1000) / s.card_spending_cents / 10).toFixed(1).replace('.', ',') : '0'
  return [
    ['Paiements carte', String(s.card_payments_count)],
    ['Arrondis', String(s.rounded_count)],
    ['Arrondi moyen', formatCents(s.average_rounding_cents)],
    ['Opérations exclues', String(s.excluded_count)],
    ['Limités par un plafond', String(s.capped_count)],
    ['Taux d’épargne sur la carte', `${rate} %`],
  ]
})
const top = computed(() => stats.value?.top_merchants ?? [])
const topMax = computed(() => Math.max(1, ...top.value.map((m) => m.saved_cents)))
const exclusions = computed(() => Object.entries(stats.value?.excluded_by_reason ?? {}))

function signed(t: Schemas['TransactionOut']) {
  return formatCents(t.direction === 'debit' ? -t.amount_cents : t.amount_cents, { sign: true })
}

const syncing = ref(false)
async function syncNow() {
  syncing.value = true
  try {
    await call(api.POST('/bank/sync'))
    toast.show('Synchronisation lancée. Les nouvelles opérations arrivent dans quelques secondes.')
    setTimeout(() => {
      emit('refresh')
      load()
    }, 5000)
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Synchronisation impossible.', 'error')
  } finally {
    syncing.value = false
  }
}
</script>

<template>
  <div class="flex flex-col gap-6 px-4 lg:px-0">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 class="text-[28px] leading-[34px] font-bold tracking-tight">Tableau de bord</h1>
        <p class="mt-1 text-ink-muted">Ce que tu arrondis, tu le gardes.</p>
      </div>
      <div v-if="dash.bank" class="flex flex-wrap items-center gap-3">
        <span v-if="dash.bank.last_sync_ok_at" class="text-caption text-ink-muted">Dernière synchro : {{ formatSyncTime(dash.bank.last_sync_ok_at) }}</span>
        <button class="btn-secondary gap-2" :disabled="syncing" @click="syncNow">
          <AppIcon name="sync" :size="20" />{{ syncing ? 'Lancement…' : 'Synchroniser' }}
        </button>
      </div>
    </header>

    <div class="grid grid-cols-2 gap-4 xl:grid-cols-4">
      <section class="flex flex-col gap-1.5 rounded-[var(--radius-md)] bg-primary p-5 text-on-primary">
        <h2 class="text-label">Total épargné</h2>
        <p class="text-[32px] leading-[38px] font-bold tracking-tight tabular-nums">{{ formatCents(dash.total_saved_cents) }}</p>
        <p class="text-caption">dont {{ formatCents(dash.transferred_cents) }} déjà virés</p>
      </section>
      <section class="card flex flex-col gap-1.5 p-5">
        <h2 class="text-label text-ink-muted">Épargne de {{ monthName }}</h2>
        <p class="amount text-[32px] leading-[38px] font-bold tracking-tight">{{ formatCents(dash.month_saved_cents) }}</p>
        <p v-if="stats" class="text-caption text-ink-muted">
          {{ capitalize(prevMonthName ?? '') }} à la même date : {{ formatCents(stats.previous_month_same_day_cents) }}
        </p>
      </section>
      <section class="card flex flex-col gap-1.5 p-5">
        <h2 class="text-label text-ink-muted">À virer</h2>
        <p class="amount text-[32px] leading-[38px] font-bold tracking-tight">{{ formatCents(dash.to_transfer_cents) }}</p>
        <button v-if="dash.to_transfer_cents > 0" class="btn-primary mt-1 min-h-11" :disabled="transferring" @click="emit('transferred')">
          {{ transferring ? 'Enregistrement…' : 'J’ai viré' }}
        </button>
        <p v-else class="text-caption text-ink-muted">Rien à virer pour l’instant.</p>
      </section>
      <RouterLink v-if="dash.goal" to="/objectif" class="card flex flex-col gap-2 p-5 hover:border-line">
        <span class="flex items-baseline justify-between gap-2">
          <h2 class="truncate text-label text-ink-muted">{{ dash.goal.name }}</h2>
          <span class="amount text-label text-primary">{{ dash.goal.progress_percent }} %</span>
        </span>
        <span class="amount text-title">{{ formatCents(dash.total_saved_cents) }}</span>
        <ProgressBar :percent="dash.goal.progress_percent" :label="`Progression vers ${dash.goal.name}`" />
        <span class="text-caption text-ink-muted">
          sur {{ formatCents(dash.goal.target_cents) }}<template v-if="dash.goal.estimated_month"> · atteint vers {{ formatMonth(dash.goal.estimated_month) }}</template>
        </span>
      </RouterLink>
      <RouterLink v-else to="/objectif" class="card flex flex-col justify-center gap-2 p-5 hover:border-line">
        <h2 class="text-heading">Pas encore d’objectif</h2>
        <span class="text-primary">Définir un objectif</span>
      </RouterLink>
    </div>

    <div v-if="error" class="card bg-danger-soft text-danger" role="alert">{{ error }}</div>

    <div class="grid gap-4 xl:grid-cols-3">
      <section class="card flex flex-col gap-4 p-5 xl:col-span-2">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-heading">Épargne par mois</h2>
          <span class="flex flex-wrap gap-4 text-caption text-ink-muted">
            <span class="inline-flex items-center gap-1.5"><span class="size-2.5 rounded-[3px] bg-primary" />Épargne du mois</span>
            <span v-if="isCurrent" class="inline-flex items-center gap-1.5"><span class="size-2.5 rounded-[3px] border-[1.5px] border-dashed border-primary bg-primary-soft" />Mois en cours</span>
          </span>
        </div>
        <BarChart
          v-if="months.length"
          :labels="chartLabels"
          :series="[{ name: 'Épargne', color: 'var(--primary)', values: chartValues }]"
          :current="isCurrent"
          :summary="chartSummary"
        />
        <p v-else class="text-ink-muted">Le graphique apparaîtra après tes premiers arrondis.</p>
      </section>

      <section class="card flex flex-col p-5">
        <h2 class="mb-2 text-heading">{{ capitalize(monthName ?? '') }} en chiffres</h2>
        <dl>
          <div v-for="[k, v] in figures" :key="k" class="flex justify-between gap-3 border-b border-surface-sunken py-2.5">
            <dt>{{ k }}</dt>
            <dd class="amount font-semibold">{{ v }}</dd>
          </div>
        </dl>
        <p v-if="exclusions.length" class="mt-3 text-caption text-ink-muted">
          Exclues :
          {{ exclusions.map(([r, n]) => `${exclusionLabels[r] ?? r} (${n})`).join(', ') }}
        </p>
      </section>
    </div>

    <div class="grid gap-4 xl:grid-cols-3">
      <section class="card flex flex-col p-5 xl:col-span-2">
        <div class="mb-2 flex items-center justify-between">
          <h2 class="text-heading">Dernières opérations</h2>
          <RouterLink to="/operations" class="font-semibold text-primary">Tout voir</RouterLink>
        </div>
        <p v-if="!recent.length" class="text-ink-muted">Aucune opération pour l’instant.</p>
        <table v-else class="w-full border-collapse">
          <caption class="sr-only">Cinq dernières opérations</caption>
          <tbody>
            <tr v-for="t in recent" :key="t.id" class="border-b border-surface-sunken last:border-0">
              <td class="py-3 pr-2 text-caption whitespace-nowrap text-ink-muted">{{ formatDate(t.booked_on).replace(/ \d{4}$/, '') }}</td>
              <td class="px-2 py-3" :class="t.rounding ? '' : 'text-ink-muted'">{{ t.merchant }}</td>
              <td class="px-2 py-3 text-right whitespace-nowrap tabular-nums" :class="t.rounding ? '' : 'text-ink-muted'">{{ signed(t) }}</td>
              <td class="py-3 pl-2 text-right">
                <RoundingPill v-if="t.rounding" :cents="t.rounding.final_cents" />
                <span v-else-if="t.exclusion_reason" class="text-caption text-ink-muted">Exclu : {{ exclusionLabels[t.exclusion_reason] }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <section class="card flex flex-col p-5">
        <h2 class="text-heading">Où tu arrondis le plus</h2>
        <p class="mb-2 text-caption text-ink-muted">{{ capitalize(monthName ?? '') }}, par commerçant</p>
        <p v-if="stats && !top.length" class="text-ink-muted">Aucun arrondi ce mois-ci.</p>
        <ul v-else-if="stats" class="flex flex-col gap-3">
          <li v-for="m in top" :key="m.merchant" class="flex flex-col gap-1.5">
            <span class="flex justify-between gap-3">
              <span class="truncate">{{ m.merchant }}</span>
              <span class="amount font-semibold">{{ formatCents(m.saved_cents, { sign: true }) }}</span>
            </span>
            <span class="flex items-center gap-2.5">
              <span class="h-1.5 flex-1 rounded-full bg-surface-sunken">
                <span class="block h-1.5 rounded-full bg-coin" :style="{ width: `${(m.saved_cents / topMax) * 100}%` }" />
              </span>
              <span class="w-20 text-right text-caption text-ink-muted">{{ m.count }} paiement{{ m.count > 1 ? 's' : '' }}</span>
            </span>
          </li>
        </ul>
      </section>
    </div>
  </div>
</template>
