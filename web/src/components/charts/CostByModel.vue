<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, computed } from 'vue';
import { Chart, registerables } from 'chart.js';
import { MODEL_COLORS } from '../../utils/colors';
import { fmtCost } from '../../utils/format';

Chart.register(...registerables);

const props = defineProps<{
  costByModel: Record<string, number>;
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

const sorted = computed(() => Object.keys(props.costByModel).sort());

const updateChart = (): void => {
  if (!chart) return;
  const m = sorted.value;
  chart.data.labels = m;
  chart.data.datasets[0]!.data = m.map(k => props.costByModel[k]!);
  (chart.data.datasets[0] as unknown as { backgroundColor: string[] }).backgroundColor =
    m.map((_, i) => MODEL_COLORS[i % MODEL_COLORS.length]!);
  chart.update();
};

onMounted(() => {
  if (!canvas.value) return;
  chart = new Chart(canvas.value, {
    type: 'doughnut',
    data: { labels: [], datasets: [{ data: [], backgroundColor: [...MODEL_COLORS] }] },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      cutout: '55%',
      plugins: {
        tooltip: { callbacks: { label: ctx => `${ctx.label}: ${fmtCost(ctx.raw as number)}` } },
        legend: { position: 'bottom', labels: { boxWidth: 12 } },
      },
    },
  });
  updateChart();
});

watch(() => props.costByModel, updateChart);

onBeforeUnmount(() => {
  chart?.destroy();
  chart = null;
});
</script>

<template>
  <div :class="$style.panel">
    <h2 :class="$style.heading">Cost by Model</h2>
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
