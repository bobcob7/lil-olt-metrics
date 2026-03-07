<script setup lang="ts">
import { ref, watch, onMounted } from 'vue';
import { Chart } from 'chart.js';
import { useTimeRange } from './composables/useTimeRange';
import { useAutoRefresh } from './composables/useAutoRefresh';
import { useDashboardData } from './composables/useDashboardData';
import DashboardHeader from './components/DashboardHeader.vue';
import ErrorBanner from './components/ErrorBanner.vue';
import SummaryCards from './components/SummaryCards.vue';
import CollapsibleSection from './components/CollapsibleSection.vue';
import ActivityTrends from './components/ActivityTrends/ActivityTrends.vue';
import TokensOverTime from './components/charts/TokensOverTime.vue';
import CostOverTime from './components/charts/CostOverTime.vue';
import CodeActivity from './components/CodeActivity.vue';
import TokenBreakdown from './components/TokenBreakdown.vue';
import EfficiencyAnalysis from './components/EfficiencyAnalysis.vue';

Chart.defaults.color = '#8b949e';
Chart.defaults.borderColor = '#30363d';
Chart.defaults.font.family = '-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif';

const { selectedRange } = useTimeRange();
const { data, error, lastUpdated, refresh } = useDashboardData();
const refreshKey = ref(0);

const doRefresh = async (): Promise<void> => {
  await refresh(selectedRange.value);
  refreshKey.value++;
};

const { enabled: autoRefresh, toggle: toggleAutoRefresh } = useAutoRefresh(doRefresh);

watch(selectedRange, doRefresh);
onMounted(doRefresh);
</script>

<template>
  <DashboardHeader
    :selected-range="selectedRange"
    :auto-refresh="autoRefresh"
    :last-updated="lastUpdated"
    @update:selected-range="selectedRange = $event"
    @update:auto-refresh="toggleAutoRefresh($event)"
  />
  <ErrorBanner :message="error" />
  <template v-if="data">
    <SummaryCards :data="data" />
    <CollapsibleSection title="Activity Trends" :open="true">
      <ActivityTrends :refresh-key="refreshKey" />
    </CollapsibleSection>
    <div :class="$style.grid">
      <TokensOverTime :data="data.tokensRange" />
      <CostOverTime :data="data.costRange" />
    </div>
    <CodeActivity :data="data" />
    <TokenBreakdown :data="data" />
    <EfficiencyAnalysis :data="data" />
  </template>
</template>

<style module>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(420px, 1fr));
  gap: 1rem;
  margin-bottom: 1.5rem;
}
</style>
