import { onScopeDispose, ref } from 'vue'

// Mise en page ordinateur à partir de 1 024 px (D-20) ; panneau latéral de détail à partir de 1 280 px.
function media(query: string) {
  const mq = typeof window !== 'undefined' && window.matchMedia ? window.matchMedia(query) : null
  const matches = ref(mq?.matches ?? false)
  const on = (e: MediaQueryListEvent) => (matches.value = e.matches)
  mq?.addEventListener('change', on)
  onScopeDispose(() => mq?.removeEventListener('change', on))
  return matches
}

export const useDesktop = () => media('(min-width: 1024px)')
export const useWide = () => media('(min-width: 1280px)')
export const useTablet = () => media('(min-width: 768px)')
