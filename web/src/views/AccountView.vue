<script setup lang="ts">
// E-07 Compte : export des données (EF-29) et suppression du compte (EF-30).
import { ref } from 'vue'
import { api, ApiError, call } from '@/api/client'
import { useSession } from '@/stores/session'
import { useToast } from '@/stores/toast'
import PageTitle from '@/components/PageTitle.vue'
import SettingsNav from '@/components/SettingsNav.vue'
import TextField from '@/components/TextField.vue'

const session = useSession()
const toast = useToast()

const exporting = ref(false)
async function exportData() {
  exporting.value = true
  try {
    const data = await call(api.GET('/me/export'))
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `arrondi-export-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(url)
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Export impossible.', 'error')
  } finally {
    exporting.value = false
  }
}

const confirmText = ref('')
const deleting = ref(false)
const deleted = ref(false)
async function deleteAccount() {
  deleting.value = true
  try {
    await call(api.DELETE('/me', { body: { confirm: true } }))
    deleted.value = true
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Suppression impossible.', 'error')
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div v-if="deleted" class="mx-4 lg:mx-0 mt-8 flex flex-col gap-3 text-center">
    <h1 class="text-title">Compte supprimé</h1>
    <p class="text-ink-muted">Toutes tes données ont été effacées. Tu peux fermer l’application.</p>
  </div>

  <template v-else>
    <PageTitle title="Mon compte" back="/reglages" />
    <div class="lg:flex lg:items-start lg:gap-6">
    <SettingsNav />
    <div class="mx-4 flex flex-col gap-6 lg:mx-0 lg:max-w-[720px] lg:flex-1">
      <section class="card flex flex-col gap-1">
        <p class="text-heading">{{ session.me?.display_name }}</p>
        <p class="text-ink-muted">{{ session.me?.email }}</p>
      </section>

      <section class="card flex flex-col gap-3">
        <h2 class="text-heading">Mes données</h2>
        <p class="text-ink-muted">Télécharge toutes tes données dans un fichier : réglages, objectif, virements, opérations et arrondis.</p>
        <button class="btn-secondary" :disabled="exporting" @click="exportData">{{ exporting ? 'Préparation…' : 'Exporter mes données' }}</button>
      </section>

      <section class="card flex flex-col gap-3">
        <h2 class="text-heading text-danger">Supprimer mon compte</h2>
        <p class="text-ink-muted">
          Toutes tes données et ta connexion bancaire sont effacées, sans retour possible. Tape SUPPRIMER pour confirmer.
        </p>
        <TextField id="confirm-delete" v-model="confirmText" label="Confirmation" autocomplete="off" autocapitalize="characters" placeholder="SUPPRIMER" />
        <button class="btn-danger" :disabled="confirmText.trim() !== 'SUPPRIMER' || deleting" @click="deleteAccount">
          {{ deleting ? 'Suppression…' : 'Supprimer mon compte' }}
        </button>
      </section>
    </div>
    </div>
  </template>
</template>
