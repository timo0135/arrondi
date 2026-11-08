import { createRouter, createWebHistory } from 'vue-router'

// Les noms de chemins suivent les écrans E-01 à E-08 du cahier des charges.
// /reglages/banque est aussi l'adresse de retour après l'autorisation bancaire (?status=ok|refused|error).
export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'accueil', component: () => import('./views/HomeView.vue'), meta: { title: 'Accueil', tab: 'accueil' } },
    { path: '/operations', name: 'operations', component: () => import('./views/OperationsView.vue'), meta: { title: 'Opérations', tab: 'operations' } },
    { path: '/virements', name: 'virements', component: () => import('./views/TransfersView.vue'), meta: { title: 'Virements', tab: 'virements' } },
    { path: '/objectif', name: 'objectif', component: () => import('./views/GoalView.vue'), meta: { title: 'Objectif', tab: 'objectif' } },
    { path: '/reglages', name: 'reglages', component: () => import('./views/SettingsView.vue'), meta: { title: 'Réglages', tab: 'reglages' } },
    { path: '/reglages/banque', name: 'banque', component: () => import('./views/BankView.vue'), meta: { title: 'Banque', tab: 'reglages' } },
    { path: '/reglages/compte', name: 'compte', component: () => import('./views/AccountView.vue'), meta: { title: 'Mon compte', tab: 'reglages' } },
    { path: '/admin', name: 'admin', component: () => import('./views/AdminView.vue'), meta: { title: 'Administration', tab: 'reglages', admin: true } },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${String(to.meta.title)} · Arrondi` : 'Arrondi'
})

declare module 'vue-router' {
  interface RouteMeta {
    title?: string
    tab?: 'accueil' | 'operations' | 'virements' | 'objectif' | 'reglages'
    admin?: boolean
  }
}
