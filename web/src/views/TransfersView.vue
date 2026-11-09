<script setup lang="ts">
// E-03 Virements : ce qui reste à virer, historique sur 12 mois, virements enregistrés.
import { computed, onMounted, ref, watch } from 'vue'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { formatCents, formatDate, formatMonth, parseEuros, todayParis } from '@/lib/format'
import { useToast } from '@/stores/toast'
import { useDesktop } from '@/lib/useDesktop'
import BarChart from '@/components/BarChart.vue'
import BottomSheet from '@/components/BottomSheet.vue'
import PageTitle from '@/components/PageTitle.vue'
import TextField from '@/components/TextField.vue'

const toast = useToast()
const isDesktop = useDesktop()
const toTransfer = ref<number | null>(null)
const months = ref<Schemas['MonthOut'][]>([])
const transfers = ref<Schemas['TransferOut'][]>([])
const cursor = ref<string | null>(null)
const error = ref<string | null>(null)

async function load() {
  error.value = null
  try {
    const [d, m, t] = await Promise.all([
      call(api.GET('/dashboard')),
      call(api.GET('/months', { params: { query: { limit: 12 } } })),
      call(api.GET('/transfers', { params: { query: { limit: 20 } } })),
    ])
    toTransfer.value = d.to_transfer_cents
    months.value = m ?? []
    transfers.value = t.items ?? []
    cursor.value = t.next_cursor ?? null
  } catch (e) {
    error.value = e instanceof ApiError ? e.detail : 'Impossible de joindre le serveur.'
  }
}
onMounted(load)

async function more() {
  if (!cursor.value) return
  const t = await call(api.GET('/transfers', { params: { query: { limit: 20, cursor: cursor.value } } }))
  transfers.value.push(...(t.items ?? []))
  cursor.value = t.next_cursor ?? null
}

// --- enregistrer ---
const sheet = ref(false)
const amount = ref('')
const date = ref(todayParis())
const formError = ref<string | null>(null)
const saving = ref(false)

function openSheet() {
  amount.value = toTransfer.value ? (toTransfer.value / 100).toFixed(2).replace('.', ',') : ''
  date.value = todayParis()
  formError.value = null
  sheet.value = true
}

async function save() {
  const cents = parseEuros(amount.value)
  if (!cents || cents <= 0) return (formError.value = 'Indique un montant, par exemple 37,10.')
  if (!date.value || date.value > todayParis()) return (formError.value = 'La date ne peut pas être dans le futur.')
  saving.value = true
  try {
    await call(api.POST('/transfers', { body: { amount_cents: cents, transferred_on: date.value } }))
    sheet.value = false
    toast.show(`Virement de ${formatCents(cents)} enregistré.`)
    await load()
  } catch (e) {
    formError.value = e instanceof ApiError ? e.detail : 'Virement non enregistré.'
  } finally {
    saving.value = false
  }
}

// --- supprimer ---
const toDelete = ref<Schemas['TransferOut'] | null>(null)
async function remove() {
  if (!toDelete.value) return
  try {
    await call(api.DELETE('/transfers/{id}', { params: { path: { id: toDelete.value.id } } }))
    toDelete.value = null
    toast.show('Virement supprimé.')
    await load()
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Suppression impossible.', 'error')
  }
}

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

// D-03 (ordinateur) : graphique épargné et viré sur 6 mois, formulaire affiché en place.
const SHORT = ['janv.', 'févr.', 'mars', 'avr.', 'mai', 'juin', 'juil.', 'août', 'sept.', 'oct.', 'nov.', 'déc.']
const last6 = computed(() => [...months.value].slice(0, 6).reverse())
const chartSummary = computed(
  () => `Épargné et viré par mois, de ${formatMonth(last6.value[0]?.month ?? '')} à ${formatMonth(last6.value.at(-1)?.month ?? '')}`,
)
const totals = computed(() => months.value.reduce((a, m) => ({ saved: a.saved + m.saved_cents, sent: a.sent + m.transferred_cents }), { saved: 0, sent: 0 }))
// Sur ordinateur, le formulaire est prérempli avec le montant à virer et la date du jour.
function startInline() {
  amount.value = toTransfer.value ? (toTransfer.value / 100).toFixed(2).replace('.', ',') : ''
  date.value = todayParis()
  formError.value = null
}
watch(toTransfer, () => isDesktop.value && startInline())
async function saveInline() {
  await save()
  if (!formError.value) startInline()
}
</script>

