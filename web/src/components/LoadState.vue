<script setup lang="ts">
// Chargement et erreur d'un écran ; le contenu s'affiche quand les données sont là.
import AppIcon from './AppIcon.vue'
defineProps<{ loading: boolean; error: string | null }>()
defineEmits<{ retry: [] }>()
</script>

<template>
  <div v-if="error" class="card mx-4 lg:mx-0 flex flex-col items-start gap-3 border-danger bg-danger-soft" role="alert">
    <p class="flex items-center gap-2 text-danger"><AppIcon name="alert" :size="20" />{{ error }}</p>
    <button class="btn-secondary" @click="$emit('retry')">Réessayer</button>
  </div>
  <div v-else-if="loading" class="px-4 text-ink-muted" aria-live="polite">Chargement…</div>
  <slot v-else />
</template>
