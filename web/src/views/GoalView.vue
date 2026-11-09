<script setup lang="ts">
// E-04 Objectif : progression, puis nom, montant cible et date facultative.
import { computed, onMounted, ref } from 'vue'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { formatCents, formatDate, formatMonth, parseEuros, shiftMonth, todayParis } from '@/lib/format'
import { useDesktop } from '@/lib/useDesktop'
import GoalChart from '@/components/GoalChart.vue'
import { useToast } from '@/stores/toast'
import BottomSheet from '@/components/BottomSheet.vue'
import PageTitle from '@/components/PageTitle.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import TextField from '@/components/TextField.vue'

const toast = useToast()
const loaded = ref(false)
const exists = ref(false)
const saved = ref(0)
const name = ref('')
const target = ref('')
const date = ref('')
const errors = ref<{ name?: string; target?: string; form?: string }>({})
const saving = ref(false)
const confirmDelete = ref(false)
const isDesktop = useDesktop()

// D-04 (ordinateur) : projection de l'API (EF-41) et courbe de l'épargne cumulée.
const goal = ref<Schemas['GoalOut'] | null>(null)
const months = ref<Schemas['MonthOut'][]>([])
async function loadDesktop() {
  if (!isDesktop.value) return
  const [m, g] = await Promise.all([
    call(api.GET('/months', { params: { query: { limit: 24 } } })),
    exists.value ? call(api.GET('/goal')) : Promise.resolve(null),
  ])
  months.value = [...(m ?? [])].reverse()
  goal.value = g
}
const SHORT = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.']
const short = (ym: string) => SHORT[Number(ym.slice(5, 7)) - 1] ?? ym
const chart = computed(() => {
  const ms = months.value
  if (!ms.length) return null
  const sum = ms.reduce((a, m) => a + m.saved_cents, 0)
  let run = saved.value - sum // épargne plus ancienne que les mois renvoyés
  const actual = ms.map((m) => (run += m.saved_cents))
  const labels = ms.map((m) => short(m.month))
  const projection: number[] = []
  const g = goal.value
  if (g?.monthly_pace_cents && g.estimated_month) {
    let ym = ms.at(-1)!.month
    let v = actual.at(-1)!
    projection.push(v)
    while (ym < g.estimated_month && projection.length < 25) {
      ym = shiftMonth(ym, 1)
      v += g.monthly_pace_cents
      projection.push(v)
      labels.push(short(ym))
    }
  }
  return { labels, actual, projection }
})
const ahead = computed(() => {
  const g = goal.value
  if (!g?.estimated_month || !g.target_date) return null
  const est = g.estimated_month
  const tgt = g.target_date.slice(0, 7)
  if (est === tgt) return 'pile à ta date cible'
  const diff = (Number(tgt.slice(0, 4)) - Number(est.slice(0, 4))) * 12 + Number(tgt.slice(5, 7)) - Number(est.slice(5, 7))
  return diff > 0 ? `${diff} mois avant ta date cible` : `${-diff} mois après ta date cible`
})
const chartSummary = computed(() => {
  const g = goal.value
  const base = `Épargne cumulée : ${formatCents(saved.value)} sur ${formatCents(targetCents.value ?? 0)}`
  return g?.estimated_month ? `${base} ; objectif atteint vers ${formatMonth(g.estimated_month)} au rythme actuel` : base
})

async function load() {
  const d = await call(api.GET('/dashboard'))
  saved.value = d.total_saved_cents
  exists.value = !!d.goal
  if (d.goal) {
    name.value = d.goal.name
    target.value = (d.goal.target_cents / 100).toFixed(2).replace('.', ',')
    date.value = d.goal.target_date ?? ''
  }
  loaded.value = true
  await loadDesktop()
}
onMounted(() => load().catch(() => ((errors.value = { form: 'Impossible de joindre le serveur.' }), (loaded.value = true))))

const targetCents = computed(() => parseEuros(target.value))
const percent = computed(() => (targetCents.value ? Math.min(100, Math.floor((saved.value * 100) / targetCents.value)) : 0))

async function save() {
  errors.value = {}
  if (!name.value.trim()) errors.value.name = 'Donne un nom à ton objectif.'
  if (!targetCents.value || targetCents.value <= 0) errors.value.target = 'Indique un montant, par exemple 2 000.'
  if (errors.value.name || errors.value.target) return
  saving.value = true
  try {
    await call(api.PUT('/goal', { body: { name: name.value.trim(), target_cents: targetCents.value!, target_date: date.value || null } }))
    exists.value = true
    toast.show('Objectif enregistré.')
    await loadDesktop()
  } catch (e) {
    errors.value.form = e instanceof ApiError ? e.detail : 'Objectif non enregistré.'
  } finally {
    saving.value = false
  }
}

async function remove() {
  try {
    await call(api.DELETE('/goal'))
    confirmDelete.value = false
    exists.value = false
    name.value = target.value = date.value = ''
    toast.show('Objectif supprimé.')
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Suppression impossible.', 'error')
  }
}
</script>

