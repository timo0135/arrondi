<script setup lang="ts">
// Logo horizontal : repère (un seul coin arrondi + pièce) et « arrondi » en minuscules.
</script>

<template>
  <header class="flex items-center gap-2 px-4 pb-4">
    <svg width="28" height="28" viewBox="0 0 64 64" aria-hidden="true">
      <path d="M0 0H36A28 28 0 0 1 64 28V64H0Z" fill="var(--primary)" />
      <circle cx="24" cy="40" r="12" fill="var(--coin)" />
    </svg>
    <span class="text-title lowercase">arrondi</span>
  </header>
</template>
