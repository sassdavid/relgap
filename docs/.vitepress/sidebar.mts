// The sidebar: hand-written sections. The CLI tree from cli/commands.json joins it once
// scripts/gendocs writes the nested reference (docs-site.md "CLI reference").
import type { DefaultTheme } from "vitepress";

export const sidebar: DefaultTheme.Sidebar = [
  {
    text: "Start",
    items: [{ text: "Install", link: "/install" }],
  },
];
