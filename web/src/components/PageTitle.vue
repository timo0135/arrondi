<script setup lang="ts">
defineProps<{ title: string; subtitle?: string; back?: string }>()
</script>

<template>
  <div class="px-4 pb-4">
    <RouterLink v-if="back" :to="back" class="mb-1 inline-flex min-h-11 items-center text-primary">‹ Retour</RouterLink>
    <h1 class="text-title">{{ title }}</h1>
    <p v-if="subtitle" class="mt-1 text-ink-muted">{{ subtitle }}</p>
  </div>
</template>
