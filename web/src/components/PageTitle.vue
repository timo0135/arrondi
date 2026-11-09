<script setup lang="ts">
import AppIcon from './AppIcon.vue'
defineProps<{ title: string; subtitle?: string; back?: string }>()
</script>

<template>
  <div class="flex flex-wrap items-end justify-between gap-4 px-4 pb-4 lg:px-0 lg:pb-6">
    <div>
      <div class="flex items-center gap-1">
        <RouterLink v-if="back" :to="back" class="-ml-3 flex size-11 items-center justify-center text-ink lg:hidden" aria-label="Retour">
          <AppIcon name="chevron" class="rotate-180" />
        </RouterLink>
        <h1 class="text-title lg:text-[28px] lg:leading-[34px]">{{ title }}</h1>
      </div>
      <p v-if="subtitle" class="mt-1 text-ink-muted">{{ subtitle }}</p>
    </div>
    <!-- Actions d'en-tête, affichées sur ordinateur seulement. -->
    <div v-if="$slots.actions" class="hidden flex-wrap items-center gap-3 lg:flex"><slot name="actions" /></div>
  </div>
</template>
