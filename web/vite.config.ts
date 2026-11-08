/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['arrondi-icon.svg', 'apple-touch-icon.png'],
      manifest: {
        name: 'Arrondi',
        short_name: 'Arrondi',
        description: 'Ce que tu arrondis, tu le gardes.',
        lang: 'fr',
        start_url: '/',
        scope: '/',
        // Obligatoire : sur iPhone, seule une PWA en standalone garde ses données au-delà de 7 jours.
        display: 'standalone',
        orientation: 'portrait',
        theme_color: '#0E6B57',
        background_color: '#F7F5F0',
        icons: [
          { src: 'arrondi-icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: 'arrondi-icon-512.png', sizes: '512x512', type: 'image/png' },
          { src: 'arrondi-icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      workbox: {
        // L'API n'est jamais mise en cache par le service worker : la base serveur reste la source de vérité.
        navigateFallbackDenylist: [/^\/api\//, /^\/healthz/],
        runtimeCaching: [],
      },
    }),
  ],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    // En dev, l'API Go tourne sur :8080 ; même origine pour le navigateur, pas de CORS.
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
  },
})
