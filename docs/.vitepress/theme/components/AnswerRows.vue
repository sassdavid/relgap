<script setup lang="ts">
// "What it answers": one row per question with the command that answers it and a link. The rows
// are not a sequence, so they carry no numbers.
import { useData, withBase } from "vitepress";

interface Answer {
  question: string;
  command: string;
  link: string;
}

const { frontmatter } = useData();
const answers = (frontmatter.value.answers ?? []) as Answer[];

function href(link: string): string {
  return /^https?:/.test(link) ? link : withBase(link);
}
</script>

<template>
  <section class="rg-answers" aria-labelledby="rg-answers-title">
    <h2 id="rg-answers-title" class="rg-answers__title">What it answers</h2>
    <ul class="rg-answers__list">
      <li v-for="answer in answers" :key="answer.command" class="rg-answers__row">
        <span class="rg-answers__question">{{ answer.question }}</span>
        <code class="rg-answers__command">{{ answer.command }}</code>
        <a class="rg-answers__link" :href="href(answer.link)"
          >Read<span class="rg-sr-only"> about {{ answer.command }}</span></a
        >
      </li>
    </ul>
  </section>
</template>

<style scoped>
.rg-answers {
  padding: 1rem 0 3rem;
}

.rg-answers__title {
  margin: 0 0 0.75rem;
  color: var(--rg-ink);
  font-size: 1.6rem;
  font-weight: 700;
  letter-spacing: 0;
}

.rg-answers__list {
  margin: 0;
  padding: 0;
  list-style: none;
  border-top: 1px solid var(--rg-rule);
}

.rg-answers .rg-answers__row {
  max-width: none;
  margin: 0;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 17rem 4rem;
  column-gap: 2rem;
  gap: 1rem;
  align-items: baseline;
  padding: 0.85rem 0;
  border-bottom: 1px solid var(--rg-rule);
}

.rg-answers__question {
  color: var(--rg-ink);
}

.rg-answers__command {
  padding: 0;
  background: none;
  color: var(--rg-ink);
  font-family: var(--vp-font-family-mono);
  font-size: 0.9375rem;
  white-space: nowrap;
}

.rg-answers__link {
  display: inline-flex;
  align-items: center;
  min-height: 1.5rem;
  color: var(--rg-brand);
  font-weight: 650;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.rg-answers__link:hover {
  color: var(--rg-ink);
}

@media (max-width: 639px) {
  .rg-answers .rg-answers__row {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 0.25rem 1rem;
  }

  .rg-answers__question {
    grid-column: 1 / -1;
  }
}
</style>
