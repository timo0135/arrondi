<script setup lang="ts">
// E-06 Banque. Socle : message de retour de la banque ; parcours complet à la passe 3.
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import PageTitle from '@/components/PageTitle.vue'
import ComingSoon from '@/components/ComingSoon.vue'

const route = useRoute()
const messages: Record<string, { text: string; tone: string }> = {
  ok: { text: 'Ta banque est connectée.', tone: 'bg-primary-soft text-ink' },
  refused: { text: "L'autorisation a été annulée chez ta banque.", tone: 'bg-warning-soft text-warning' },
  error: { text: "La connexion n'a pas abouti. Réessaie dans un instant.", tone: 'bg-danger-soft text-danger' },
}
const back = computed(() => messages[String(route.query.status ?? '')])
</script>

<template>
  <PageTitle title="Banque" />
  <div v-if="back" class="card mx-4 mb-4" :class="back.tone" role="status">{{ back.text }}</div>
  <ComingSoon what="La connexion de ta banque" />
</template>
