<script setup lang="ts">
// Barres groupées en SVG, sans bibliothèque (D-22). La dernière colonne peut être
// marquée « en cours » (contour pointillé). Chaque barre a sa valeur exacte en info-bulle,
// au survol comme au clavier (focus).
import { computed, ref } from 'vue'
import { formatCents } from '@/lib/format'

const props = defineProps<{
  labels: string[]
  series: { name: string; color: string; values: number[] }[]
  current?: boolean
  summary: string
}>()

const W = 720
const H = 260
const L = 52
const B = 28
const T = 14
const pw = W - L - 8
const ph = H - B - T

const max = computed(() => Math.max(1, ...props.series.flatMap((s) => s.values)))
// Graduation « ronde » en euros : 4 lignes au plus.
const step = computed(() => {
  const raw = max.value / 4
  const pow = 10 ** Math.floor(Math.log10(raw))
  const n = [1, 2, 2.5, 5, 10].find((m) => m * pow >= raw) ?? 10
  return n * pow
})
const top = computed(() => Math.ceil(max.value / step.value) * step.value)
const ticks = computed(() => Array.from({ length: Math.round(top.value / step.value) + 1 }, (_, i) => i * step.value))
const y = (v: number) => T + ph - (v / top.value) * ph
const slot = computed(() => pw / Math.max(1, props.labels.length))
const bw = computed(() => Math.min(28, (slot.value - 12) / props.series.length))

const bars = computed(() =>
  props.labels.flatMap((label, i) =>
    props.series.map((s, j) => {
      const v = s.values[i] ?? 0
      const x0 = L + i * slot.value + (slot.value - bw.value * props.series.length - 4 * (props.series.length - 1)) / 2
      const last = props.current && i === props.labels.length - 1 && j === 0
      return { key: `${i}-${j}`, x: x0 + j * (bw.value + 4), y: y(v), h: Math.max(0, y(0) - y(v)), color: s.color, last, text: `${s.name}, ${label} : ${formatCents(v)}` }
    }),
  ),
)
const tip = ref<{ x: number; y: number; text: string } | null>(null)
const euros = (c: number) => `${Math.round(c / 100)} €`
</script>

<template>
  <figure class="relative m-0">
    <svg :viewBox="`0 0 ${W} ${H}`" class="block h-auto w-full" role="img" :aria-label="summary" @mouseleave="tip = null">
      <g v-for="t in ticks" :key="t">
        <line :x1="L" :x2="W - 8" :y1="y(t)" :y2="y(t)" stroke="var(--surface-sunken)" />
        <text :x="L - 8" :y="y(t) + 4" font-size="11" fill="var(--ink-muted)" text-anchor="end">{{ euros(t) }}</text>
      </g>
      <rect
        v-for="b in bars"
        :key="b.key"
        :x="b.x"
        :y="b.y"
        :width="bw"
        :height="b.h"
        rx="4"
        :fill="b.last ? 'var(--primary-soft)' : b.color"
        :stroke="b.last ? 'var(--primary)' : 'none'"
        :stroke-dasharray="b.last ? '4 3' : undefined"
        stroke-width="1.5"
        tabindex="0"
        class="outline-none focus-visible:stroke-[var(--ink)]"
        :aria-label="b.text"
        @mouseenter="tip = { x: b.x + bw / 2, y: b.y, text: b.text }"
        @focus="tip = { x: b.x + bw / 2, y: b.y, text: b.text }"
        @blur="tip = null"
      />
      <text
        v-for="(l, i) in labels"
        :key="l + i"
        :x="L + i * slot + slot / 2"
        :y="H - 8"
        font-size="11"
        fill="var(--ink-muted)"
        text-anchor="middle"
      >
        {{ l }}
      </text>
    </svg>
    <div
      v-if="tip"
      class="pointer-events-none absolute -translate-x-1/2 -translate-y-full rounded-[var(--radius-sm)] bg-ink px-2 py-1 text-caption whitespace-nowrap text-surface"
      :style="{ left: `${(tip.x / W) * 100}%`, top: `calc(${(tip.y / H) * 100}% - 6px)` }"
      role="status"
    >
      {{ tip.text }}
    </div>
  </figure>
</template>
