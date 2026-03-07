<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue';
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

const totals = (mt: Record<string, ModelTokens>) => {
  let input = 0, output = 0, cacheRead = 0, cacheCreation = 0;
  for (const m of Object.values(mt)) {
    input += m.input;
    output += m.output;
    cacheRead += m.cacheRead;
    cacheCreation += m.cacheCreation;
  }
  return [input, output, cacheRead, cacheCreation];
};

const updateChart = (): void => {
  if (!chart) return;
  chart.data.datasets[0]!.data = totals(props.modelTokens);
  chart.update();
};

onMounted(() => {
  if (!canvas.value) return;
  chart = new Chart(canvas.value, {
    type: 'doughnut',
    data: {
      labels: ['Input', 'Output', 'Cache Read', 'Cache Creation'],
      datasets: [{
        data: [0, 0, 0, 0],
        backgroundColor: [COLORS.input, COLORS.output, COLORS.cacheRead, COLORS.cacheCreation],
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      cutout: '40%',
      plugins: {
        tooltip: { callbacks: { label: ctx => `${ctx.label}: ${fmtNum(ctx.raw as number)}` } },
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
    <h2 :class="$style.heading">Token Type Distribution</h2>
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
