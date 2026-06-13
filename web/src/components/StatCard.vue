<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'

// A clean stat tile with a tinted icon chip, a count-up value animation, a
// staggered entrance, and a subtle hover lift.
const props = withDefaults(
  defineProps<{
    label: string
    value: number
    format?: (n: number) => string
    iconColor: string
    iconBg: string
    index?: number
    // When false, the value snaps to the new number (with a soft fade) instead
    // of rolling through every intermediate digit — better for fast-changing
    // metrics like live throughput, where the count-up is dizzying.
    animate?: boolean
  }>(),
  { index: 0, animate: true },
)

const display = ref(0)
let raf = 0

function animateTo(target: number) {
  cancelAnimationFrame(raf)
  const start = display.value
  const t0 = performance.now()
  const dur = 700
  const step = (now: number) => {
    const p = Math.min(1, (now - t0) / dur)
    const eased = 1 - Math.pow(1 - p, 3) // easeOutCubic
    display.value = start + (target - start) * eased
    if (p < 1) raf = requestAnimationFrame(step)
  }
  raf = requestAnimationFrame(step)
}

function setValue(target: number) {
  if (props.animate) animateTo(target)
  else display.value = target
}

const text = computed(() =>
  props.format ? props.format(display.value) : String(Math.round(display.value)),
)

onMounted(() => setValue(props.value))
watch(() => props.value, setValue)
onUnmounted(() => cancelAnimationFrame(raf))
</script>

<template>
  <div class="stat-enter" :style="{ animationDelay: index * 80 + 'ms' }">
    <div class="stat-card">
      <div class="stat-icon" :style="{ background: iconBg, color: iconColor }">
        <slot />
      </div>
      <div class="stat-body">
        <div class="stat-label">{{ label }}</div>
        <div class="stat-value">{{ text }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Entrance lives on the wrapper so it never fights the hover transform. */
.stat-enter {
  animation: fadeUp 0.5s ease both;
}
.stat-card {
  display: flex;
  align-items: center;
  gap: 14px;
  background: #fff;
  border: 1px solid #eef1f4;
  border-radius: 14px;
  padding: 18px;
  transition: box-shadow 0.22s ease, transform 0.22s ease, border-color 0.22s ease;
}
.stat-card:hover {
  box-shadow: 0 10px 28px rgba(17, 24, 39, 0.08);
  transform: translateY(-3px);
  border-color: #e6e9ee;
}
.stat-icon {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  transition: transform 0.22s ease;
}
.stat-card:hover .stat-icon {
  transform: scale(1.08);
}
.stat-label {
  font-size: 13px;
  color: #7b8794;
  margin-bottom: 4px;
}
.stat-value {
  font-size: 24px;
  font-weight: 650;
  color: #1f2933;
  line-height: 1.1;
  letter-spacing: -0.01em;
  font-variant-numeric: tabular-nums;
}
</style>
