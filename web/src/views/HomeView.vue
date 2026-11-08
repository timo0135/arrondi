<script setup lang="ts">
// E-01 Accueil. Version du socle : données réelles, mise en page complète à la passe 2.
import { onMounted } from 'vue'
import { api, call } from '@/api/client'
import { formatCents, formatMonth } from '@/lib/format'
import { useAsync } from '@/lib/useAsync'
import { useSession } from '@/stores/session'
import LoadState from '@/components/LoadState.vue'

const session = useSession()
const dash = useAsync(() => call(api.GET('/dashboard')))
onMounted(dash.run)
</script>

<template>
  <div class="px-4 pb-4">
    <p class="text-ink-muted">Bonjour {{ session.firstName }}</p>
  </div>
  <LoadState :loading="dash.loading.value" :error="dash.error.value" @retry="dash.run">
    <section v-if="dash.data.value" class="mx-4 flex flex-col gap-6">
      <div class="card">
        <p class="text-label text-ink-muted">Total épargné</p>
        <p class="amount text-amount-xl">{{ formatCents(dash.data.value.total_saved_cents) }}</p>
      </div>
      <div class="card">
        <p class="text-label text-ink-muted">Épargne de {{ formatMonth(dash.data.value.month) }}</p>
        <p class="amount text-title">{{ formatCents(dash.data.value.month_saved_cents) }}</p>
        <p class="mt-2 text-ink-muted">
          À virer : <span class="amount">{{ formatCents(dash.data.value.to_transfer_cents) }}</span>
        </p>
      </div>
      <RouterLink v-if="!dash.data.value.bank" to="/reglages/banque" class="btn-primary">Connecter ma banque</RouterLink>
    </section>
  </LoadState>
</template>
