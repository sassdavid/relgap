<script setup lang="ts">
// An entry class's inline mark: a colour and a bar pattern, so colour is never the only cue
// (WCAG 1.4.1). The label carries the meaning; with bare the caller prints the label itself.
import { computed } from "vue";

export type EntryClass = "breaking" | "deprecation" | "security" | "feature" | "fix" | "other";

const props = defineProps<{ kind: EntryClass; label?: string; bare?: boolean }>();
const text = computed(() => props.label ?? props.kind);
</script>

<template>
  <span class="rg-mark" :class="`rg-mark--${kind}`">
    <svg class="rg-mark__svg" viewBox="0 0 16 16" width="1em" height="1em" aria-hidden="true">
      <template v-if="kind === 'breaking'">
        <rect x="1" y="5" width="6" height="6" fill="currentColor" />
        <rect x="9" y="5" width="6" height="6" fill="currentColor" />
      </template>
      <template v-else-if="kind === 'deprecation'">
        <rect x="1.5" y="5.5" width="13" height="5" fill="none" stroke="currentColor" />
        <path d="M2 11 6 5M6 11l4-6M10 11l4-6" stroke="currentColor" stroke-width="1.2" />
      </template>
      <template v-else-if="kind === 'security'">
        <rect x="1.5" y="5.5" width="13" height="5" fill="none" stroke="currentColor" />
        <circle cx="4.5" cy="8" r="1.2" fill="currentColor" />
        <circle cx="8" cy="8" r="1.2" fill="currentColor" />
        <circle cx="11.5" cy="8" r="1.2" fill="currentColor" />
      </template>
      <template v-else-if="kind === 'feature'">
        <rect x="1" y="5" width="14" height="6" fill="currentColor" />
      </template>
      <template v-else-if="kind === 'fix'">
        <rect x="1" y="5" width="14" height="6" fill="currentColor" fill-opacity="0.45" />
      </template>
      <template v-else>
        <rect
          x="1.75"
          y="5.75"
          width="12.5"
          height="4.5"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
        />
      </template>
    </svg>
    <span v-if="!bare" class="rg-mark__label">{{ text }}</span>
  </span>
</template>

<style scoped>
.rg-mark {
  display: inline-flex;
  align-items: center;
  gap: 0.35em;
  white-space: nowrap;
}

.rg-mark__svg {
  flex: none;
  vertical-align: -0.125em;
}

.rg-mark--breaking .rg-mark__svg {
  color: var(--rg-breaking);
}

.rg-mark--deprecation .rg-mark__svg {
  color: var(--rg-deprecation);
}

.rg-mark--security .rg-mark__svg {
  color: var(--rg-security);
}

.rg-mark--feature .rg-mark__svg {
  color: var(--rg-feature);
}

.rg-mark--fix .rg-mark__svg {
  color: var(--rg-fix);
}

.rg-mark--other .rg-mark__svg {
  color: var(--rg-other);
}
</style>
