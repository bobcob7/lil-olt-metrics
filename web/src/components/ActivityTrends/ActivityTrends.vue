<script setup lang="ts">
import { ref, onMounted, watch } from 'vue';
import { promRangeQuery } from '../../api/prometheus';
import CalendarHeatmap from './CalendarHeatmap.vue';
import type { DailyData } from './CalendarHeatmap.vue';
import WeekdayHeatmap from './WeekdayHeatmap.vue';
import HourlyHeatmap from './HourlyHeatmap.vue';
import type { HourlyData } from './HourlyHeatmap.vue';
import HeatmapLegend from './HeatmapLegend.vue';

const props = defineProps<{
  refreshKey: number;
}>();

const YEAR_SECS = 365 * 86400;
const dailyData = ref<DailyData[]>([]);
const hourlyData = ref<HourlyData[]>([]);

const fetchHeatmap = async (): Promise<void> => {
  const end = Math.floor(Date.now() / 1000);
  const start = end - YEAR_SECS;
  try {
    const [daily, hourly] = await Promise.all([
      promRangeQuery(
        'sum(increase(claude_code_active_time_total_seconds_total[1d]))',
        start, end, '86400',
      ),
      promRangeQuery(
        'sum(increase(claude_code_active_time_total_seconds_total[1h]))',
        start, end, '3600',
      ),
    ]);
    dailyData.value = [];
    for (const series of daily) {
      for (const [ts, val] of series.values) {
        const d = new Date(ts * 1000);
        dailyData.value.push({ date: d.toISOString().slice(0, 10), value: parseFloat(val) || 0 });
      }
    }
    hourlyData.value = [];
    for (const series of hourly) {
      for (const [ts, val] of series.values) {
        const d = new Date(ts * 1000);
        hourlyData.value.push({ hour: d.getHours(), value: parseFloat(val) || 0 });
      }
    }
  } catch {
    // silently ignore heatmap errors — main data fetch shows the error
  }
};

onMounted(fetchHeatmap);
watch(() => props.refreshKey, fetchHeatmap);
</script>

<template>
  <div :class="$style.panel">
    <CalendarHeatmap :data="dailyData" :range-secs="YEAR_SECS" />
    <WeekdayHeatmap :data="dailyData" />
    <HourlyHeatmap :data="hourlyData" />
    <HeatmapLegend />
  </div>
</template>

<style module>
.panel {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 1.25rem;
  margin-bottom: 1rem;
}
</style>