<template>
  <template v-if="isDesktop && loaded">
    <PageTitle title="Objectif" subtitle="Ton épargne, ton rythme, et quand tu y arriveras." />
    <div v-if="exists && targetCents" class="mb-4 grid grid-cols-2 gap-4 xl:grid-cols-4">
      <section class="flex flex-col gap-1.5 rounded-[var(--radius-md)] bg-primary p-5 text-on-primary">
        <h2 class="text-label">Épargné</h2>
        <p class="text-[32px] leading-[38px] font-bold tabular-nums">{{ formatCents(saved) }}</p>
        <p class="text-caption">sur {{ formatCents(targetCents) }} · {{ percent }} %</p>
      </section>
      <section class="card flex flex-col gap-1.5 p-5">
        <h2 class="text-label text-ink-muted">Reste à épargner</h2>
        <p class="amount text-[32px] leading-[38px] font-bold">{{ formatCents(Math.max(0, targetCents - saved)) }}</p>
      </section>
      <section class="card flex flex-col gap-1.5 p-5">
        <h2 class="text-label text-ink-muted">Rythme</h2>
        <p class="amount text-[32px] leading-[38px] font-bold">{{ goal?.monthly_pace_cents != null ? formatCents(goal.monthly_pace_cents) : '–' }}</p>
        <p class="text-caption text-ink-muted">{{ goal?.monthly_pace_cents != null ? 'par mois, moyenne des 3 derniers mois complets' : 'disponible après un mois complet' }}</p>
      </section>
      <section class="card flex flex-col gap-1.5 p-5">
        <h2 class="text-label text-ink-muted">Date estimée</h2>
        <p class="text-[32px] leading-[38px] font-bold first-letter:uppercase">{{ goal?.estimated_month ? formatMonth(goal.estimated_month) : 'Pas d’estimation' }}</p>
        <p v-if="ahead" class="text-caption text-ink-muted">{{ ahead }}</p>
      </section>
    </div>
    <div class="flex flex-col items-stretch gap-4 xl:flex-row xl:items-start">
      <section v-if="exists && targetCents" class="card flex min-w-0 flex-1 flex-col gap-4 p-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-heading">Vers ton objectif</h2>
          <span class="flex gap-4 text-caption text-ink-muted">
            <span class="inline-flex items-center gap-1.5"><span class="h-[3px] w-4 bg-primary" />Épargne cumulée</span>
            <span v-if="chart?.projection.length" class="inline-flex items-center gap-1.5"><span class="w-4 border-t-2 border-dashed border-primary" />Projection au rythme actuel</span>
          </span>
        </div>
        <GoalChart v-if="chart" :labels="chart.labels" :actual="chart.actual" :projection="chart.projection" :target="targetCents" :summary="chartSummary" />
        <p v-else class="text-ink-muted">La courbe apparaîtra après tes premiers arrondis.</p>
        <ProgressBar :percent="percent" label="Progression vers l’objectif" />
      </section>
      <section v-else class="card flex min-w-0 flex-1 flex-col gap-2 p-5">
        <h2 class="text-heading">Fixe-toi un cap</h2>
        <p class="text-ink-muted">Donne un nom et un montant à ton objectif : Arrondi suit ta progression et estime quand tu l’atteindras.</p>
      </section>
      <form class="card flex shrink-0 flex-col gap-4 p-5 xl:w-[360px]" @submit.prevent="save">
        <h2 class="text-heading">Ton objectif</h2>
        <TextField id="goald-name" v-model="name" label="Nom" maxlength="60" placeholder="Vacances en Grèce" :error="errors.name" />
        <TextField id="goald-target" v-model="target" label="Montant cible (€)" inputmode="decimal" autocomplete="off" placeholder="2 000" :error="errors.target" />
        <TextField id="goald-date" v-model="date" label="Date cible (facultative)" type="date" :min="todayParis()" help="Sert à comparer avec la date estimée." />
        <div v-if="errors.form" class="rounded-[var(--radius-sm)] bg-danger-soft p-3 text-danger" role="alert">{{ errors.form }}</div>
        <button type="submit" class="btn-primary" :disabled="saving">{{ saving ? 'Enregistrement…' : 'Enregistrer' }}</button>
        <button v-if="exists" type="button" class="link-danger" @click="confirmDelete = true">Supprimer l’objectif</button>
        <p v-if="goal?.target_date" class="sr-only">Date cible : {{ formatDate(goal.target_date) }}</p>
      </form>
    </div>
  </template>

  <template v-else>
  <PageTitle title="Objectif" :subtitle="exists ? undefined : 'Fixe-toi un cap : Arrondi suit ta progression.'" />

  <form v-if="loaded" class="mx-4 lg:mx-0 flex flex-col gap-5" @submit.prevent="save">
    <section v-if="exists && targetCents" class="card flex flex-col gap-3">
      <div class="flex items-baseline justify-between">
        <h2 class="text-heading">Progression</h2>
        <span class="amount text-label text-ink-muted">{{ percent }} %</span>
      </div>
      <ProgressBar :percent="percent" label="Progression vers l’objectif" />
      <p class="amount text-caption text-ink-muted">{{ formatCents(saved) }} sur {{ formatCents(targetCents) }}</p>
    </section>

    <TextField id="goal-name" v-model="name" label="Nom de l’objectif" maxlength="60" placeholder="Vacances en Grèce" :error="errors.name" />
    <TextField id="goal-target" v-model="target" label="Montant cible (€)" inputmode="decimal" autocomplete="off" placeholder="2 000" :error="errors.target" />
    <TextField id="goal-date" v-model="date" label="Date cible" type="date" :min="todayParis()" help="Facultative" />

    <div v-if="errors.form" class="card bg-danger-soft text-danger" role="alert">{{ errors.form }}</div>
    <button type="submit" class="btn-primary" :disabled="saving">{{ saving ? 'Enregistrement…' : 'Enregistrer' }}</button>
    <button v-if="exists" type="button" class="link-danger" @click="confirmDelete = true">Supprimer l’objectif</button>
  </form>
  <p v-else class="px-4 text-ink-muted" aria-live="polite">Chargement…</p>
  </template>

  <BottomSheet :open="confirmDelete" title="Supprimer l’objectif ?" @close="confirmDelete = false">
    <p class="mb-4">Ton épargne n’est pas touchée : seul l’objectif disparaît de l’accueil.</p>
    <button class="btn-danger w-full" @click="remove">Supprimer</button>
  </BottomSheet>
</template>
