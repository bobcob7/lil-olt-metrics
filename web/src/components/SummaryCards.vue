<script setup lang="ts">
import { computed } from 'vue';
import type { DashboardData } from '../composables/useDashboardData';
import { fmtNum, fmtCost, fmtDuration } from '../utils/format';

const props = defineProps<{
  data: DashboardData;
}>();

const netLines = computed(() => props.data.linesAdded - props.data.linesRemoved);

const tokenSub = computed(() => {
  const mt = props.data.modelTokens;
  let totIn = 0;
  let totOut = 0;
  for (const m of Object.values(mt)) {
    totIn += m.input;
    totOut += m.output;
  }
  return `${fmtNum(totIn)} in / ${fmtNum(totOut)} out`;
});

const costSub = computed(() => {
  const entries = Object.entries(props.data.costByModel).sort((a, b) => b[1] - a[1]);
  const top = entries[0];
  if (!top) return '';
  return `highest: ${top[0]} (${fmtCost(top[1])})`;
});

const activeSub = computed(() => {
  if (props.data.totalSessions <= 0) return '';
  return `avg ${fmtDuration(props.data.totalActive / props.data.totalSessions)} / session`;
});
</script>

<template>
  <div :class="$style.cards">
    <div :class="$style.card">
      <div :class="$style.label">Total Tokens</div>
      <div :class="$style.value">{{ fmtNum(data.totalTokens) }}</div>
      <div :class="$style.sub">{{ tokenSub }}</div>
    </div>
    <div :class="$style.card">
      <div :class="$style.label">Total Cost</div>
      <div :class="$style.value">{{ fmtCost(data.totalCost) }}</div>
      <div :class="$style.sub">{{ costSub }}</div>
    </div>
    <div :class="$style.card">
      <div :class="$style.label">Active Time</div>
      <div :class="$style.value">{{ fmtDuration(data.totalActive) }}</div>
      <div :class="$style.sub">{{ activeSub }}</div>
    </div>
    <div :class="$style.card">
      <div :class="$style.label">Sessions</div>
      <div :class="$style.value">{{ Math.round(data.totalSessions).toLocaleString() }}</div>
    </div>
    <div :class="$style.card">
      <div :class="$style.label">Commits</div>
      <div :class="$style.value">{{ Math.round(data.totalCommits).toLocaleString() }}</div>
    </div>
    <div :class="$style.card">
      <div :class="$style.label">Pull Requests</div>
      <div :class="$style.value">{{ Math.round(data.totalPRs).toLocaleString() }}</div>
    </div>
    <div :class="$style.card">
      <div :class="$style.label">Lines Changed</div>
      <div :class="$style.value">{{ (netLines >= 0 ? '+' : '') + fmtNum(netLines) }}</div>
      <div :class="$style.sub">+{{ fmtNum(data.linesAdded) }} / -{{ fmtNum(data.linesRemoved) }}</div>
    </div>
  </div>
</template>

<style module>
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 1rem;
  margin-bottom: 1.5rem;
}
.card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 1.25rem;
}
.label {
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-muted);
  margin-bottom: 0.5rem;
}
.value {
  font-size: 1.75rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.sub {
  font-size: 0.8rem;
  color: var(--text-muted);
  margin-top: 0.25rem;
}
</style>
