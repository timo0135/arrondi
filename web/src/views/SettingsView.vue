<script setup lang="ts">
// E-05 Réglages : palier, multiplicateur, plafonds (section 4.1), puis accès Banque, Compte, Administration.
// Chaque changement est enregistré tout de suite ; il ne s'applique qu'aux opérations suivantes (D-08).
import { computed, onMounted, ref } from 'vue'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { formatCents, parseEuros } from '@/lib/format'
import { useSession } from '@/stores/session'
import { useToast } from '@/stores/toast'
import AppIcon from '@/components/AppIcon.vue'
import BottomSheet from '@/components/BottomSheet.vue'
import PageTitle from '@/components/PageTitle.vue'

type Settings = Schemas['SettingsBody']

const session = useSession()
const toast = useToast()
const settings = ref<Settings | null>(null)
const error = ref<string | null>(null)
const saving = ref(false)

const steps = [100, 200, 500, 1000]
const multipliers = [1, 2, 3, 5, 10] as const

onMounted(async () => {
  try {
    settings.value = await call(api.GET('/settings'))
  } catch (e) {
    error.value = e instanceof ApiError ? e.detail : 'Impossible de joindre le serveur.'
  }
})

async function save(patch: Partial<Settings>): Promise<boolean> {
  if (!settings.value) return false
  const before = settings.value
  const next = { ...before, ...patch }
  settings.value = next
  saving.value = true
  try {
    settings.value = await call(api.PUT('/settings', { body: { ...next, month_cap_cents: next.month_cap_cents ?? null } }))
    return true
  } catch (e) {
    settings.value = before
    toast.show(e instanceof ApiError ? e.detail : 'Réglage non enregistré.', 'error')
    return false
  } finally {
    saving.value = false
  }
}

const customStep = computed(() => !!settings.value && !steps.includes(settings.value.step_cents))

// --- feuilles d'édition ---
type Sheet = 'step' | 'txCap' | 'monthCap' | null
const sheet = ref<Sheet>(null)
const input = ref('')
const monthCapOn = ref(false)
const inputError = ref<string | null>(null)

function open(s: Exclude<Sheet, null>) {
  if (!settings.value) return
  inputError.value = null
  const v = s === 'step' ? settings.value.step_cents : s === 'txCap' ? settings.value.tx_cap_cents : (settings.value.month_cap_cents ?? 5000)
  input.value = (v / 100).toFixed(2).replace('.', ',')
  monthCapOn.value = settings.value.month_cap_cents != null
  sheet.value = s
}

const rules = {
  step: { min: 50, max: 5000, mult: 50, text: 'Entre 0,50 € et 50,00 €, par pas de 0,50 €.' },
  txCap: { min: 100, max: 10000, mult: 100, text: 'Entre 1 € et 100 €, par pas de 1 €.' },
  monthCap: { min: 500, max: 100000, mult: 500, text: 'Entre 5 € et 1 000 €, par pas de 5 €.' },
}

async function submit() {
  const s = sheet.value
  if (!s) return
  if (s === 'monthCap' && !monthCapOn.value) {
    if (await save({ month_cap_cents: null })) sheet.value = null
    return
  }
  const cents = parseEuros(input.value)
  const r = rules[s]
  if (cents == null || cents < r.min || cents > r.max || cents % r.mult !== 0) {
    inputError.value = r.text
    return
  }
  const patch: Partial<Settings> = s === 'step' ? { step_cents: cents } : s === 'txCap' ? { tx_cap_cents: cents } : { month_cap_cents: cents }
  if (await save(patch)) sheet.value = null
}

const sheetTitle = computed(() => ({ step: 'Palier personnalisé', txCap: 'Plafond par transaction', monthCap: 'Plafond mensuel', '': '' })[sheet.value ?? ''])

function chip(active: boolean) {
  return active ? 'bg-primary text-on-primary border-primary' : 'bg-surface-raised text-ink border-line'
}
</script>

