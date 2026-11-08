<script setup lang="ts">
import { onMounted } from 'vue'
import TabBar from '@/components/TabBar.vue'
import ToastHost from '@/components/ToastHost.vue'
import { useSession } from '@/stores/session'

const session = useSession()
onMounted(session.load)
</script>

<template>
  <div class="mx-auto max-w-md pt-4 pb-24">
    <div v-if="session.error" class="card mx-4 border-danger bg-danger-soft text-danger" role="alert">
      {{ session.error }}
    </div>
    <RouterView v-else-if="session.me" />
  </div>
  <ToastHost />
  <TabBar />
</template>
