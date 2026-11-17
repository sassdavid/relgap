// The sidebar: hand-written sections, then the CLI tree that scripts/gendocs writes to
// cli/commands.json (docs-site.md "CLI reference").
import { readFileSync } from "node:fs";
import type { DefaultTheme } from "vitepress";

// CommandNode is one entry of cli/commands.json.
export type CommandNode = {
  name: string;
  path: string;
  short: string;
  hidden: boolean;
  children?: CommandNode[];
};

const commands = JSON.parse(
  readFileSync(new URL("../cli/commands.json", import.meta.url), "utf8"),
) as CommandNode;

function cliItem(node: CommandNode): DefaultTheme.SidebarItem {
  const children = (node.children ?? []).filter((c) => !c.hidden);
  if (children.length === 0) {
    return { text: node.name, link: node.path };
  }
  return { text: node.name, link: node.path, collapsed: true, items: children.map(cliItem) };
}

export const sidebar: DefaultTheme.SidebarItem[] = [
  {
    text: "Start",
    items: [
      { text: "Getting started", link: "/getting-started" },
      { text: "Install", link: "/install" },
    ],
  },
  {
    text: "Concepts",
    items: [{ text: "Network, privacy and tokens", link: "/concepts/network" }],
  },
  {
    text: "Reference",
    items: [
      { text: "Configuration", link: "/reference/config" },
      { text: "Environment variables", link: "/reference/environment" },
      { text: "Exit codes", link: "/reference/exit-codes" },
      { text: "Fail-on values", link: "/reference/fail-on" },
      { text: "JSON schemas", link: "/reference/schemas" },
      { text: "MCP tools and prompts", link: "/reference/mcp" },
      { text: "SARIF rules", link: "/reference/sarif-rules" },
      { text: "Supported tools", link: "/reference/tools" },
      { text: "Registry entry schema", link: "/reference/registry-schema" },
      { text: "Registry snapshots", link: "/reference/snapshots" },
      { text: "Changelog", link: "/changelog" },
    ],
  },
  {
    text: "CLI",
    collapsed: true,
    items: [
      { text: "Overview", link: "/cli/" },
      { text: "Global flags", link: "/cli/global-flags" },
      ...(commands.children ?? []).filter((c) => !c.hidden).map(cliItem),
    ],
  },
];
