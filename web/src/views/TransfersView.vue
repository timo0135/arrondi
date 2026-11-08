<script setup lang="ts">
// E-03 Virements : ce qui reste à virer, historique sur 12 mois, virements enregistrés.
import { onMounted, ref } from 'vue'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { formatCents, formatDate, formatMonth, parseEuros, todayParis } from '@/lib/format'
import { useToast } from '@/stores/toast'
import BottomSheet from '@/components/BottomSheet.vue'
import PageTitle from '@/components/PageTitle.vue'
import TextField from '@/components/TextField.vue'

const toast = useToast()
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
</script>

<template>
  <PageTitle title="Virements" />
  <div v-if="error" class="card mx-4 bg-danger-soft text-danger" role="alert">{{ error }}</div>

  <template v-else-if="toTransfer !== null">
    <section class="card mx-4 flex flex-col gap-4">
      <div>
        <p class="text-label text-ink-muted">À virer sur ton épargne</p>
        <p class="amount text-amount-xl">{{ formatCents(toTransfer) }}</p>
      </div>
      <button class="btn-primary" @click="openSheet">Enregistrer un virement</button>
    </section>

    <section v-if="months.length" class="mx-4 mt-6">
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

    <section class="mx-4 mt-6">
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
