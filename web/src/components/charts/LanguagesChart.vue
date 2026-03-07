<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, computed } from 'vue';
import { Chart, registerables } from 'chart.js';
import { LANG_COLORS } from '../../utils/colors';
import { fmtNum } from '../../utils/format';

Chart.register(...registerables);

const props = defineProps<{
  langCounts: Record<string, number>;
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

const sorted = computed(() =>
  Object.entries(props.langCounts).sort((a, b) => b[1] - a[1])
);

const updateChart = (): void => {
  if (!chart) return;
  const entries = sorted.value;
  chart.data.labels = entries.map(e => e[0]);
  chart.data.datasets[0]!.data = entries.map(e => e[1]);
  (chart.data.datasets[0] as unknown as { backgroundColor: string[] }).backgroundColor =
    entries.map((_, i) => LANG_COLORS[i % LANG_COLORS.length]!);
  chart.update();
};

onMounted(() => {
  if (!canvas.value) return;
  chart = new Chart(canvas.value, {
    type: 'bar',
    data: { labels: [], datasets: [{ label: 'Edits', data: [], backgroundColor: [...LANG_COLORS] }] },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      indexAxis: 'y',
      plugins: {
        tooltip: { callbacks: { label: ctx => `${ctx.raw} edits` } },
        legend: { display: false },
      },
      scales: { x: { ticks: { callback: v => fmtNum(v as number) } } },
    },
  });
  updateChart();
});

watch(() => props.langCounts, updateChart);

onBeforeUnmount(() => {
  chart?.destroy();
  chart = null;
});
</script>

<template>
  <div :class="$style.panel">
    <h2 :class="$style.heading">Languages Edited</h2>
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
