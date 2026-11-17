<script setup lang="ts">
// The footer in the layout-bottom slot: license, source and releases. On pages with a sidebar it
// starts where the content column starts.
import { useSidebar } from "vitepress/theme-without-fonts";

const { hasSidebar } = useSidebar();
const repo = "https://github.com/sassdavid/relgap";
</script>

<template>
  <footer class="rg-footer" :class="{ 'has-sidebar': hasSidebar }">
    <ul class="rg-footer__links">
      <li><a :href="`${repo}/blob/main/LICENSE`">Apache-2.0</a></li>
      <li><a :href="repo">Source</a></li>
      <li><a :href="`${repo}/releases`">Releases</a></li>
      <li><a href="https://sasskovacs.dev">sasskovacs.dev</a></li>
    </ul>
  </footer>
</template>

<style scoped>
.rg-footer {
  padding: 1.5rem 1.5rem 2rem;
  border-top: 1px solid var(--rg-rule);
  color: var(--rg-ink-muted);
  font-size: 0.875rem;
}

.rg-footer__links {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem 1.5rem;
  max-width: 1152px;
  margin: 0 auto;
  padding: 0;
  list-style: none;
}

.rg-footer a {
  display: inline-flex;
  align-items: center;
  min-height: 1.5rem;
  color: var(--rg-ink-muted);
  text-decoration: underline;
  text-underline-offset: 2px;
}

.rg-footer a:hover {
  color: var(--rg-brand);
}

@media (min-width: 960px) {
  .rg-footer.has-sidebar {
    padding-left: calc(var(--vp-sidebar-width) + 2rem);
  }
}

@media (min-width: 1440px) {
  .rg-footer.has-sidebar {
    padding-left: calc(
      (100% - (var(--vp-layout-max-width) - 64px)) / 2 + var(--vp-sidebar-width) - 32px + 2rem
    );
  }
}
</style>
