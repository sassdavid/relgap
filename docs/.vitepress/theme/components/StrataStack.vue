<script lang="ts">
// The settle animation plays once per visit, not on every return to the home page.
let played = false;
</script>

<script setup lang="ts">
// Release strata: one layer per release between the pin (bottom) and latest (top), its length
// set by its entry count and its segments by entry class. A breaking release gets a fault line
// under it. The layers are hidden from screen readers, which read the list instead.
import { computed, onMounted } from "vue";
import EntryMark, { type EntryClass } from "./EntryMark.vue";
import VersionRange from "./VersionRange.vue";

export interface Layer {
  version: string;
  counts: Record<EntryClass, number>;
  total: number;
}

const props = defineProps<{
  tool: string;
  from: string;
  to: string;
  layers: Layer[];
  breaking: string[];
}>();

const classes: EntryClass[] = ["breaking", "deprecation", "security", "feature", "fix", "other"];

const animate = !played;
onMounted(() => {
  played = true;
});

const most = computed(() => Math.max(1, ...props.layers.map((l) => l.total)));
const rows = computed(() =>
  props.layers
    .map((layer, i) => ({
      ...layer,
      step: i,
      width: layer.total === 0 ? 0 : Math.max(2, (layer.total / most.value) * 100),
      segments: classes
        .filter((c) => layer.counts[c] > 0)
        .map((c) => ({ kind: c, n: layer.counts[c] })),
      breaks: props.breaking.includes(layer.version),
    }))
    .toReversed(),
);

function summary(layer: Layer): string {
  const parts = classes.filter((c) => layer.counts[c] > 0).map((c) => `${layer.counts[c]} ${c}`);
  return parts.length ? parts.join(", ") : "no entries";
}
</script>

<template>
  <figure class="rg-strata" :class="{ 'rg-strata--animate': animate }">
    <figcaption class="rg-strata__caption">
      {{ tool }} <VersionRange :from="from" :to="to" />
    </figcaption>
    <div class="rg-strata__rows" aria-hidden="true" :style="{ '--steps': layers.length }">
      <template v-for="(row, i) in rows" :key="row.version">
        <div class="rg-strata__row" :style="{ '--step': row.step }">
          <span class="rg-strata__tag">{{ i === 0 ? "latest" : "" }}</span>
          <span class="rg-strata__version">{{ row.version }}</span>
          <span class="rg-strata__track">
            <span class="rg-strata__bar" :style="{ width: `${row.width}%` }">
              <span
                v-for="seg in row.segments"
                :key="seg.kind"
                class="rg-strata__seg"
                :class="`rg-strata__seg--${seg.kind}`"
                :style="{ flexGrow: seg.n }"
              />
            </span>
          </span>
          <span class="rg-strata__count">{{ row.total }}</span>
        </div>
        <div v-if="row.breaks" class="rg-strata__fault">
          <span class="rg-strata__fault-text">breaking in {{ row.version }}</span>
        </div>
      </template>
      <div class="rg-strata__row rg-strata__row--pin">
        <span class="rg-strata__tag">pinned</span>
        <span class="rg-strata__version">{{ from }}</span>
      </div>
    </div>
    <ul class="rg-strata__legend" aria-hidden="true">
      <li v-for="c in classes" :key="c"><EntryMark :kind="c" /></li>
    </ul>
    <ul class="rg-sr-only">
      <li v-for="layer in [...layers].reverse()" :key="layer.version">
        {{ layer.version }}{{ breaking.includes(layer.version) ? " (breaking release)" : "" }}:
        {{ summary(layer) }}
      </li>
      <li>pinned {{ from }}</li>
    </ul>
  </figure>
</template>

<style scoped>
.rg-strata {
  position: relative;
  margin: 0;
  padding: 1.75rem 1.25rem 1rem;
  border: 1px solid var(--rg-rule);
  border-radius: 2px;
  background: var(--rg-surface);
}

.rg-strata__caption {
  position: absolute;
  top: -0.75em;
  left: 0.9rem;
  padding: 0 0.4rem;
  background: var(--rg-surface);
  color: var(--rg-ink);
  font-size: 0.875rem;
  line-height: 1.4;
}

.rg-strata__rows {
  display: grid;
  gap: 0.3rem;
}

.rg-strata__row {
  display: grid;
  grid-template-columns: 3.25rem 3.75rem 1fr 2.25rem;
  align-items: center;
  gap: 0.5rem;
  min-height: 1.25rem;
}

.rg-strata__tag {
  color: var(--rg-ink-muted);
  font-size: 0.8125rem;
}

.rg-strata__version {
  color: var(--rg-ink);
  font-family: var(--vp-font-family-mono);
  font-size: 0.875rem;
}

.rg-strata__track {
  display: block;
}

.rg-strata__bar {
  display: flex;
  gap: 1px;
  height: 0.875rem;
}

.rg-strata__seg {
  flex: 1 0 3px;
}

.rg-strata__seg--breaking {
  background: var(--rg-breaking);
}

.rg-strata__seg--deprecation {
  background: repeating-linear-gradient(-45deg, var(--rg-deprecation) 0 2px, transparent 2px 5px);
  box-shadow: inset 0 0 0 1px var(--rg-deprecation);
}

.rg-strata__seg--security {
  background: radial-gradient(circle, var(--rg-security) 1.2px, transparent 1.6px) 0 0 / 5px 5px;
  box-shadow: inset 0 0 0 1px var(--rg-security);
}

.rg-strata__seg--feature {
  background: var(--rg-feature);
}

.rg-strata__seg--fix {
  background: color-mix(in srgb, var(--rg-fix) 50%, var(--rg-surface));
}

.rg-strata__seg--other {
  box-shadow: inset 0 0 0 1px var(--rg-other);
}

.rg-strata__count {
  color: var(--rg-ink);
  font-family: var(--vp-font-family-mono);
  font-size: 0.875rem;
  text-align: right;
}

.rg-strata__fault {
  position: relative;
  display: flex;
  justify-content: flex-end;
  margin: 0.15rem 0;
  border-top: 2px dashed var(--rg-breaking);
}

.rg-strata__fault-text {
  margin-top: 0.1rem;
  color: var(--rg-breaking);
  font-size: 0.8125rem;
}

.rg-strata .rg-strata__legend li {
  margin: 0;
}

.rg-strata__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem 1rem;
  margin: 1rem 0 0;
  padding: 0;
  list-style: none;
  align-items: center;
  color: var(--rg-ink-muted);
  font-size: 0.8125rem;
}

.rg-strata--animate .rg-strata__row {
  animation: rg-strata-settle 220ms ease-out both;
  animation-delay: calc(var(--step, 0) * 45ms);
}

.rg-strata--animate .rg-strata__fault {
  animation: rg-strata-fault 200ms ease-out both;
  animation-delay: calc(var(--steps) * 45ms + 100ms);
  transform-origin: left center;
}

@keyframes rg-strata-settle {
  from {
    opacity: 0;
    transform: translateY(-6px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}

@keyframes rg-strata-fault {
  from {
    transform: scaleX(0);
  }
  to {
    transform: scaleX(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .rg-strata--animate .rg-strata__row,
  .rg-strata--animate .rg-strata__fault {
    animation: none;
  }
}

@media (max-width: 420px) {
  .rg-strata {
    padding-inline: 0.9rem;
  }

  .rg-strata__row {
    grid-template-columns: 2.9rem 3.5rem 1fr 2rem;
    gap: 0.4rem;
  }
}
</style>
