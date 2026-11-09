<script setup lang="ts">
// Feuille modale du bas (charte : radius-lg), sur l'élément <dialog> natif.
// Échap la ferme (comportement natif de <dialog>).
import { ref, watch } from 'vue'
const props = defineProps<{ open: boolean; title: string }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>()
watch(
  () => props.open,
  (o) => (o ? dialog.value?.showModal() : dialog.value?.close()),
)
</script>

<template>
  <dialog
    ref="dialog"
    class="m-0 mt-auto w-full max-w-none bg-transparent p-0 backdrop:bg-black/40 lg:m-auto lg:max-w-[480px]"
    @close="emit('close')"
    @click.self="emit('close')"
  >
    <!-- Sur ordinateur, la feuille devient une boîte de dialogue centrée de 480 px (section 8). -->
    <div class="mx-auto max-w-md rounded-t-[var(--radius-lg)] bg-surface-raised p-4 text-ink lg:rounded-[var(--radius-lg)] lg:p-6" style="padding-bottom: calc(16px + env(safe-area-inset-bottom))">
      <div class="mb-4 flex items-center justify-between">
        <h2 class="text-heading">{{ title }}</h2>
        <button class="min-h-11 px-2 text-primary" @click="emit('close')">Fermer</button>
      </div>
      <slot />
    </div>
  </dialog>
</template>
