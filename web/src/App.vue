<script setup lang="ts">
import { onMounted } from 'vue'
import BrandHeader from '@/components/BrandHeader.vue'
import TabBar from '@/components/TabBar.vue'
import { useSession } from '@/stores/session'

const session = useSession()
onMounted(session.load)
</script>

<template>
  <div class="mx-auto max-w-md pb-24">
    <BrandHeader />
    <div v-if="session.error" class="card mx-4 border-danger bg-danger-soft text-danger" role="alert">
      {{ session.error }}
    </div>
    <RouterView v-else />
  </div>
  <TabBar />
</template>
