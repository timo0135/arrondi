<script setup lang="ts">
// E-01 Accueil : état de la banque, total épargné, épargne du mois, objectif, dernière synchro.
import { computed, onMounted, ref } from 'vue'
import { api, ApiError, call } from '@/api/client'
import { formatCents, formatSyncTime, todayParis } from '@/lib/format'
import { useAsync } from '@/lib/useAsync'
import { useReconnect } from '@/lib/useReconnect'
import { useToast } from '@/stores/toast'
import BrandHeader from '@/components/BrandHeader.vue'
import DesktopDashboard from '@/components/DesktopDashboard.vue'
import { useTablet } from '@/lib/useDesktop'
import LoadState from '@/components/LoadState.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import RoundingPill from '@/components/RoundingPill.vue'
import StatusBanner from '@/components/StatusBanner.vue'

const dash = useAsync(() => call(api.GET('/dashboard')))
onMounted(dash.run)
const { busy, reconnect } = useReconnect()
const toast = useToast()
// Tableau de bord dès 768 px (D-20) : indicateurs sur 2 colonnes puis graphiques.
const isDesktop = useTablet()

const banner = computed(() => {
  const b = dash.data.value?.bank
  if (!b) return null
  switch (b.status) {
    case 'expired':
      return { tone: 'danger' as const, text: 'Ta connexion bancaire a expiré. Reconnecte-la pour reprendre la synchronisation.', action: 'Reconnecter' }
    case 'error':
      return { tone: 'danger' as const, text: 'La synchronisation avec ta banque a échoué. Vérifie ta connexion bancaire.', action: 'Reconnecter' }
    case 'expiring_soon':
      return {
        tone: 'warning' as const,
        text: `Ta connexion bancaire expire dans ${b.expires_in_days ?? 0} jour${(b.expires_in_days ?? 0) > 1 ? 's' : ''}. Renouvelle-la pour ne rien manquer.`,
        action: 'Renouveler',
      }
    case 'pending':
      return { tone: 'warning' as const, text: 'Ta banque n’est pas encore autorisée. Termine la connexion pour commencer à épargner.', action: 'Autoriser ma banque' }
    default:
      return null
  }
})

// « J'ai viré » : enregistre un virement du montant restant à virer, daté d'aujourd'hui.
const transferring = ref(false)
async function markTransferred() {
  const amount = dash.data.value?.to_transfer_cents ?? 0
  if (amount <= 0) return
  transferring.value = true
  try {
    await call(api.POST('/transfers', { body: { amount_cents: amount, transferred_on: todayParis() } }))
    toast.show(`Virement de ${formatCents(amount)} enregistré.`)
    await dash.run()
  } catch (e) {
    toast.show(e instanceof ApiError ? e.detail : 'Impossible d’enregistrer le virement.', 'error')
  } finally {
    transferring.value = false
  }
}
</script>

<template>
  <BrandHeader class="lg:hidden" />
  <LoadState :loading="dash.loading.value && !dash.data.value" :error="dash.error.value" @retry="dash.run">
    <StatusBanner
      v-if="isDesktop && banner && dash.data.value"
      class="mx-4 mb-6 lg:mx-0"
      :tone="banner.tone"
      :text="banner.text"
      :action="banner.action"
      :busy="busy"
      @act="reconnect"
    />
    <DesktopDashboard
      v-if="isDesktop && dash.data.value"
      :dash="dash.data.value"
      :transferring="transferring"
      @transferred="markTransferred"
      @refresh="dash.run"
    />
    <div v-else-if="dash.data.value" class="mx-4 flex flex-col gap-6">
      <StatusBanner v-if="banner" :tone="banner.tone" :text="banner.text" :action="banner.action" :busy="busy" @act="reconnect" />

      <section class="rounded-[var(--radius-md)] bg-primary p-4 text-on-primary">
        <p class="text-label">Total épargné</p>
        <p class="text-amount-xl" style="font-variant-numeric: tabular-nums">{{ formatCents(dash.data.value.total_saved_cents) }}</p>
        <p v-if="dash.data.value.transferred_cents > 0" class="mt-1 text-caption">
          dont {{ formatCents(dash.data.value.transferred_cents) }} déjà virés
        </p>
      </section>

      <section class="card flex flex-col gap-3">
        <div class="flex items-start justify-between gap-2">
          <div>
            <p class="text-label text-ink-muted">Épargne du mois</p>
            <p class="amount text-title">{{ formatCents(dash.data.value.month_saved_cents) }}</p>
          </div>
          <RoundingPill v-if="dash.data.value.month_saved_cents > 0" :cents="dash.data.value.month_saved_cents" />
        </div>
        <template v-if="dash.data.value.to_transfer_cents > 0">
          <p>
            <span class="amount font-semibold">{{ formatCents(dash.data.value.to_transfer_cents) }}</span>
            à virer sur ton épargne.
          </p>
          <button class="btn-primary w-full" :disabled="transferring" @click="markTransferred">
            {{ transferring ? 'Enregistrement…' : 'J’ai viré' }}
          </button>
        </template>
        <p v-else class="text-ink-muted">Rien à virer pour l’instant.</p>
      </section>

      <RouterLink v-if="dash.data.value.goal" to="/objectif" class="card flex flex-col gap-3">
        <div class="flex items-baseline justify-between gap-2">
          <h2 class="text-heading">{{ dash.data.value.goal.name }}</h2>
          <span class="amount text-label text-ink-muted">{{ dash.data.value.goal.progress_percent }} %</span>
        </div>
        <ProgressBar :percent="dash.data.value.goal.progress_percent" :label="`Progression vers ${dash.data.value.goal.name}`" />
        <p class="amount text-caption text-ink-muted">
          {{ formatCents(dash.data.value.total_saved_cents) }} sur {{ formatCents(dash.data.value.goal.target_cents) }}
        </p>
      </RouterLink>
      <RouterLink v-else to="/objectif" class="btn-secondary">Définir un objectif</RouterLink>

      <RouterLink v-if="!dash.data.value.bank" to="/reglages/banque" class="btn-primary">Connecter ma banque</RouterLink>
      <p v-else-if="dash.data.value.bank.last_sync_ok_at" class="text-center text-caption text-ink-muted">
        Dernière synchronisation réussie : {{ formatSyncTime(dash.data.value.bank.last_sync_ok_at) }}
      </p>
    </div>
  </LoadState>
</template>
