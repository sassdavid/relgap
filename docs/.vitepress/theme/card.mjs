// The relgap part of the social cards (docs-design.md "Social cards"): the site's Atkinson
// Hyperlegible Next variable font and three thin strata along the right edge, coloured by the class
// mix of the three largest releases in the home page sample. social.mjs draws the rest of the card.
import { readFileSync } from "node:fs";

// The fontsource files the site serves, at the weights docs-design.md gives.
export const font = {
  file: "@fontsource-variable/atkinson-hyperlegible-next/files/atkinson-hyperlegible-next-latin-wght-normal.woff2",
  license: "@fontsource-variable/atkinson-hyperlegible-next/LICENSE",
  title: 700,
  wordmark: 800,
};

const strataWidth = 240;
const gutter = 48;

// The title stops short of the strata.
export const titleWidth = 1040 - strataWidth - gutter;

const sample = JSON.parse(
  readFileSync(new URL("./data/home-strata.json", import.meta.url), "utf8"),
);
const classes = ["breaking", "deprecation", "security", "feature", "fix", "other"];

/**
 * Draws the three strata: the sample's three largest releases, newest on top. The bars get
 * shorter towards the bottom, as in the glyph, because one large release would shrink the others
 * to slivers. A class segment is at least 6 px wide so a few breaking entries stay visible.
 * @param {import("@napi-rs/canvas").SKRSContext2D} ctx
 * @param {Record<string, string>} tokens light theme colours
 * @param {{ width: number, height: number, margin: number }} card
 */
export function art(ctx, tokens, card) {
  const layers = sample.layers
    .map((layer, order) => ({ layer, order }))
    .toSorted((a, b) => b.layer.total - a.layer.total)
    .slice(0, 3)
    .toSorted((a, b) => b.order - a.order)
    .map((l) => l.layer);
  const lengths = [strataWidth, Math.round(strataWidth * 0.8), Math.round(strataWidth * 0.6)];
  const bar = 14;
  const gap = 12;
  const right = card.width - card.margin;
  const top = 250;
  layers.forEach((layer, i) => {
    const present = classes.filter((c) => layer.counts[c] > 0);
    const min = 6;
    const spare = lengths[i] - min * present.length;
    let x = right - lengths[i];
    for (const c of present) {
      const w = min + (spare * layer.counts[c]) / layer.total;
      ctx.fillStyle = tokens[c];
      ctx.fillRect(x, top + i * (bar + gap), w, bar);
      x += w;
    }
  });
}
