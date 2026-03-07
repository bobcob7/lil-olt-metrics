<script setup lang="ts">
import { computed } from 'vue';
import type { DashboardData } from '../composables/useDashboardData';
import CollapsibleSection from './CollapsibleSection.vue';
import TokenTypePie from './charts/TokenTypePie.vue';
import CostPer1k from './charts/CostPer1k.vue';
import ActivityBySession from './charts/ActivityBySession.vue';

const props = defineProps<{
  data: DashboardData;
}>();

const cachePct = computed(() => {
  let totInput = 0, totCacheRead = 0;
  for (const m of Object.values(props.data.modelTokens)) {
    totInput += m.input;
    totCacheRead += m.cacheRead;
  }
  const total = totCacheRead + totInput;
  return total > 0 ? (totCacheRead / total * 100) : 0;
});

const cacheColor = computed(() => {
  const pct = cachePct.value;
  if (pct > 60) return 'var(--green)';
  if (pct > 30) return 'var(--orange)';
  return 'var(--red)';
});
</script>

<template>
  <CollapsibleSection title="Efficiency & Cost Analysis">
    <div :class="$style.grid">
      <div :class="$style.panel">
        <h2 :class="$style.heading">Cache Efficiency</h2>
        <div :class="$style.gaugeWrap">
          <div :class="$style.gaugeValue" :style="{ color: cacheColor }">
            {{ cachePct.toFixed(1) }}%
          </div>
          <div :class="$style.gaugeLabel">cache read / (cache read + input)</div>
        </div>
      </div>
      <TokenTypePie :model-tokens="data.modelTokens" />
    </div>
    <div :class="$style.grid">
      <CostPer1k :model-tokens="data.modelTokens" :cost-by-model="data.costByModel" />
      <ActivityBySession :session-map="data.activeTimeBySession" :types="data.activeTimeTypes" />
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
