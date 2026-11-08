<script setup lang="ts">
defineProps<{ what: string }>()
</script>

<template>
  <div class="card mx-4 text-ink-muted">{{ what }} arrive à la prochaine étape.</div>
</template>
