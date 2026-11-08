import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, call, type Schemas } from '@/api/client'

/** Utilisateur connecté (identité fournie par Cloudflare Access). */
export const useSession = defineStore('session', () => {
  const me = ref<Schemas['UserOut'] | null>(null)
  const error = ref<string | null>(null)

  async function load() {
    try {
      me.value = await call(api.GET('/me'))
      error.value = null
    } catch (e) {
      error.value = (e as Error).message
    }
  }

  const isAdmin = computed(() => me.value?.role === 'admin')
  const firstName = computed(() => me.value?.display_name.split(' ')[0] ?? '')
  return { me, error, load, isAdmin, firstName }
})
