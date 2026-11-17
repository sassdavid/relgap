<script setup lang="ts">
// The home page: headline, lead, install command, actions and the release strata, read from the
// page's frontmatter and the strata fixture written by scripts/gensite.
import { useData, withBase } from "vitepress";
import { ref } from "vue";
import strata from "../data/home-strata.json";
import StrataStack, { type Layer } from "./StrataStack.vue";

interface Action {
  text: string;
  link: string;
  theme?: "brand" | "alt";
}

const { frontmatter } = useData();
const copied = ref(false);

async function copy(text: string) {
  await navigator.clipboard.writeText(text);
  copied.value = true;
  setTimeout(() => (copied.value = false), 1500);
}

function href(link: string): string {
  return /^https?:/.test(link) ? link : withBase(link);
}

const actions = (frontmatter.value.actions ?? []) as Action[];
const layers = strata.layers as Layer[];
</script>

<template>
  <section class="rg-hero">
    <div class="rg-hero__text">
      <h1 class="rg-hero__headline">{{ frontmatter.headline }}</h1>
      <p class="rg-hero__lead">{{ frontmatter.lead }}</p>
      <div class="rg-hero__install">
        <code
          ><span class="rg-hero__prompt" aria-hidden="true">$ </span>{{ frontmatter.install }}</code
        >
        <button
          type="button"
          class="rg-hero__copy"
          :aria-label="`Copy the install command: ${frontmatter.install}`"
          @click="copy(frontmatter.install)"
        >
          {{ copied ? "Copied" : "Copy" }}
        </button>
        <span class="rg-sr-only" aria-live="polite">{{ copied ? "Copied" : "" }}</span>
      </div>
      <div class="rg-hero__actions">
        <a
          v-for="action in actions"
          :key="action.link"
          class="rg-button"
          :class="action.theme ?? 'brand'"
          :href="href(action.link)"
          >{{ action.text }}</a
        >
      </div>
    </div>
    <StrataStack
      class="rg-hero__strata"
      :tool="strata.tool"
      :from="strata.from"
      :to="strata.to"
      :layers="layers"
      :breaking="strata.breaking"
    />
  </section>
</template>

<style scoped>
.rg-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 27rem);
  gap: 3rem;
  align-items: center;
  padding: 3.5rem 0 3rem;
}

.rg-hero__headline {
  margin: 0;
  color: var(--rg-ink);
  font-size: 2.8rem;
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: 0;
}

.rg-hero__lead {
  max-width: 34rem;
  margin: 1.25rem 0 0;
  color: var(--rg-ink-muted);
  font-size: 1.125rem;
  line-height: 1.6;
}

.rg-hero__install {
  display: inline-flex;
  align-items: center;
  gap: 0.75rem;
  max-width: 100%;
  margin-top: 1.75rem;
  padding: 0.5rem 0.5rem 0.5rem 0.9rem;
  border: 1px solid var(--rg-rule);
  border-radius: 2px;
  background: var(--rg-surface);
}

.rg-hero__install code {
  overflow-wrap: anywhere;
  color: var(--rg-ink);
  font-family: var(--vp-font-family-mono);
  font-size: 0.9375rem;
}

.rg-hero__prompt {
  color: var(--rg-ink-muted);
}

.rg-hero__copy {
  flex: none;
  min-width: 4rem;
  min-height: 2rem;
  padding: 0 0.75rem;
  border: 1px solid var(--rg-control);
  border-radius: 2px;
  background: var(--rg-surface);
  color: var(--rg-ink);
  font-size: 0.875rem;
}

.rg-hero__copy:hover {
  border-color: var(--rg-brand);
  color: var(--rg-brand);
}

.rg-hero__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  margin-top: 1.5rem;
}

.rg-button {
  display: inline-flex;
  align-items: center;
  min-height: 2.75rem;
  padding: 0 1.25rem;
  border: 1px solid var(--rg-brand);
  border-radius: 2px;
  font-weight: 650;
  text-decoration: none;
  transition:
    background-color 0.2s,
    color 0.2s,
    border-color 0.2s;
}

.rg-button.brand {
  background: var(--rg-brand);
  color: var(--rg-paper);
}

.rg-button.brand:hover {
  border-color: var(--rg-ink);
  background: var(--rg-ink);
}

.rg-button.alt {
  border-color: var(--rg-control);
  background: var(--rg-surface);
  color: var(--rg-ink);
}

.rg-button.alt:hover {
  border-color: var(--rg-brand);
  color: var(--rg-brand);
}

@media (max-width: 959px) {
  .rg-hero {
    grid-template-columns: minmax(0, 1fr);
    gap: 2.5rem;
    padding-top: 2rem;
  }

  .rg-hero__headline {
    font-size: 2.1rem;
  }
}
</style>
