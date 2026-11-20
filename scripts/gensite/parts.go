package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/enrich"
	"github.com/sassdavid/relgap/internal/httpx"
	"github.com/sassdavid/relgap/internal/registry"
	"github.com/sassdavid/relgap/internal/registry/remote"
	"github.com/sassdavid/relgap/internal/scan"
	"github.com/sassdavid/relgap/internal/source"
	"github.com/sassdavid/relgap/internal/summarize"
	"github.com/sassdavid/relgap/internal/version"
)

// partsDir holds the generated fragments that hand-written pages include; the site doesn't
// build them as pages (srcExclude in config.mts).
const partsDir = "docs/parts/"

// partsFiles writes the generated tables of the concept pages.
func partsFiles(_ string, files map[string][]byte) error {
	files[partsDir+"detectors.md"] = part(detectorsTable())
	files[partsDir+"source-kinds.md"] = part(sourceKindsTable())
	p, err := placeholdersTable()
	if err != nil {
		return err
	}
	files[partsDir+"placeholders.md"] = part(p)
	files[partsDir+"web-hosts.md"] = part(webHostsList())
	files[partsDir+"confidence.md"] = part(confidenceTable())
	hosts, err := networkHosts()
	if err != nil {
		return err
	}
	files[partsDir+"network-hosts.md"] = part(networkHostsTable(hosts))
	return nil
}

func detectorsTable() string {
	t := &table{head: []string{"Detector", "Files", "Pins"}}
	for _, d := range scan.DetectorDocs() {
		t.add(code(d.Name), prose(d.Files), prose(d.Pins))
	}
	return t.String()
}

func sourceKindsTable() string {
	t := &table{head: []string{"Kind", "Reads"}}
	for _, k := range source.Kinds() {
		t.add(code(k.Kind), prose(k.Reads))
	}
	return t.String()
}

// placeholderMeaning says what each placeholder of [registry.Placeholders] stands for.
func placeholderMeaning() map[string]string {
	return map[string]string{
		registry.PlaceholderVersion:   "the version",
		registry.PlaceholderMajor:     "the major line",
		registry.PlaceholderMinor:     "the minor number alone",
		registry.PlaceholderTag:       "the Git tag of the version",
		registry.PlaceholderPrevMinor: "the minor line of the version before (upgrade-guide only)",
		registry.PlaceholderPrevMajor: "the major line of the version before (upgrade-guide only)",
	}
}

// Sample versions for the placeholder table: the values come from [source.ExpandPrev].
const (
	sampleVersion = "1.32.4"
	sampleTag     = "v1.32.4"
	samplePrev    = "1.31.2"
)

func placeholdersTable() (string, error) {
	s, err := version.NewScheme(version.SchemeSemver, "")
	if err != nil {
		return "", fmt.Errorf("semver scheme: %w", err)
	}
	v, err := s.Parse(sampleVersion)
	if err != nil {
		return "", fmt.Errorf("parse sample version: %w", err)
	}
	prev, err := s.Parse(samplePrev)
	if err != nil {
		return "", fmt.Errorf("parse sample version: %w", err)
	}
	t := &table{head: []string{"Placeholder", "Stands for", "For " + sampleTag + ", after " + samplePrev}}
	for _, p := range registry.Placeholders() {
		meaning, known := placeholderMeaning()[p]
		if !known {
			return "", fmt.Errorf("placeholder %s has no meaning in gensite", p)
		}
		got := source.ExpandPrev(p, s, version.Version{V: v, Tag: sampleTag}, prev)
		if got == p {
			return "", fmt.Errorf("source.ExpandPrev leaves %s as it is", p)
		}
		t.add(code(p), meaning, code(got))
	}
	return t.String(), nil
}

func webHostsList() string {
	var b strings.Builder
	for _, h := range registry.WebHosts() {
		b.WriteString("- `" + h + "`\n")
	}
	return b.String()
}

func confidenceTable() string {
	t := &table{head: []string{"Confidence", "Evidence"}}
	for _, s := range classify.Signals() {
		t.add(strconv.FormatFloat(s.Confidence, 'f', -1, 64), prose(s.Evidence))
	}
	return t.String()
}

const summaryPrompt = "the summary prompt"

// host is one row of the network host table.
type host struct {
	name, purpose, when string
	token               bool
}

// networkHosts returns every host relgap sends requests to. The test checks it against the
// hosts in the cassettes.
func networkHosts() ([]host, error) {
	var parsed []*url.URL
	for _, raw := range []string{
		enrich.EndOfLifeURL, summarize.AnthropicBaseURL, summarize.MantleBaseURL("REGION", summarize.APIMessages),
	} {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", raw, err)
		}
		parsed = append(parsed, u)
	}
	eol, anthropic, mantle := parsed[0], parsed[1], parsed[2]
	hosts := []host{
		{
			httpx.GitHubAPIHost, "releases, tags, files and repository search (REST and GraphQL), the " +
				"`index.yaml` of a GitHub Pages Helm repository, and the pull request comment of `relgap notify`",
			"every command that looks up a tool", true,
		},
		{eol.Host, "release cycles and their end-of-life dates", "a tool whose registry entry names an " +
			"endoflife.date product", false},
		{httpx.OSVHost, "known vulnerabilities of the pinned and latest versions", "a tool whose registry " +
			"entry names OSV packages", false},
		{
			remote.TerraformRegistryHost, "provider versions and source repositories", "`tfprovider:` pins and inputs",
			false,
		},
		{remote.ArtifactHubHost, "chart versions and source repositories", "`chart:` pins and inputs", false},
		{remote.NPMHost, "package repositories", "`npm:` inputs", false},
		{remote.PyPIHost, "package repositories", "`pypi:` inputs", false},
	}
	for _, h := range registry.WebHosts() {
		hosts = append(hosts, host{h, "release-note pages", "a registry entry with a web-doc, structured-json " +
			"or upgrade-guide source on this host", false})
	}
	hosts = append(hosts,
		host{anthropic.Host, summaryPrompt, "`--summarize` with `llm.provider: anthropic` and no " +
			"`llm.base_url`", false},
		host{
			strings.Replace(mantle.Host, "REGION", "<region>", 1), summaryPrompt,
			"`--summarize` with `llm.provider: bedrock-mantle` and no `llm.base_url`", false,
		},
		host{
			"the Bedrock Runtime endpoint of the AWS region", summaryPrompt,
			"`--summarize` with `llm.provider: bedrock`", false,
		},
		host{
			"the host of `llm.base_url`", summaryPrompt,
			"`--summarize` with `llm.provider: openai-compatible`, or any provider with `llm.base_url` set", false,
		},
	)
	return hosts, nil
}

func networkHostsTable(hosts []host) string {
	t := &table{head: []string{"Host", "What relgap asks for", "When", "GitHub token"}}
	for _, h := range hosts {
		name := code(h.name)
		if strings.Contains(h.name, " ") {
			name = prose(h.name)
		}
		token := "no"
		if h.token {
			token = "yes, when one is found"
		}
		t.add(name, prose(h.purpose), prose(h.when), token)
	}
	return t.String()
}

// sentToModelFile writes the data of the SentToModel component.
func sentToModelFile(_ string, files map[string][]byte) error {
	b, err := jsonFile(summarize.SentToModel())
	if err != nil {
		return err
	}
	files["docs/.vitepress/theme/data/sent-to-model.json"] = b
	return nil
}
