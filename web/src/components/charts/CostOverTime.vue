<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue';
import { Chart, registerables } from 'chart.js';
import 'chartjs-adapter-date-fns';
import type { PromRangeResult } from '../../api/types';
import { MODEL_COLORS } from '../../utils/colors';
import { fmtCost } from '../../utils/format';

Chart.register(...registerables);

const props = defineProps<{
  data: readonly PromRangeResult[];
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

const updateChart = (): void => {
  if (!chart) return;
  chart.data.datasets = props.data.map((series, i) => ({
    label: series.metric['model'] ?? 'unknown',
    borderColor: MODEL_COLORS[i % MODEL_COLORS.length],
    backgroundColor: MODEL_COLORS[i % MODEL_COLORS.length] + '20',
    fill: true,
    data: series.values.map(([ts, val]) => ({ x: ts * 1000, y: parseFloat(val) })),
  }));
  chart.update();
};

onMounted(() => {
  if (!canvas.value) return;
  chart = new Chart(canvas.value, {
    type: 'line',
    data: { datasets: [] },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { type: 'time', time: { tooltipFormat: 'MMM d, HH:mm' }, ticks: { maxTicksLimit: 10 } },
        y: { ticks: { callback: v => fmtCost(v as number) } },
      },
      plugins: {
        legend: { position: 'bottom', labels: { boxWidth: 12 } },
        tooltip: { callbacks: { label: ctx => `${ctx.dataset.label}: ${fmtCost((ctx.raw as { y: number }).y)}` } },
      },
      elements: { point: { radius: 0 }, line: { tension: 0.3, borderWidth: 2 } },
    },
  });
  updateChart();
});

watch(() => props.data, updateChart);

onBeforeUnmount(() => {
  chart?.destroy();
  chart = null;
});
</script>

<template>
  <div :class="$style.panel">
    <h2 :class="$style.heading">Cost Over Time</h2>
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