<template>
  <template v-if="isDesktop">
    <PageTitle title="Virements" subtitle="Ce que tu as mis de côté et ce qui est déjà sur ton épargne." />
    <div v-if="error" class="card bg-danger-soft text-danger" role="alert">{{ error }}</div>
    <div v-else-if="toTransfer !== null" class="flex flex-col items-stretch gap-4 xl:flex-row xl:items-start">
      <div class="flex min-w-0 flex-1 flex-col gap-4">
        <section class="card flex flex-col gap-4 p-5">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <h2 class="text-heading">Épargné et viré, {{ last6.length }} derniers mois</h2>
            <span class="flex gap-4 text-caption text-ink-muted">
              <span class="inline-flex items-center gap-1.5"><span class="size-2.5 rounded-[3px] bg-primary" />Épargné</span>
              <span class="inline-flex items-center gap-1.5"><span class="size-2.5 rounded-[3px] bg-coin" />Viré</span>
            </span>
          </div>
          <BarChart
            v-if="last6.length"
            :labels="last6.map((m) => SHORT[Number(m.month.slice(5, 7)) - 1] ?? m.month)"
            :series="[
              { name: 'Épargné', color: 'var(--primary)', values: last6.map((m) => m.saved_cents) },
              { name: 'Viré', color: 'var(--coin)', values: last6.map((m) => m.transferred_cents) },
            ]"
            :summary="chartSummary"
          />
          <p v-else class="text-ink-muted">Le graphique apparaîtra après tes premiers arrondis.</p>
        </section>
        <section class="card p-2">
          <table class="w-full border-collapse">
            <caption class="px-3 pt-3 pb-1 text-left text-heading">Par mois</caption>
            <thead>
              <tr class="border-b border-line text-left text-label text-ink-muted">
                <th scope="col" class="px-3 py-2.5">Mois</th>
                <th scope="col" class="px-3 py-2.5 text-right">Arrondis</th>
                <th scope="col" class="px-3 py-2.5 text-right">Épargné</th>
                <th scope="col" class="px-3 py-2.5 text-right">Viré</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="m in months" :key="m.month" class="border-b border-surface-sunken">
                <td class="px-3 py-3">{{ capitalize(formatMonth(m.month)) }}</td>
                <td class="px-3 py-3 text-right tabular-nums text-ink-muted">{{ m.transactions_count }}</td>
                <td class="px-3 py-3 text-right tabular-nums">{{ formatCents(m.saved_cents) }}</td>
                <td class="px-3 py-3 text-right tabular-nums">{{ formatCents(m.transferred_cents) }}</td>
              </tr>
            </tbody>
            <tfoot>
              <tr class="font-semibold">
                <td class="px-3 py-3">Total sur {{ months.length }} mois</td>
                <td />
                <td class="px-3 py-3 text-right tabular-nums">{{ formatCents(totals.saved) }}</td>
                <td class="px-3 py-3 text-right tabular-nums">{{ formatCents(totals.sent) }}</td>
              </tr>
            </tfoot>
          </table>
        </section>
      </div>

      <div class="flex shrink-0 flex-col gap-4 xl:w-[380px]">
        <section class="card flex flex-col gap-4 p-5">
          <div>
            <p class="text-label text-ink-muted">À virer</p>
            <p class="amount text-[32px] leading-[38px] font-bold">{{ formatCents(toTransfer) }}</p>
          </div>
          <h2 class="text-heading">Enregistrer un virement</h2>
          <form class="flex flex-col gap-4" @submit.prevent="saveInline">
            <div class="grid grid-cols-2 gap-3">
              <TextField id="trd-amount" v-model="amount" label="Montant (€)" inputmode="decimal" autocomplete="off" />
              <TextField id="trd-date" v-model="date" label="Date" type="date" :max="todayParis()" />
            </div>
            <p v-if="formError" class="text-caption text-danger" role="alert">{{ formError }}</p>
            <button type="submit" class="btn-primary" :disabled="saving">{{ saving ? 'Enregistrement…' : 'Enregistrer le virement' }}</button>
            <p class="text-caption text-ink-muted">Arrondi ne fait aucun virement : fais-le depuis ta banque, puis note-le ici.</p>
          </form>
        </section>
        <section class="card flex flex-col p-5">
          <h2 class="mb-1 text-heading">Virements enregistrés</h2>
          <p v-if="!transfers.length" class="py-3 text-ink-muted">Aucun virement pour l’instant.</p>
          <ul class="divide-y divide-surface-sunken">
            <li v-for="t in transfers" :key="t.id" class="flex min-h-12 items-center justify-between gap-3 py-2">
              <span>{{ formatDate(t.transferred_on) }}</span>
              <span class="flex items-center gap-2">
                <span class="amount font-semibold">{{ formatCents(t.amount_cents) }}</span>
                <button class="min-h-9 rounded-[var(--radius-sm)] px-2 text-label text-danger hover:bg-danger-soft" :aria-label="`Supprimer le virement du ${formatDate(t.transferred_on)}`" @click="toDelete = t">
                  Supprimer
                </button>
              </span>
            </li>
          </ul>
          <button v-if="cursor" class="btn-secondary mt-3" @click="more">Afficher plus</button>
        </section>
      </div>
    </div>
    <p v-else class="text-ink-muted" aria-live="polite">Chargement…</p>
  </template>

  <template v-else>
  <PageTitle title="Virements" />
  <div v-if="error" class="card mx-4 lg:mx-0 bg-danger-soft text-danger" role="alert">{{ error }}</div>

  <template v-else-if="toTransfer !== null">
    <section class="card mx-4 lg:mx-0 flex flex-col gap-4">
      <div>
        <p class="text-label text-ink-muted">À virer sur ton épargne</p>
        <p class="amount text-amount-xl">{{ formatCents(toTransfer) }}</p>
      </div>
      <button class="btn-primary" @click="openSheet">Enregistrer un virement</button>
    </section>

    <section v-if="months.length" class="mx-4 lg:mx-0 mt-6">
      <h2 class="mb-1 text-label text-ink-muted">Historique sur 12 mois</h2>
      <ul class="divide-y divide-surface-sunken">
        <li v-for="m in months" :key="m.month" class="flex items-center justify-between gap-3 py-3">
          <div>
            <p>{{ capitalize(formatMonth(m.month)) }}</p>
            <p class="text-caption text-ink-muted">
              {{ m.transactions_count }} arrondi{{ m.transactions_count > 1 ? 's' : '' }} · viré {{ formatCents(m.transferred_cents) }}
            </p>
          </div>
          <p class="amount">{{ formatCents(m.saved_cents) }}</p>
        </li>
      </ul>
    </section>

    <section class="mx-4 lg:mx-0 mt-6">
      <h2 class="mb-1 text-label text-ink-muted">Virements enregistrés</h2>
      <p v-if="!transfers.length" class="py-3 text-ink-muted">Aucun virement pour l’instant.</p>
      <ul class="divide-y divide-surface-sunken">
        <li v-for="t in transfers" :key="t.id">
          <button class="flex min-h-14 w-full items-center justify-between gap-3 py-3 text-left" @click="toDelete = t">
            <span>{{ formatDate(t.transferred_on) }}</span>
            <span class="amount">{{ formatCents(t.amount_cents) }}</span>
          </button>
        </li>
      </ul>
      <button v-if="cursor" class="btn-secondary mt-3 w-full" @click="more">Afficher plus</button>
    </section>
  </template>
  <p v-else class="px-4 text-ink-muted" aria-live="polite">Chargement…</p>
  </template>

  <BottomSheet :open="sheet" title="Enregistrer un virement" @close="sheet = false">
    <form class="flex flex-col gap-4" @submit.prevent="save">
      <p class="text-ink-muted">Le virement se fait dans l’application de ta banque ; ici, tu le notes pour suivre ce qui reste à virer.</p>
      <TextField id="tr-amount" v-model="amount" label="Montant viré (€)" inputmode="decimal" autocomplete="off" />
      <TextField id="tr-date" v-model="date" label="Date du virement" type="date" :max="todayParis()" :error="formError" />
      <button type="submit" class="btn-primary" :disabled="saving">{{ saving ? 'Enregistrement…' : 'Enregistrer' }}</button>
    </form>
  </BottomSheet>

  <BottomSheet :open="!!toDelete" title="Supprimer ce virement ?" @close="toDelete = null">
    <p v-if="toDelete" class="mb-4">
      Virement de <span class="amount font-semibold">{{ formatCents(toDelete.amount_cents) }}</span> du {{ formatDate(toDelete.transferred_on) }}.
      Le montant reviendra dans « À virer ».
    </p>
    <button class="btn-danger w-full" @click="remove">Supprimer</button>
  </BottomSheet>
</template>
