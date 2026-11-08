<script setup lang="ts">
// Bandeau d'état de la connexion bancaire (charte : icône + texte + action, jamais la couleur seule).
import AppIcon from './AppIcon.vue'
defineProps<{ tone: 'warning' | 'danger'; text: string; action?: string; busy?: boolean }>()
defineEmits<{ act: [] }>()
</script>

<template>
  <div
    class="flex flex-col gap-3 rounded-[var(--radius-md)] border p-4"
    :class="tone === 'danger' ? 'border-danger/40 bg-danger-soft' : 'border-warning/40 bg-warning-soft'"
    role="alert"
  >
    <p class="flex gap-2" :class="tone === 'danger' ? 'text-danger' : 'text-warning'">
      <AppIcon name="alert" :size="20" class="mt-0.5 shrink-0" />
      <span class="text-ink">{{ text }}</span>
    </p>
    <button v-if="action" class="btn-primary w-full" :disabled="busy" @click="$emit('act')">{{ busy ? 'Ouverture…' : action }}</button>
  </div>
</template>
