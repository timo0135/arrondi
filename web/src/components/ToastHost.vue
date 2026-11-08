<script setup lang="ts">
import { useToast } from '@/stores/toast'
const toast = useToast()
</script>

<template>
  <div class="pointer-events-none fixed inset-x-0 bottom-20 z-20 flex justify-center px-4" aria-live="polite">
    <p
      v-if="toast.message"
      class="max-w-md rounded-[var(--radius-md)] px-4 py-3 shadow-sm"
      :class="toast.message.tone === 'error' ? 'bg-danger-soft text-danger' : 'bg-ink text-surface'"
    >
      {{ toast.message.text }}
    </p>
  </div>
</template>
