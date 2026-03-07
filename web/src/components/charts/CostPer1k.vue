<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, computed } from 'vue';
import { Chart, registerables } from 'chart.js';
import type { ModelTokens } from '../../composables/useDashboardData';
import { MODEL_COLORS } from '../../utils/colors';
import { fmtCost } from '../../utils/format';

Chart.register(...registerables);

const props = defineProps<{
  modelTokens: Record<string, ModelTokens>;
  costByModel: Record<string, number>;
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

const chartData = computed(() => {
  const models = Object.keys(props.modelTokens).sort().filter(m => props.costByModel[m]);
  const data = models.map(m => {
    const t = props.modelTokens[m]!;
    const total = t.input + t.output + t.cacheRead + t.cacheCreation;
    return total > 0 ? ((props.costByModel[m] ?? 0) / total) * 1000 : 0;
  });
  return { models, data };
});

const updateChart = (): void => {
  if (!chart) return;
  const { models, data } = chartData.value;
  chart.data.labels = models;
  chart.data.datasets[0]!.data = data;
  (chart.data.datasets[0] as unknown as { backgroundColor: string[] }).backgroundColor =
    models.map((_, i) => MODEL_COLORS[i % MODEL_COLORS.length]!);
  chart.update();
};

onMounted(() => {
  if (!canvas.value) return;
  chart = new Chart(canvas.value, {
    type: 'bar',
    data: { labels: [], datasets: [{ label: '$/1K tokens', data: [], backgroundColor: [...MODEL_COLORS] }] },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        tooltip: { callbacks: { label: ctx => `${fmtCost(ctx.raw as number)} / 1K tokens` } },
        legend: { display: false },
      },
      scales: { y: { ticks: { callback: v => '$' + (v as number).toFixed(4) } } },
    },
  });
  updateChart();
});

watch([() => props.modelTokens, () => props.costByModel], updateChart);

onBeforeUnmount(() => {
  chart?.destroy();
  chart = null;
});
</script>

<template>
  <div :class="$style.panel">
    <h2 :class="$style.heading">Cost per 1K Tokens by Model</h2>
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
