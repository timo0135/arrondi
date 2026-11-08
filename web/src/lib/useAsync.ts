import { ref, type Ref } from 'vue'
import { ApiError } from '@/api/client'

/** État de chargement d'un écran : données, chargement, message d'erreur, relance. */
export function useAsync<T>(fn: () => Promise<T>) {
  const data = ref<T | null>(null) as Ref<T | null>
  const loading = ref(false)
  const error = ref<string | null>(null)
  const status = ref<number | null>(null)

  async function run() {
    loading.value = true
    error.value = null
    status.value = null
    try {
      data.value = await fn()
    } catch (e) {
      error.value = e instanceof ApiError ? e.detail : 'Impossible de joindre le serveur. Vérifie ta connexion.'
      status.value = e instanceof ApiError ? e.status : null
    } finally {
      loading.value = false
    }
  }
  return { data, loading, error, status, run }
}
