<script setup lang="ts">
// E-05 Réglages. Socle : accès aux sous-écrans ; réglages d'arrondi à la passe 2.
import PageTitle from '@/components/PageTitle.vue'
import ComingSoon from '@/components/ComingSoon.vue'
import AppIcon from '@/components/AppIcon.vue'
import { useSession } from '@/stores/session'

const session = useSession()
</script>

<template>
  <PageTitle title="Réglages" />
  <ComingSoon what="Le réglage du palier et des plafonds" />
  <ul class="card mx-4 mt-6 divide-y divide-surface-sunken p-0">
    <li>
      <RouterLink to="/reglages/banque" class="flex min-h-14 items-center gap-3 px-4">
        <AppIcon name="bank" class="text-ink-muted" /><span class="flex-1">Banque</span><AppIcon name="chevron" class="text-ink-muted" />
      </RouterLink>
    </li>
    <li>
      <RouterLink to="/reglages/compte" class="flex min-h-14 items-center gap-3 px-4">
        <AppIcon name="user" class="text-ink-muted" /><span class="flex-1">Mon compte</span><AppIcon name="chevron" class="text-ink-muted" />
      </RouterLink>
    </li>
    <li v-if="session.isAdmin">
      <RouterLink to="/admin" class="flex min-h-14 items-center gap-3 px-4">
        <AppIcon name="shield" class="text-ink-muted" /><span class="flex-1">Administration</span><AppIcon name="chevron" class="text-ink-muted" />
      </RouterLink>
    </li>
  </ul>
</template>
