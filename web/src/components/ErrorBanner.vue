<script setup lang="ts">
defineProps<{
  message: string | null;
}>();
</script>

<template>
  <div v-if="message" :class="$style.banner">{{ message }}</div>
</template>

<style module>
.banner {
  background: rgba(248, 81, 73, 0.15);
  border: 1px solid var(--red);
  color: var(--red);
  padding: 0.75rem 1rem;
  border-radius: 6px;
  margin-bottom: 1rem;
  font-size: 0.85rem;
}
</style>