<template>
  <PageTitle title="Réglages" />

  <div v-if="error" class="card mx-4 bg-danger-soft text-danger" role="alert">{{ error }}</div>

  <template v-else-if="settings">
    <section class="mx-4" aria-labelledby="step-label">
      <h2 id="step-label" class="mb-2 text-label text-ink-muted">Palier d’arrondi</h2>
      <div class="grid grid-cols-5 gap-2">
        <button
          v-for="s in steps"
          :key="s"
          class="min-h-12 rounded-[var(--radius-sm)] border font-semibold"
          :class="chip(settings.step_cents === s)"
          :aria-pressed="settings.step_cents === s"
          :disabled="saving"
          @click="save({ step_cents: s })"
        >
          {{ s / 100 }} €
        </button>
        <button
          class="min-h-12 rounded-[var(--radius-sm)] border font-semibold"
          :class="chip(customStep)"
          :aria-pressed="customStep"
          @click="open('step')"
        >
          {{ customStep ? formatCents(settings.step_cents).replace(',00', '') : 'Autre' }}
        </button>
      </div>
    </section>

    <section class="mx-4 mt-6" aria-labelledby="mult-label">
      <h2 id="mult-label" class="mb-2 text-label text-ink-muted">Multiplicateur</h2>
      <div class="grid grid-cols-5 gap-2">
        <button
          v-for="m in multipliers"
          :key="m"
          class="min-h-12 rounded-[var(--radius-sm)] border font-semibold"
          :class="chip(settings.multiplier === m)"
          :aria-pressed="settings.multiplier === m"
          :disabled="saving"
          @click="save({ multiplier: m })"
        >
          ×{{ m }}
        </button>
      </div>
    </section>

    <ul class="card mx-4 mt-6 divide-y divide-surface-sunken p-0">
      <li>
        <button class="flex min-h-14 w-full items-center justify-between px-4" @click="open('txCap')">
          <span>Plafond par transaction</span>
          <span class="amount font-semibold">{{ formatCents(settings.tx_cap_cents) }}</span>
        </button>
      </li>
      <li>
        <button class="flex min-h-14 w-full items-center justify-between px-4" @click="open('monthCap')">
          <span>Plafond mensuel</span>
          <span v-if="settings.month_cap_cents != null" class="amount font-semibold">{{ formatCents(settings.month_cap_cents) }}</span>
          <span v-else class="text-label text-ink-muted">Désactivé</span>
        </button>
      </li>
    </ul>
    <p class="mx-4 mt-2 text-caption text-ink-muted">Les changements s’appliquent aux prochaines opérations, pas aux arrondis déjà comptés.</p>

    <ul class="mx-4 mt-6 divide-y divide-surface-sunken border-b border-surface-sunken">
      <li>
        <RouterLink to="/reglages/banque" class="flex min-h-14 items-center gap-3">
          <AppIcon name="bank" class="text-ink-muted" /><span class="flex-1">Banque</span><AppIcon name="chevron" class="text-ink-muted" />
        </RouterLink>
      </li>
      <li>
        <RouterLink to="/reglages/compte" class="flex min-h-14 items-center gap-3">
          <AppIcon name="user" class="text-ink-muted" /><span class="flex-1">Compte</span><AppIcon name="chevron" class="text-ink-muted" />
        </RouterLink>
      </li>
      <li v-if="session.isAdmin">
        <RouterLink to="/admin" class="flex min-h-14 items-center gap-3">
          <AppIcon name="shield" class="text-ink-muted" /><span class="flex-1">Administration</span><AppIcon name="chevron" class="text-ink-muted" />
        </RouterLink>
      </li>
    </ul>
  </template>

  <p v-else class="px-4 text-ink-muted" aria-live="polite">Chargement…</p>

  <BottomSheet :open="sheet !== null" :title="sheetTitle" @close="sheet = null">
    <form class="flex flex-col gap-4" @submit.prevent="submit">
      <label v-if="sheet === 'monthCap'" class="flex min-h-11 items-center justify-between">
        <span>Limiter l’épargne par mois</span>
        <input v-model="monthCapOn" type="checkbox" class="size-6 accent-[var(--primary)]" />
      </label>
      <label v-if="sheet !== 'monthCap' || monthCapOn" class="flex flex-col gap-1">
        <span class="text-label text-ink-muted">Montant en euros</span>
        <input
          v-model="input"
          inputmode="decimal"
          autocomplete="off"
          class="min-h-12 rounded-[var(--radius-sm)] border border-line bg-surface px-3 text-heading tabular-nums"
          :aria-invalid="!!inputError"
          aria-describedby="sheet-help"
        />
        <span id="sheet-help" class="text-caption" :class="inputError ? 'text-danger' : 'text-ink-muted'">
          {{ inputError ?? (sheet ? rules[sheet].text : '') }}
        </span>
      </label>
      <button type="submit" class="btn-primary" :disabled="saving">{{ saving ? 'Enregistrement…' : 'Enregistrer' }}</button>
    </form>
  </BottomSheet>
</template>
