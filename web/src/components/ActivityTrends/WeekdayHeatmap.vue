<script setup lang="ts">
import { computed } from 'vue';
import { computeLevels, DAY_NAMES } from '../../utils/heatmap';
import { HEATMAP_LEVELS } from '../../utils/colors';
import { fmtDuration } from '../../utils/format';

export interface DailyData {
  date: string;
  value: number;
}

const props = defineProps<{
  data: readonly DailyData[];
}>();

const dowData = computed(() => {
  const totals = [0, 0, 0, 0, 0, 0, 0];
  const counts = [0, 0, 0, 0, 0, 0, 0];
  for (const d of props.data) {
    const date = new Date(d.date + 'T12:00:00');
    const dow = (date.getDay() + 6) % 7;
    totals[dow]! += d.value;
    counts[dow]! += 1;
  }
  const getLevel = computeLevels(totals);
  return DAY_NAMES.map((name, i) => ({
    name,
    total: totals[i]!,
    avg: counts[i]! > 0 ? totals[i]! / counts[i]! : 0,
    level: getLevel(totals[i]!),
  }));
});
</script>

<template>
  <div :class="$style.section">
    <h3 :class="$style.heading">Activity by Day of Week</h3>
    <div :class="$style.row">
      <div v-for="d in dowData" :key="d.name" :class="$style.wrapper">
        <div :class="$style.label">{{ d.name }}</div>
        <div
          :class="[$style.cell, $style[HEATMAP_LEVELS[d.level] ?? 'level-0']]"
          :title="`${d.name}: ${fmtDuration(d.total)} total (avg: ${fmtDuration(d.avg)}/day)`"
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
  height: 32px;
  cursor: pointer;
  outline: 1px solid rgba(255, 255, 255, 0.1);
}
.level-0 { background: #161b22; }
.level-1 { background: #0e4429; }
.level-2 { background: #006d32; }
.level-3 { background: #26a641; }
.level-4 { background: #3fb950; }
</style>
