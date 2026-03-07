<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount, computed } from 'vue';
import { Chart, registerables } from 'chart.js';
import { ACTIVITY_COLORS } from '../../utils/colors';
import { fmtDuration } from '../../utils/format';

Chart.register(...registerables);

const props = defineProps<{
  sessionMap: Record<string, Record<string, number>>;
  types: readonly string[];
}>();

const canvas = ref<HTMLCanvasElement | null>(null);
let chart: Chart | null = null;

const sessionIds = computed(() => Object.keys(props.sessionMap).sort());

const shortIds = computed(() =>
  sessionIds.value.map(id => id.length > 12 ? id.slice(0, 6) + '..' + id.slice(-4) : id)
);

const updateChart = (): void => {
  if (!chart) return;
  const ids = sessionIds.value;
  chart.data.labels = shortIds.value;
  chart.data.datasets = props.types.map((type, i) => ({
    label: type,
    data: ids.map(sid => props.sessionMap[sid]?.[type] ?? 0),
    backgroundColor: ACTIVITY_COLORS[i % ACTIVITY_COLORS.length]!,
  }));
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
        y: { stacked: true, ticks: { callback: v => fmtDuration(v as number) } },
      },
      plugins: {
        tooltip: { callbacks: { label: ctx => `${ctx.dataset.label}: ${fmtDuration(ctx.raw as number)}` } },
        legend: { position: 'bottom', labels: { boxWidth: 12 } },
      },
    },
  });
  updateChart();
});

watch([() => props.sessionMap, () => props.types], updateChart);

onBeforeUnmount(() => {
  chart?.destroy();
  chart = null;
});
</script>

<template>
  <div :class="$style.panel">
    <h2 :class="$style.heading">Active Time by Session</h2>
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
