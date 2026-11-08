<script setup lang="ts">
defineProps<{ title: string; subtitle?: string }>()
</script>

<template>
  <div class="px-4 pt-2 pb-4">
    <h1 class="text-title">{{ title }}</h1>
    <p v-if="subtitle" class="mt-1 text-ink-muted">{{ subtitle }}</p>
  </div>
</template>
