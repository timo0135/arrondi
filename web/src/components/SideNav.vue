<script setup lang="ts">
// Barre latérale de la version ordinateur (D-20) : navigation, état de la banque, utilisateur.
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api, call, type Schemas } from '@/api/client'
import { bankStatus } from '@/lib/bankStatus'
import { formatSyncTime } from '@/lib/format'
import { useSession } from '@/stores/session'
import AppIcon, { type IconName } from './AppIcon.vue'

const links: { key: string; label: string; to: string; icon: IconName }[] = [
  { key: 'accueil', label: 'Tableau de bord', to: '/', icon: 'home' },
  { key: 'operations', label: 'Opérations', to: '/operations', icon: 'list' },
  { key: 'virements', label: 'Virements', to: '/virements', icon: 'transfer' },
  { key: 'objectif', label: 'Objectif', to: '/objectif', icon: 'target' },
  { key: 'reglages', label: 'Réglages', to: '/reglages', icon: 'settings' },
]
const route = useRoute()
const session = useSession()
const active = computed(() => (route.meta.admin ? 'admin' : route.meta.tab))

const bank = ref<Schemas['DashboardBank'] | null>(null)
const hasBank = ref(true)
onMounted(async () => {
  try {
    const d = await call(api.GET('/dashboard'))
    bank.value = d.bank ?? null
    hasBank.value = !!d.bank
  } catch {
    /* l'état de la banque est un complément : rien à afficher en cas d'erreur */
  }
})
const short: Record<string, string> = { active: 'Connectée', expiring_soon: 'Expire bientôt', expired: 'Expirée', error: 'Erreur', pending: 'À autoriser' }
const status = computed(() => (bank.value ? { cls: bankStatus[bank.value.status]?.cls ?? '', label: short[bank.value.status] ?? bank.value.status } : null))
const roleLabel = computed(() => (session.isAdmin ? 'Administrateur' : 'Membre'))
</script>

<template>
  <aside class="sticky top-0 flex h-dvh w-60 shrink-0 flex-col gap-6 border-r border-surface-sunken bg-surface-raised px-4 py-6">
    <RouterLink to="/" class="flex items-center gap-2.5 px-2" aria-label="Arrondi, tableau de bord">
      <svg width="28" height="28" viewBox="0 0 64 64" aria-hidden="true">
        <path d="M0 0H36A28 28 0 0 1 64 28V64H0Z" fill="var(--primary)" />
        <circle cx="24" cy="40" r="12" fill="var(--coin)" />
      </svg>
      <span class="text-[20px] leading-6 font-bold tracking-tight lowercase">arrondi</span>
    </RouterLink>

    <nav aria-label="Navigation principale" class="flex flex-col gap-1">
      <RouterLink
        v-for="l in links"
        :key="l.key"
        :to="l.to"
        class="flex min-h-11 items-center gap-3 rounded-[var(--radius-sm)] px-3 hover:bg-surface"
        :class="active === l.key ? 'bg-primary-soft font-semibold text-primary hover:bg-primary-soft' : 'text-ink'"
        :aria-current="active === l.key ? 'page' : undefined"
      >
        <AppIcon :name="l.icon" :size="20" :class="active === l.key ? 'text-primary' : 'text-ink-muted'" />
        {{ l.label }}
      </RouterLink>
      <template v-if="session.isAdmin">
        <div class="mx-3 my-2 h-px bg-surface-sunken" />
        <RouterLink
          to="/admin"
          class="flex min-h-11 items-center gap-3 rounded-[var(--radius-sm)] px-3 hover:bg-surface"
          :class="active === 'admin' ? 'bg-primary-soft font-semibold text-primary hover:bg-primary-soft' : 'text-ink'"
          :aria-current="active === 'admin' ? 'page' : undefined"
        >
          <AppIcon name="shield" :size="20" :class="active === 'admin' ? 'text-primary' : 'text-ink-muted'" />
          Administration
        </RouterLink>
      </template>
    </nav>

    <div class="mt-auto flex flex-col gap-3">
      <RouterLink to="/reglages/banque" class="flex flex-col gap-1.5 rounded-[var(--radius-md)] bg-surface p-3 hover:bg-surface-sunken">
        <template v-if="status && bank">
          <span class="flex items-center justify-between gap-2">
            <span class="text-label">Ma banque</span>
            <span class="pill whitespace-nowrap" :class="status.cls">{{ status.label }}</span>
          </span>
          <span v-if="bank.last_sync_ok_at" class="text-caption text-ink-muted">Synchro réussie {{ formatSyncTime(bank.last_sync_ok_at) }}</span>
        </template>
        <span v-else-if="!hasBank" class="text-label text-primary">Connecter ma banque</span>
      </RouterLink>
      <div class="flex items-center gap-2.5 px-1">
        <span class="flex size-8 items-center justify-center rounded-full bg-primary text-label text-on-primary" aria-hidden="true">
          {{ session.firstName.charAt(0).toUpperCase() }}
        </span>
        <span class="flex min-w-0 flex-col">
          <span class="truncate text-label">{{ session.firstName }}</span>
          <span class="text-caption text-ink-muted">{{ roleLabel }}</span>
        </span>
      </div>
    </div>
  </aside>
</template>
