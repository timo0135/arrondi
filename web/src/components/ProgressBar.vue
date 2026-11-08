<script setup lang="ts">
// Barre de progression : piste surface-sunken, remplissage primary, 8 px.
defineProps<{ percent: number; label: string }>()
</script>

<template>
  <div
    class="h-2 overflow-hidden rounded-full bg-surface-sunken"
    role="progressbar"
    :aria-valuenow="percent"
    aria-valuemin="0"
    aria-valuemax="100"
    :aria-label="label"
  >
    <div class="h-full rounded-full bg-primary transition-[width] duration-700" :style="{ width: `${Math.min(100, Math.max(0, percent))}%` }" />
  </div>
</template>
