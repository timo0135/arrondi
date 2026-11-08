import { defineStore } from 'pinia'
import { ref } from 'vue'

/** Message bref en bas d'écran (succès ou erreur d'une action). */
export const useToast = defineStore('toast', () => {
  const message = ref<{ text: string; tone: 'ok' | 'error' } | null>(null)
  let timer: ReturnType<typeof setTimeout> | undefined
  function show(text: string, tone: 'ok' | 'error' = 'ok') {
    message.value = { text, tone }
    clearTimeout(timer)
    timer = setTimeout(() => (message.value = null), 3500)
  }
  return { message, show }
})
