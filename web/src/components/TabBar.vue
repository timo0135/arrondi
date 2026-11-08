<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppIcon, { type IconName } from './AppIcon.vue'

const tabs: { key: string; label: string; to: string; icon: IconName }[] = [
  { key: 'accueil', label: 'Accueil', to: '/', icon: 'home' },
  { key: 'operations', label: 'Opérations', to: '/operations', icon: 'list' },
  { key: 'virements', label: 'Virements', to: '/virements', icon: 'transfer' },
  { key: 'objectif', label: 'Objectif', to: '/objectif', icon: 'target' },
  { key: 'reglages', label: 'Réglages', to: '/reglages', icon: 'settings' },
]
const route = useRoute()
const active = computed(() => route.meta.tab)
</script>

<template>
  <nav
    aria-label="Navigation"
    class="fixed inset-x-0 bottom-0 z-10 border-t border-surface-sunken bg-surface-raised"
    style="padding-bottom: env(safe-area-inset-bottom)"
  >
    <ul class="mx-auto grid max-w-md grid-cols-5">
      <li v-for="t in tabs" :key="t.key">
        <RouterLink
          :to="t.to"
          class="flex min-h-14 flex-col items-center justify-center gap-0.5 text-caption"
          :class="active === t.key ? 'text-primary font-semibold' : 'text-ink-muted'"
          :aria-current="active === t.key ? 'page' : undefined"
        >
          <AppIcon :name="t.icon" />
          <span>{{ t.label }}</span>
        </RouterLink>
      </li>
    </ul>
  </nav>
</template>
