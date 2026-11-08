<script setup lang="ts">
// Champ de formulaire avec libellé, aide et erreur (charte : bordure line, radius-sm, 48 px).
defineProps<{ label: string; help?: string; error?: string | null; id: string }>()
const model = defineModel<string>({ default: '' })
defineOptions({ inheritAttrs: false })
</script>

<template>
  <label :for="id" class="flex flex-col gap-1">
    <span class="text-label text-ink-muted">{{ label }}</span>
    <input
      :id="id"
      v-model="model"
      v-bind="$attrs"
      class="min-h-12 rounded-[var(--radius-sm)] border bg-surface-raised px-3 text-ink"
      :class="error ? 'border-danger' : 'border-line'"
      :aria-invalid="!!error"
      :aria-describedby="`${id}-help`"
    />
    <span v-if="error || help" :id="`${id}-help`" class="text-caption" :class="error ? 'text-danger' : 'text-ink-muted'">{{ error ?? help }}</span>
  </label>
</template>
