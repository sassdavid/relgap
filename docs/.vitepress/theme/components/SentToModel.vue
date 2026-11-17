<script setup lang="ts">
// What --summarize sends to the model provider. scripts/gensite writes the list from
// summarize.SentToModel, which a test checks against the request the code builds.
import fields from "../data/sent-to-model.json";

interface SentField {
  part: string;
  name: string;
  description: string;
}

const all = fields as SentField[];
const request = all.filter((f) => f.part === "request");
const lines = ["upgrade", "entry"].map((part) => ({
  part,
  fields: all.filter((f) => f.part === part),
}));
</script>

<template>
  <aside class="rg-sent" aria-labelledby="rg-sent-title">
    <p id="rg-sent-title" class="rg-sent__title">What --summarize sends to the model provider</p>
    <ul class="rg-sent__list">
      <li v-for="f in request" :key="f.name"><strong>{{ f.name }}</strong>: {{ f.description }}</li>
    </ul>
    <template v-for="line in lines" :key="line.part">
      <p class="rg-sent__line">
        One <code>"type": "{{ line.part }}"</code> JSON line per
        {{ line.part === "upgrade" ? "tool and range" : "listed entry" }}, with:
      </p>
      <ul class="rg-sent__list">
        <li v-for="f in line.fields" :key="f.name">
          <code>{{ f.name }}</code>: {{ f.description }}
        </li>
      </ul>
    </template>
    <p class="rg-sent__line">
      Nothing else: no file path, pin location, repository name or GitHub token.
    </p>
  </aside>
</template>

<style scoped>
.rg-sent {
  margin: 1.5rem 0;
  padding: 1rem 1.25rem;
  border: 1px solid var(--rg-rule);
  border-left: 4px solid var(--rg-security);
  border-radius: 4px;
  background: var(--rg-surface);
  color: var(--rg-ink);
}

.rg-sent__title {
  margin: 0 0 0.5rem;
  font-weight: 700;
}

.rg-sent__line {
  margin: 0.75rem 0 0.25rem;
}

.rg-sent__list {
  margin: 0;
  padding-left: 1.25rem;
}

.rg-sent__list li {
  margin: 0.15rem 0;
}
</style>
