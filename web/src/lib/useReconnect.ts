import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api, ApiError, call } from '@/api/client'
import { useToast } from '@/stores/toast'

/** Lance l'autorisation chez la banque et y envoie le navigateur (section 9.2). */
export function useReconnect() {
  const busy = ref(false)
  const router = useRouter()
  const toast = useToast()
  async function reconnect() {
    busy.value = true
    try {
      const { url } = await call(api.POST('/bank/authorizations'))
      window.location.assign(url)
    } catch (e) {
      busy.value = false
      if (e instanceof ApiError && e.status === 404) return router.push('/reglages/banque')
      toast.show(e instanceof ApiError ? e.detail : 'Impossible de joindre le serveur.', 'error')
    }
  }
  return { busy, reconnect }
}
