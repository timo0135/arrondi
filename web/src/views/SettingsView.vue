<script setup lang="ts">
// E-05 Réglages : palier, multiplicateur, plafonds (section 4.1), puis accès Banque, Compte, Administration.
// Chaque changement est enregistré tout de suite ; il ne s'applique qu'aux opérations suivantes (D-08).
import { computed, onMounted, ref, watch } from 'vue'
import { api, ApiError, call, type Schemas } from '@/api/client'
import { formatCents, parseEuros } from '@/lib/format'
import { useSession } from '@/stores/session'
import { useToast } from '@/stores/toast'
import AppIcon from '@/components/AppIcon.vue'
import BottomSheet from '@/components/BottomSheet.vue'
import PageTitle from '@/components/PageTitle.vue'
import RoundingPill from '@/components/RoundingPill.vue'
import SettingsNav from '@/components/SettingsNav.vue'
import { formatMonth } from '@/lib/format'
import { useDesktop } from '@/lib/useDesktop'

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

// --- D-05 (ordinateur) : brouillon, aperçu (EF-42) et bouton « Enregistrer » ---
const isDesktop = useDesktop()
const draft = ref({ step: '', multiplier: 1, txCap: '', monthCapOn: false, monthCap: '' })
const toEuros = (c: number) => (c / 100).toFixed(2).replace('.', ',').replace(',00', '')
function resetDraft() {
  const st = settings.value
  if (!st) return
  draft.value = {
    step: toEuros(st.step_cents),
    multiplier: st.multiplier,
    txCap: toEuros(st.tx_cap_cents),
    monthCapOn: st.month_cap_cents != null,
    monthCap: toEuros(st.month_cap_cents ?? 20000),
  }
}
watch(settings, (st, old) => st && !old && resetDraft())
type Parsed = { ok: true; body: Settings } | { ok: false; field: 'step' | 'txCap' | 'monthCap'; text: string }
const parsed = computed<Parsed>(() => {
  const d = draft.value
  const check = (v: string, k: 'step' | 'txCap' | 'monthCap') => {
    const c = parseEuros(v)
    const r = rules[k]
    return c != null && c >= r.min && c <= r.max && c % r.mult === 0 ? c : null
  }
  const step = check(d.step, 'step')
  if (step == null) return { ok: false, field: 'step', text: rules.step.text }
  const tx = check(d.txCap, 'txCap')
  if (tx == null) return { ok: false, field: 'txCap', text: rules.txCap.text }
  let month: number | null = null
  if (d.monthCapOn) {
    month = check(d.monthCap, 'monthCap')
    if (month == null) return { ok: false, field: 'monthCap', text: rules.monthCap.text }
  }
  return { ok: true, body: { step_cents: step, multiplier: d.multiplier as Settings['multiplier'], tx_cap_cents: tx, month_cap_cents: month } }
})
const dirty = computed(() => {
  const p = parsed.value
  const st = settings.value
  if (!p.ok || !st) return p.ok === false
  return p.body.step_cents !== st.step_cents || p.body.multiplier !== st.multiplier || p.body.tx_cap_cents !== st.tx_cap_cents || (p.body.month_cap_cents ?? null) !== (st.month_cap_cents ?? null)
})
const preview = ref<Schemas['PreviewOut'] | null>(null)
const trial = ref('23,40')
let ptimer: ReturnType<typeof setTimeout> | undefined
watch(
  [parsed, trial, isDesktop],
  () => {
    clearTimeout(ptimer)
    const p = parsed.value
    const amount = parseEuros(trial.value)
    if (!isDesktop.value || !p.ok || !amount) return
    ptimer = setTimeout(async () => {
      try {
        preview.value = await call(
          api.GET('/settings/preview', {
            params: {
              query: {
                step_cents: p.body.step_cents,
                multiplier: p.body.multiplier,
                tx_cap_cents: p.body.tx_cap_cents,
                month_cap_cents: p.body.month_cap_cents ?? 0,
                amount_cents: amount,
              },
            },
          }),
        )
      } catch {
        preview.value = null
      }
    }, 200)
  },
  { deep: true },
)
async function saveDraft() {
  const p = parsed.value
  if (!p.ok) return
  if (await save(p.body)) {
    resetDraft()
    toast.show('Réglages enregistrés. Ils s’appliquent aux prochains paiements.')
  }
}
const stepPresets = ['1', '2', '5', '10']

