<script setup lang="ts">
import { computed } from 'vue';
import type { DashboardData, ModelTokens } from '../composables/useDashboardData';
import { fmtNum } from '../utils/format';
import CollapsibleSection from './CollapsibleSection.vue';
import TokensByModel from './charts/TokensByModel.vue';
import CostByModel from './charts/CostByModel.vue';

const props = defineProps<{
  data: DashboardData;
}>();

interface TableRow {
  model: string;
  input: number;
  output: number;
  cacheRead: number;
  cacheCreation: number;
  total: number;
}

const models = computed(() => Object.keys(props.data.modelTokens).sort());

const tableRows = computed((): readonly TableRow[] => {
  return models.value.map(m => {
    const d = props.data.modelTokens[m]!;
    return {
      model: m,
      input: d.input,
      output: d.output,
      cacheRead: d.cacheRead,
      cacheCreation: d.cacheCreation,
      total: d.input + d.output + d.cacheRead + d.cacheCreation,
    };
  });
});

const totals = computed(() => {
  let input = 0, output = 0, cacheRead = 0, cacheCreation = 0;
  for (const r of tableRows.value) {
    input += r.input;
    output += r.output;
    cacheRead += r.cacheRead;
    cacheCreation += r.cacheCreation;
  }
  return { input, output, cacheRead, cacheCreation, total: input + output + cacheRead + cacheCreation };
});
</script>

<template>
  <CollapsibleSection title="Token Breakdown">
    <div :class="$style.grid">
      <TokensByModel :model-tokens="data.modelTokens" />
      <CostByModel :cost-by-model="data.costByModel" />
    </div>
    <div :class="$style.panel">
      <h2 :class="$style.heading">Token Details</h2>
      <div style="overflow-x: auto">
        <table>
          <thead>
            <tr>
              <th>Model</th>
              <th class="num">Input</th>
              <th class="num">Output</th>
              <th class="num">Cache Read</th>
              <th class="num">Cache Create</th>
              <th class="num">Total</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in tableRows" :key="row.model">
              <td>{{ row.model }}</td>
              <td class="num">{{ fmtNum(row.input) }}</td>
              <td class="num">{{ fmtNum(row.output) }}</td>
              <td class="num">{{ fmtNum(row.cacheRead) }}</td>
              <td class="num">{{ fmtNum(row.cacheCreation) }}</td>
              <td class="num">{{ fmtNum(row.total) }}</td>
            </tr>
            <tr v-if="models.length > 1" style="font-weight: 600">
              <td>Total</td>
              <td class="num">{{ fmtNum(totals.input) }}</td>
              <td class="num">{{ fmtNum(totals.output) }}</td>
              <td class="num">{{ fmtNum(totals.cacheRead) }}</td>
              <td class="num">{{ fmtNum(totals.cacheCreation) }}</td>
              <td class="num">{{ fmtNum(totals.total) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
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
  margin-bottom: 1rem;
}
.heading {
  font-size: 1rem;
  font-weight: 600;
  margin-bottom: 1rem;
}
</style>
