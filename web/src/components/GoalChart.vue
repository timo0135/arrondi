<script setup lang="ts">
// Courbe de l'épargne cumulée, ligne de l'objectif et projection en pointillés (D-04, D-22).
import { computed } from 'vue'
import { formatCents } from '@/lib/format'

const props = defineProps<{
  labels: string[] // un libellé par mois, réel puis projeté
  actual: number[] // cumul réel, un point par mois écoulé
  projection: number[] // cumul projeté, à partir du dernier point réel ; vide sans estimation
  target: number
  summary: string
}>()

const W = 720
const H = 280
const L = 60
const B = 28
const T = 16
const pw = W - L - 16
const ph = H - B - T

const max = computed(() => Math.max(props.target * 1.1, ...props.actual, ...props.projection, 1))
const step = computed(() => {
  const raw = max.value / 4
  const pow = 10 ** Math.floor(Math.log10(raw))
  return ([1, 2, 2.5, 5, 10].find((m) => m * pow >= raw) ?? 10) * pow
})
const top = computed(() => Math.ceil(max.value / step.value) * step.value)
const ticks = computed(() => Array.from({ length: Math.round(top.value / step.value) + 1 }, (_, i) => i * step.value))
const n = computed(() => Math.max(2, props.labels.length))
const x = (i: number) => L + (i * pw) / (n.value - 1)
const y = (v: number) => T + ph - (v / top.value) * ph
const pts = (vals: number[], from = 0) => vals.map((v, i) => `${x(i + from).toFixed(1)},${y(v).toFixed(1)}`).join(' ')
const area = computed(() => {
  const a = props.actual
  if (!a.length) return ''
  return `${x(0)},${y(0)} ${pts(a)} ${x(a.length - 1)},${y(0)}`
})
const projFrom = computed(() => Math.max(0, props.actual.length - 1))
const every = computed(() => Math.ceil(props.labels.length / 9))
const euros = (c: number) => `${Math.round(c / 100).toLocaleString('fr-FR')} €`
</script>

<template>
  <svg :viewBox="`0 0 ${W} ${H}`" class="block h-auto w-full" role="img" :aria-label="summary">
    <g v-for="t in ticks" :key="t">
      <line :x1="L" :x2="W - 16" :y1="y(t)" :y2="y(t)" stroke="var(--surface-sunken)" />
      <text :x="L - 8" :y="y(t) + 4" font-size="11" fill="var(--ink-muted)" text-anchor="end">{{ euros(t) }}</text>
    </g>
    <line :x1="L" :x2="W - 16" :y1="y(target)" :y2="y(target)" stroke="var(--coin)" stroke-width="2" />
    <text :x="L + 6" :y="y(target) - 6" font-size="11" font-weight="600" fill="var(--warning)">Objectif {{ formatCents(target) }}</text>
    <polygon v-if="area" :points="area" fill="var(--primary-soft)" />
    <polyline :points="pts(actual)" fill="none" stroke="var(--primary)" stroke-width="2.5" stroke-linejoin="round" />
    <polyline v-if="projection.length" :points="pts(projection, projFrom)" fill="none" stroke="var(--primary)" stroke-width="2" stroke-dasharray="6 5" />
    <circle v-if="actual.length" :cx="x(actual.length - 1)" :cy="y(actual[actual.length - 1]!)" r="5" fill="var(--primary)">
      <title>{{ labels[actual.length - 1] }} : {{ formatCents(actual[actual.length - 1]!) }}</title>
    </circle>
    <template v-for="(l, i) in labels" :key="l + i">
      <text v-if="i % every === 0 || i === labels.length - 1" :x="x(i)" :y="H - 8" font-size="11" fill="var(--ink-muted)" text-anchor="middle">{{ l }}</text>
    </template>
  </svg>
</template>
