<script setup lang="ts">
import { HEATMAP_LEVELS } from '../../utils/colors';

defineProps<{
  level: number;
  tooltip: string;
  hidden?: boolean;
}>();
</script>

<template>
  <div
    :class="[$style.cell, $style[HEATMAP_LEVELS[level] ?? 'level-0']]"
    :style="hidden ? { visibility: 'hidden' } : undefined"
    :title="tooltip"
  />
</template>

<style module>
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
