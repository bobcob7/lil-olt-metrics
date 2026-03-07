<script setup lang="ts">
import { computed } from 'vue';
import { computeLevels, DAY_NAMES, MONTH_NAMES } from '../../utils/heatmap';
import { HEATMAP_LEVELS } from '../../utils/colors';
import { fmtDuration } from '../../utils/format';

export interface DailyData {
  date: string;
  value: number;
}

const props = defineProps<{
  data: readonly DailyData[];
  rangeSecs: number;
}>();

interface WeekDay {
  date: Date;
  dow: number;
  key: string;
  value: number;
}

const getLevel = computed(() => computeLevels(props.data.map(d => d.value)));

const totalDays = computed(() => Math.ceil(props.rangeSecs / 86400));
const avgDaily = computed(() =>
  props.data.reduce((s, d) => s + d.value, 0) / Math.max(totalDays.value, 1)
);

const weeks = computed(() => {
  const dateMap: Record<string, number> = {};
  for (const d of props.data) dateMap[d.date] = d.value;
  const end = new Date();
  end.setHours(23, 59, 59, 999);
  const start = new Date(end.getTime() - props.rangeSecs * 1000);
  start.setHours(0, 0, 0, 0);
  const dates: WeekDay[] = [];
  const cur = new Date(start);
  while (cur <= end) {
    const dow = (cur.getDay() + 6) % 7;
    const key = cur.toISOString().slice(0, 10);
    dates.push({ date: new Date(cur), dow, key, value: dateMap[key] ?? 0 });
    cur.setDate(cur.getDate() + 1);
  }
  const result: WeekDay[][] = [];
  let current: WeekDay[] = [];
  for (const d of dates) {
    if (d.dow === 0 && current.length > 0) {
      result.push(current);
      current = [];
    }
    current.push(d);
  }
  if (current.length > 0) result.push(current);
  return result;
});

const monthLabels = computed(() =>
  weeks.value.map((week) => {
    const first = week[0]!.date;
    if (first.getDate() <= 7) return MONTH_NAMES[first.getMonth()]!;
    return '';
  })
);

const tipText = (key: string, val: number): string =>
  `${key}: ${fmtDuration(val)} (avg: ${fmtDuration(avgDaily.value)}/day)`;
</script>

<template>
  <div :class="$style.section">
    <h3 :class="$style.heading">Daily Activity</h3>
    <div :class="$style.monthRow">
      <span :class="$style.monthSpacer" />
      <span v-for="(label, i) in monthLabels" :key="i" :class="$style.monthLabel">{{ label }}</span>
    </div>
    <div :class="$style.grid">
      <div :class="$style.labelCol">
        <div v-for="row in 7" :key="row" :class="$style.dayLabel">
          {{ row % 2 === 1 ? DAY_NAMES[row - 1] : '' }}
        </div>
      </div>
      <div v-for="(week, wi) in weeks" :key="wi" :class="$style.col">
        <template v-if="wi === 0">
          <div
            v-for="i in week[0]!.dow"
            :key="'empty-start-' + i"
            :class="[$style.cell, $style['level-0']]"
            style="visibility: hidden"
          />
        </template>
        <div
          v-for="day in week"
          :key="day.key"
          :class="[$style.cell, $style[HEATMAP_LEVELS[getLevel(day.value)] ?? 'level-0']]"
          :title="tipText(day.key, day.value)"
        />
        <template v-if="wi === weeks.length - 1">
          <div
            v-for="i in (6 - week[week.length - 1]!.dow)"
            :key="'empty-end-' + i"
            :class="[$style.cell, $style['level-0']]"
            style="visibility: hidden"
          />
        </template>
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
.monthRow {
  display: flex;
  margin-bottom: 2px;
}
.monthSpacer {
  flex: none;
  width: 30px;
}
.monthLabel {
  font-size: 0.7rem;
  color: var(--text-muted);
  flex: 1;
  min-width: 0;
}
.grid {
  display: flex;
  gap: 0;
}
.labelCol {
  display: flex;
  flex-direction: column;
  flex: none;
  width: 28px;
  margin-right: 2px;
}
.dayLabel {
  font-size: 0.7rem;
  color: var(--text-muted);
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  padding-right: 4px;
}
.col {
  display: flex;
  flex-direction: column;
  flex: 1;
}
.cell {
  aspect-ratio: 1;
  cursor: pointer;
  position: relative;
  max-width: 40px;
  max-height: 40px;
  outline: 1px solid rgba(255, 255, 255, 0.1);
}
.level-0 { background: #161b22; }
.level-1 { background: #0e4429; }
.level-2 { background: #006d32; }
.level-3 { background: #26a641; }
.level-4 { background: #3fb950; }
</style>
