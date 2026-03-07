<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue';
import { Chart, registerables } from 'chart.js';
import 'chartjs-adapter-date-fns';
import type { PromRangeResult } from '../../api/types';
import { LOC_COLORS } from '../../utils/colors';
import { fmtNum } from '../../utils/format';

Chart.register(...registerables);

const props = defineProps<{
  data: readonly PromRangeResult[];
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

const updateChart = (): void => {
  if (!chart) return;
  chart.data.datasets = props.data.map(series => {
    const type = series.metric['type'] ?? 'unknown';
    const color = LOC_COLORS[type as keyof typeof LOC_COLORS] ?? '#8b949e';
    return {
      label: type === 'added' ? 'Added' : type === 'removed' ? 'Removed' : type,
      borderColor: color,
      backgroundColor: color + '20',
      fill: true,
      data: series.values.map(([ts, val]) => ({ x: ts * 1000, y: parseFloat(val) })),
    };
  });
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
        y: { ticks: { callback: v => fmtNum(v as number) } },
      },
      plugins: {
        legend: { position: 'bottom', labels: { boxWidth: 12 } },
        tooltip: { callbacks: { label: ctx => `${ctx.dataset.label}: ${fmtNum((ctx.raw as { y: number }).y)}` } },
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
    <h2 :class="$style.heading">Lines of Code Over Time</h2>
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
  height: 280px;
}
</style>