function chip(active: boolean) {
  return active ? 'bg-primary text-on-primary border-primary' : 'bg-surface-raised text-ink border-line'
}
</script>

<template>
  <PageTitle title="Réglages" :subtitle="isDesktop ? 'Comment tes paiements sont arrondis.' : undefined" />

  <div v-if="isDesktop" class="flex items-start gap-6">
    <SettingsNav />
    <div class="flex min-w-0 flex-1 flex-col items-stretch gap-6 xl:flex-row xl:items-start">
    <div v-if="error" class="card flex-1 bg-danger-soft text-danger" role="alert">{{ error }}</div>
    <template v-else-if="settings">
      <form class="card flex min-w-0 flex-1 flex-col gap-6 p-6" @submit.prevent="saveDraft">
        <fieldset class="flex flex-col gap-2.5">
          <legend class="mb-2.5 text-heading">Palier</legend>
          <p class="text-ink-muted">Chaque paiement est arrondi au multiple supérieur de ce palier.</p>
          <div class="flex flex-wrap items-center gap-2">
            <button
              v-for="p in stepPresets"
              :key="p"
              type="button"
              class="min-h-11 min-w-14 rounded-full border px-4"
              :class="chip(draft.step === p)"
              :aria-pressed="draft.step === p"
              @click="draft.step = p"
            >
              {{ p }} €
            </button>
            <label class="flex items-center gap-2">
              <span class="text-ink-muted">ou</span>
              <input v-model="draft.step" inputmode="decimal" aria-label="Palier personnalisé en euros" class="min-h-11 w-24 rounded-[var(--radius-sm)] border border-line bg-surface-raised px-3 tabular-nums" />
              <span>€</span>
            </label>
          </div>
        </fieldset>
        <fieldset class="flex flex-col gap-2.5">
          <legend class="mb-2.5 text-heading">Multiplicateur</legend>
          <p class="text-ink-muted">L’arrondi est multiplié avant d’être mis de côté.</p>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="m in multipliers"
              :key="m"
              type="button"
              class="min-h-11 min-w-14 rounded-full border px-4"
              :class="chip(draft.multiplier === m)"
              :aria-pressed="draft.multiplier === m"
              @click="draft.multiplier = m"
            >
              ×{{ m }}
            </button>
          </div>
        </fieldset>
        <fieldset class="flex flex-col gap-2.5">
          <legend class="mb-2.5 text-heading">Plafonds</legend>
          <div class="grid grid-cols-2 gap-4">
            <label class="flex flex-col gap-1">
              <span class="text-label text-ink-muted">Par opération (€)</span>
              <input v-model="draft.txCap" inputmode="decimal" class="min-h-12 rounded-[var(--radius-sm)] border border-line bg-surface-raised px-3 tabular-nums" />
              <span class="text-caption text-ink-muted">{{ rules.txCap.text }}</span>
            </label>
            <div class="flex flex-col gap-1">
              <label class="flex items-center gap-2 text-label text-ink-muted">
                <input v-model="draft.monthCapOn" type="checkbox" class="size-4 accent-[var(--primary)]" />
                Par mois (€)
              </label>
              <input
                v-model="draft.monthCap"
                inputmode="decimal"
                :disabled="!draft.monthCapOn"
                aria-label="Plafond mensuel en euros"
                class="min-h-12 rounded-[var(--radius-sm)] border border-line bg-surface-raised px-3 tabular-nums disabled:opacity-50"
              />
              <span class="text-caption text-ink-muted">{{ draft.monthCapOn ? rules.monthCap.text : 'Désactivé : pas de limite par mois.' }}</span>
            </div>
          </div>
        </fieldset>
        <p v-if="!parsed.ok" class="text-danger" role="alert">{{ parsed.text }}</p>
        <div class="flex flex-wrap items-center gap-3 border-t border-surface-sunken pt-5">
          <button type="submit" class="btn-primary" :disabled="!parsed.ok || !dirty || saving">{{ saving ? 'Enregistrement…' : 'Enregistrer' }}</button>
          <button v-if="dirty" type="button" class="btn-secondary" @click="resetDraft">Annuler</button>
          <span class="text-caption text-ink-muted">S’applique aux prochains paiements, pas aux arrondis déjà calculés.</span>
        </div>
      </form>

      <aside class="flex shrink-0 flex-col gap-4 xl:w-[340px]">
        <section class="card flex flex-col gap-2 p-5">
          <h2 class="text-heading">Aperçu</h2>
          <label class="flex items-center justify-between gap-3">
            <span>Montant d’essai</span>
            <span class="flex items-center gap-1">
              <input v-model="trial" inputmode="decimal" class="min-h-10 w-24 rounded-[var(--radius-sm)] border border-line bg-surface-raised px-2 text-right tabular-nums" />
              €
            </span>
          </label>
          <dl v-if="preview && parsed.ok">
            <div class="flex justify-between gap-3 border-b border-surface-sunken py-2">
              <dt>Palier {{ draft.step }} € → {{ formatCents(preview.example.amount_cents + preview.example.base_cents) }}</dt>
              <dd class="amount">{{ formatCents(preview.example.base_cents, { sign: true }) }}</dd>
            </div>
            <div class="flex justify-between gap-3 border-b border-surface-sunken py-2">
              <dt>× {{ draft.multiplier }}</dt>
              <dd class="amount">{{ formatCents(preview.example.multiplied_cents, { sign: true }) }}</dd>
            </div>
            <div class="flex justify-between gap-3 border-b border-surface-sunken py-2">
              <dt>Plafond par opération</dt>
              <dd>{{ preview.example.limited_by ? `limité à ${formatCents(preview.example.final_cents)}` : 'non atteint' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3 pt-2">
              <dt class="font-semibold">Arrondi gardé</dt>
              <dd><RoundingPill :cents="preview.example.final_cents" /></dd>
            </div>
          </dl>
        </section>
        <section v-if="preview && parsed.ok && preview.last_month.card_payments_count > 0" class="flex flex-col gap-1.5 rounded-[var(--radius-md)] bg-primary p-5 text-on-primary">
          <h2 class="text-label">Avec ces réglages, {{ formatMonth(preview.last_month.month).split(' ')[0] }} aurait donné</h2>
          <p class="text-[32px] leading-[38px] font-bold tabular-nums">{{ formatCents(preview.last_month.saved_cents) }}</p>
          <p class="text-caption">
            Calculé sur tes {{ preview.last_month.card_payments_count }} paiements carte ; tu as réellement mis de côté
            {{ formatCents(preview.last_month.actual_saved_cents) }}.
          </p>
        </section>
      </aside>
    </template>
    <p v-else class="text-ink-muted" aria-live="polite">Chargement…</p>
    </div>
  </div>

  <template v-else>

  <div v-if="error" class="card mx-4 lg:mx-0 bg-danger-soft text-danger" role="alert">{{ error }}</div>

  <template v-else-if="settings">
    <section class="mx-4 lg:mx-0" aria-labelledby="step-label">
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

    <section class="mx-4 lg:mx-0 mt-6" aria-labelledby="mult-label">
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

    <ul class="card mx-4 lg:mx-0 mt-6 divide-y divide-surface-sunken p-0">
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
    <p class="mx-4 lg:mx-0 mt-2 text-caption text-ink-muted">Les changements s’appliquent aux prochaines opérations, pas aux arrondis déjà comptés.</p>

    <ul class="mx-4 lg:mx-0 mt-6 divide-y divide-surface-sunken border-b border-surface-sunken">
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
  </template>

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
