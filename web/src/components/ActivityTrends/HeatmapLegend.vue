<script setup lang="ts">
import { HEATMAP_LEVELS } from '../../utils/colors';
</script>

<template>
  <div :class="$style.legend">
    Less
    <div
      v-for="(level, i) in HEATMAP_LEVELS"
      :key="i"
      :class="[$style.cell, $style[level]]"
    />
    More
  </div>
</template>

<style module>
.legend {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 0.5rem;
  font-size: 0.7rem;
  color: var(--text-muted);
}
.cell {
  width: 10px;
  height: 10px;
  cursor: default;
}
.level-0 { background: #161b22; }
.level-1 { background: #0e4429; }
.level-2 { background: #006d32; }
.level-3 { background: #26a641; }
.level-4 { background: #3fb950; }
</style>
