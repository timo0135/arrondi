<script setup lang="ts">
// E-04 Objectif : progression, puis nom, montant cible et date facultative.
import { computed, onMounted, ref } from 'vue'
import { api, ApiError, call } from '@/api/client'
import { formatCents, parseEuros, todayParis } from '@/lib/format'
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
  <PageTitle title="Objectif" :subtitle="exists ? undefined : 'Fixe-toi un cap : Arrondi suit ta progression.'" />

  <form v-if="loaded" class="mx-4 flex flex-col gap-5" @submit.prevent="save">
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

  <BottomSheet :open="confirmDelete" title="Supprimer l’objectif ?" @close="confirmDelete = false">
    <p class="mb-4">Ton épargne n’est pas touchée : seul l’objectif disparaît de l’accueil.</p>
    <button class="btn-danger w-full" @click="remove">Supprimer</button>
  </BottomSheet>
</template>
