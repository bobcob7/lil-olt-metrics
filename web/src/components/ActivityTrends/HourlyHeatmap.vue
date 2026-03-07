<script setup lang="ts">
import { computed } from 'vue';
import { computeLevels } from '../../utils/heatmap';
import { HEATMAP_LEVELS } from '../../utils/colors';
import { fmtDuration } from '../../utils/format';

export interface HourlyData {
  hour: number;
  value: number;
}

const props = defineProps<{
  data: readonly HourlyData[];
}>();

const hourData = computed(() => {
  const totals = new Array<number>(24).fill(0);
  const counts = new Array<number>(24).fill(0);
  for (const d of props.data) {
    totals[d.hour]! += d.value;
    counts[d.hour]! += 1;
  }
  const getLevel = computeLevels(totals);
  return Array.from({ length: 24 }, (_, h) => ({
    hour: String(h).padStart(2, '0'),
    total: totals[h]!,
    avg: counts[h]! > 0 ? totals[h]! / counts[h]! : 0,
    level: getLevel(totals[h]!),
  }));
});
</script>

<template>
  <div :class="$style.section">
    <h3 :class="$style.heading">Activity by Hour</h3>
    <div :class="$style.row">
      <div v-for="h in hourData" :key="h.hour" :class="$style.wrapper">
        <div :class="$style.label">{{ h.hour }}</div>
        <div
          :class="[$style.cell, $style[HEATMAP_LEVELS[h.level] ?? 'level-0']]"
          :title="`${h.hour}:00: ${fmtDuration(h.total)} total (avg: ${fmtDuration(h.avg)}/occurrence)`"
        />
      </div>
    </div>
  </div>
</template>

<style module>
.section { margin-bottom: 0.5rem; }
.heading {
  font-size: 0.85rem;
  font-weight: 500;
  color: var(--text-muted);
  margin-bottom: 0.5rem;
}
.row {
  display: flex;
  align-items: center;
}
.wrapper {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1;
  gap: 2px;
}
.label {
  font-size: 0.7rem;
  color: var(--text-muted);
  text-align: center;
}
.cell {
  width: 100%;
  height: 24px;
  cursor: pointer;
  outline: 1px solid rgba(255, 255, 255, 0.1);
}
.level-0 { background: #161b22; }
.level-1 { background: #0e4429; }
.level-2 { background: #006d32; }
.level-3 { background: #26a641; }
.level-4 { background: #3fb950; }
</style>
