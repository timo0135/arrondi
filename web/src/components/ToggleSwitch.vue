<script setup lang="ts">
// Bascule (charte : radius-pill, primary quand active). Zone tactile de 44 px.
defineProps<{ modelValue: boolean; label: string; disabled?: boolean }>()
defineEmits<{ 'update:modelValue': [boolean] }>()
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="modelValue"
    :aria-label="label"
    :disabled="disabled"
    class="flex min-h-11 items-center disabled:opacity-40"
    @click="$emit('update:modelValue', !modelValue)"
  >
    <span class="relative h-8 w-13 rounded-full border transition-colors" :class="modelValue ? 'border-primary bg-primary' : 'border-line bg-surface-sunken'">
      <span
        class="absolute top-1/2 size-6 -translate-y-1/2 rounded-full transition-[left]"
        :class="modelValue ? 'left-[calc(100%-1.75rem)] bg-on-primary' : 'left-1 bg-ink-muted'"
      />
    </span>
  </button>
</template>
