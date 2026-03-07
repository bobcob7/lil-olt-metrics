<script setup lang="ts">
defineProps<{
  title: string;
  open?: boolean;
}>();
</script>

<template>
  <details :open="open">
    <summary :class="$style.summary">{{ title }}</summary>
    <slot />
  </details>
</template>

<style module>
.summary {
  font-size: 1.15rem;
  font-weight: 600;
  cursor: pointer;
  padding: 0.75rem 0;
  color: var(--text);
  list-style: disclosure-closed;
  user-select: none;
}
.summary:hover {
  color: var(--accent);
}
details[open] .summary {
  list-style: disclosure-open;
  margin-bottom: 1rem;
}
</style>
