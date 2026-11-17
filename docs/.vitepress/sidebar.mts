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
    text: "Guides",
    items: [
      { text: "Read the changes between two versions", link: "/guides/digest" },
      { text: "Find pins behind latest or past EOL", link: "/guides/outdated" },
      { text: "Find version drift across repos", link: "/guides/drift" },
      { text: "Gate CI on breaking changes, EOL and vulnerabilities", link: "/guides/ci-gate" },
      { text: "Comment upgrade notes on Renovate pull requests", link: "/guides/renovate-pr" },
      { text: "Use relgap-action", link: "/guides/action" },
      { text: "Upload SARIF to code scanning", link: "/guides/sarif" },
      { text: "Feed tfcensus inventory into drift", link: "/guides/from-tfcensus" },
      { text: "Map unresolved pins and private tools", link: "/guides/unresolved" },
      { text: "Use Renovate's extract report as a pin source", link: "/guides/renovate-report" },
      { text: "Write or fix a registry entry", link: "/guides/registry-entry" },
      { text: "Use relgap from an MCP client", link: "/guides/mcp" },
      { text: "Add an LLM summary", link: "/guides/summarize" },
      { text: "Work offline and manage the cache", link: "/guides/cache" },
      { text: "Shell completion", link: "/guides/completion" },
    ],
  },
  {
    text: "Concepts",
    items: [
      { text: "Pins and detectors", link: "/concepts/pins" },
      { text: "Tool resolution", link: "/concepts/resolution" },
      { text: "Versions, ranges and release lines", link: "/concepts/ranges" },
      { text: "Release-note sources", link: "/concepts/sources" },
      { text: "Classification and confidence", link: "/concepts/classification" },
      { text: "EOL and vulnerabilities", link: "/concepts/enrichment" },
      { text: "Drift", link: "/concepts/drift" },
      { text: "Network, privacy and tokens", link: "/concepts/network" },
    ],
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
      { text: "Notify targets", link: "/reference/notify" },
      { text: "Output formats", link: "/reference/output" },
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
  {
    text: "Help",
    items: [
      { text: "Troubleshooting", link: "/troubleshooting" },
      { text: "FAQ", link: "/faq" },
      { text: "Security and verification", link: "/security" },
      { text: "Contributing", link: "/contributing" },
    ],
  },
];
