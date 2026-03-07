<script setup lang="ts">
import { TIME_RANGE_OPTIONS } from '../composables/useTimeRange';

defineProps<{
  selectedRange: string;
  autoRefresh: boolean;
  lastUpdated: string | null;
}>();

const emit = defineEmits<{
  'update:selectedRange': [value: string];
  'update:autoRefresh': [value: boolean];
}>();
</script>

<template>
  <header :class="$style.header">
    <h1 :class="$style.title">Claude Code Metrics</h1>
    <div :class="$style.controls">
      <select
        :value="selectedRange"
        @change="emit('update:selectedRange', ($event.target as HTMLSelectElement).value)"
      >
        <option v-for="opt in TIME_RANGE_OPTIONS" :key="opt" :value="opt">{{ opt }}</option>
      </select>
      <label :class="$style.toggle">
        <input
          type="checkbox"
          :checked="autoRefresh"
          @change="emit('update:autoRefresh', ($event.target as HTMLInputElement).checked)"
        />
        Auto-refresh (30s)
      </label>
      <span>{{ lastUpdated ?? '--' }}</span>
    </div>
  </header>
</template>

<style module>
.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
  gap: 0.75rem;
}
.title {
  font-size: 1.5rem;
  font-weight: 600;
}
.controls {
  display: flex;
  align-items: center;
  gap: 1rem;
  font-size: 0.85rem;
  color: var(--text-muted);
}
.toggle {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  cursor: pointer;
  user-select: none;
}
.toggle input {
  accent-color: var(--accent);
}
</style>
