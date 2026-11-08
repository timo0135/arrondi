<script setup lang="ts">
// Pastille d'arrondi de la charte : fond primary-soft, pièce jaune, montant en ink.
import { formatCents } from '@/lib/format'
defineProps<{ cents: number }>()
</script>

<template>
  <span class="amount inline-flex items-center gap-1.5 rounded-full bg-primary-soft px-2.5 py-0.5 text-label">
    <span class="size-2 rounded-full bg-coin" aria-hidden="true" />
    {{ formatCents(cents, { sign: true }) }}
  </span>
</template>
