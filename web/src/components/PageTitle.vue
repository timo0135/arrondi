<script setup lang="ts">
import AppIcon from './AppIcon.vue'
defineProps<{ title: string; subtitle?: string; back?: string }>()
</script>

<template>
  <div class="px-4 pb-4">
    <div class="flex items-center gap-1">
      <RouterLink v-if="back" :to="back" class="-ml-3 flex size-11 items-center justify-center text-ink" aria-label="Retour">
        <AppIcon name="chevron" class="rotate-180" />
      </RouterLink>
      <h1 class="text-title">{{ title }}</h1>
    </div>
    <p v-if="subtitle" class="mt-1 text-ink-muted">{{ subtitle }}</p>
  </div>
</template>
