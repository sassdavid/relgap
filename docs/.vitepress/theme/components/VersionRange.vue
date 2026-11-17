<script setup lang="ts">
// A version range as relgap reads it: the start is excluded and shown muted, the end included.
defineProps<{ from: string; to: string }>();
</script>

<template>
  <span class="rg-range"
    ><span class="rg-range__from" aria-hidden="true">{{ from }}</span
    ><span aria-hidden="true">..</span><span aria-hidden="true">{{ to }}</span
    ><span class="rg-sr-only">after {{ from }} up to and including {{ to }}</span></span
  >
</template>

<style scoped>
.rg-range {
  font-family: var(--vp-font-family-mono);
  white-space: nowrap;
}

.rg-range__from {
  color: var(--rg-ink-muted);
}
</style>
