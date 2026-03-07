<script setup lang="ts">
import { computed } from 'vue';
import type { DashboardData } from '../composables/useDashboardData';
import type { PromRangeResult } from '../api/types';
import CollapsibleSection from './CollapsibleSection.vue';
import LanguagesChart from './charts/LanguagesChart.vue';
import ToolUsageDonut from './charts/ToolUsageDonut.vue';
import LocOverTime from './charts/LocOverTime.vue';

const props = defineProps<{
  data: DashboardData;
}>();

const acceptPct = computed(() => {
  const total = props.data.totalAccept + props.data.totalReject;
  return total > 0 ? (props.data.totalAccept / total * 100) : 0;
});

const acceptColor = computed(() => {
  const pct = acceptPct.value;
  if (pct > 90) return 'var(--green)';
  if (pct > 70) return 'var(--orange)';
  return 'var(--red)';
});
</script>

<template>
  <CollapsibleSection title="Code Activity">
    <div :class="$style.grid">
      <LanguagesChart :lang-counts="data.langCounts" />
      <ToolUsageDonut :tool-counts="data.toolCounts" />
    </div>
    <div :class="$style.grid">
      <div :class="$style.panel">
        <h2 :class="$style.heading">Edit Acceptance Rate</h2>
        <div :class="$style.gaugeWrap">
          <div :class="$style.gaugeValue" :style="{ color: acceptColor }">
            {{ acceptPct.toFixed(1) }}%
          </div>
          <div :class="$style.gaugeLabel">accepted / (accepted + rejected)</div>
        </div>
      </div>
      <LocOverTime :data="data.locRange" />
    </div>
  </CollapsibleSection>
</template>

<style module>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(420px, 1fr));
  gap: 1rem;
  margin-bottom: 1rem;
}
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
.gaugeWrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
}
.gaugeValue {
  font-size: 3rem;
  font-weight: 700;
}
.gaugeLabel {
  font-size: 0.85rem;
  color: var(--text-muted);
  margin-top: 0.5rem;
}
</style>
