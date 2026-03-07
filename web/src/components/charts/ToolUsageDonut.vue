<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, computed } from 'vue';
import { Chart, registerables } from 'chart.js';
import { fmtNum } from '../../utils/format';

Chart.register(...registerables);

const TOOL_COLORS = ['#58a6ff', '#3fb950', '#d29922', '#bc8cff'];

const props = defineProps<{
  toolCounts: Record<string, number>;
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

const sorted = computed(() =>
  Object.entries(props.toolCounts).sort((a, b) => b[1] - a[1])
);

const updateChart = (): void => {
  if (!chart) return;
  const entries = sorted.value;
  chart.data.labels = entries.map(e => e[0]);
  chart.data.datasets[0]!.data = entries.map(e => e[1]);
  chart.update();
};

onMounted(() => {
  if (!canvas.value) return;
  chart = new Chart(canvas.value, {
    type: 'doughnut',
    data: { labels: [], datasets: [{ data: [], backgroundColor: TOOL_COLORS }] },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      cutout: '55%',
      plugins: {
        tooltip: { callbacks: { label: ctx => `${ctx.label}: ${fmtNum(ctx.raw as number)} edits` } },
        legend: { position: 'bottom', labels: { boxWidth: 12 } },
      },
    },
  });
  updateChart();
});

watch(() => props.toolCounts, updateChart);

onBeforeUnmount(() => {
  chart?.destroy();
  chart = null;
});
</script>

<template>
  <div :class="$style.panel">
    <h2 :class="$style.heading">Edit Tool Usage</h2>
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
