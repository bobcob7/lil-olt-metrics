<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, computed } from 'vue';
import { Chart, registerables } from 'chart.js';
import type { ModelTokens } from '../../composables/useDashboardData';
import { COLORS } from '../../utils/colors';
import { fmtNum } from '../../utils/format';

Chart.register(...registerables);

const props = defineProps<{
  modelTokens: Record<string, ModelTokens>;
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

const models = computed(() => Object.keys(props.modelTokens).sort());

const updateChart = (): void => {
  if (!chart) return;
  const m = models.value;
  chart.data.labels = m;
  chart.data.datasets = [
    { label: 'Input', data: m.map(k => props.modelTokens[k]!.input), backgroundColor: COLORS.input },
    { label: 'Output', data: m.map(k => props.modelTokens[k]!.output), backgroundColor: COLORS.output },
    { label: 'Cache Read', data: m.map(k => props.modelTokens[k]!.cacheRead), backgroundColor: COLORS.cacheRead },
    { label: 'Cache Creation', data: m.map(k => props.modelTokens[k]!.cacheCreation), backgroundColor: COLORS.cacheCreation },
  ];
  chart.update();
};

onMounted(() => {
  if (!canvas.value) return;
  chart = new Chart(canvas.value, {
    type: 'bar',
    data: { labels: [], datasets: [] },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { stacked: true },
        y: { stacked: true, ticks: { callback: v => fmtNum(v as number) } },
      },
      plugins: {
        tooltip: { callbacks: { label: ctx => `${ctx.dataset.label}: ${fmtNum(ctx.raw as number)}` } },
        legend: { position: 'bottom', labels: { boxWidth: 12 } },
      },
    },
  });
  updateChart();
});

watch(() => props.modelTokens, updateChart);

onBeforeUnmount(() => {
  chart?.destroy();
  chart = null;
});
</script>

<template>
  <div :class="$style.panel">
    <h2 :class="$style.heading">Tokens by Model</h2>
    <div :class="$style.container"><canvas ref="canvas"></canvas></div>
  </div>
</template>

<style module>
.panel {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 1.25rem;
}
.heading {
  font-size: 1rem;
  font-weight: 600;
  margin-bottom: 1rem;
}
.container {
  position: relative;
  width: 100%;
  height: 320px;
}
</style>
