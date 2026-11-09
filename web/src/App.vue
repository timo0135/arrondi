<script setup lang="ts">
import { onMounted } from 'vue'
import SideNav from '@/components/SideNav.vue'
import TabBar from '@/components/TabBar.vue'
import ToastHost from '@/components/ToastHost.vue'
import { useSession } from '@/stores/session'

const session = useSession()
onMounted(session.load)
</script>

<template>
  <!-- Téléphone : une colonne et onglets en bas. 768 px : contenu centré sur 720 px.
       1 024 px : barre latérale et contenu en grille jusqu'à 1 160 px (D-20). -->
  <div class="lg:flex">
    <SideNav v-if="session.me" class="hidden lg:flex" />
    <main class="mx-auto w-full max-w-md pt-4 pb-24 md:max-w-[720px] lg:max-w-[1240px] lg:px-10 lg:pt-8 lg:pb-12">
      <div v-if="session.error" class="card mx-4 border-danger bg-danger-soft text-danger" role="alert">
        {{ session.error }}
      </div>
      <RouterView v-else-if="session.me" />
    </main>
  </div>
  <ToastHost />
  <TabBar class="lg:hidden" />
</template>
