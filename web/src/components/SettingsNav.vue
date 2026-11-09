<script setup lang="ts">
// Sous-navigation des réglages, sur ordinateur seulement (D-05 à D-07).
const items = [
  { label: 'Arrondi', to: '/reglages' },
  { label: 'Banque', to: '/reglages/banque' },
  { label: 'Mon compte', to: '/reglages/compte' },
]
</script>

<template>
  <nav aria-label="Sections des réglages" class="hidden w-52 shrink-0 flex-col gap-1 lg:flex">
    <RouterLink
      v-for="i in items"
      :key="i.to"
      :to="i.to"
      class="flex min-h-11 items-center rounded-[var(--radius-sm)] border px-3"
      exact-active-class="border-surface-sunken bg-surface-raised font-semibold text-primary"
      :class="$route.path === i.to ? '' : 'border-transparent hover:bg-surface-raised'"
      :aria-current="$route.path === i.to ? 'page' : undefined"
    >
      {{ i.label }}
    </RouterLink>
  </nav>
</template>
