package config

import (
	"encoding/json"
	"fmt"
)

// SchemaVersion is the config schema_version this binary reads.
const SchemaVersion = 1

// Config defines every configuration key. The JSON tags are the key names, and
// schemas/config-v1.json is generated from this type.
type Config struct {
	SchemaVersion int                     `json:"schema_version" jsonschema:"enum=1" jsonschema_description:"Config file format version."`
	GitHub        GitHub                  `json:"github"`
	Cache         Cache                   `json:"cache"`
	Scan          Scan                    `json:"scan"`
	Tools         map[string]ToolOverride `json:"tools"                              jsonschema_description:"Local tool overrides and additions, keyed by tool name."`
	Aliases       map[string]string       `json:"aliases"                            jsonschema_description:"Maps a raw pin name (e.g. TG_VERSION) to a tool name."`
	Ignore        []IgnoreRule            `json:"ignore"                             jsonschema_description:"Tools (optionally limited to paths) that are never reported."`
	Policy        Policy                  `json:"policy"`
	LLM           LLM                     `json:"llm"`
}

// GitHub configures the GitHub API client.
type GitHub struct {
	Host        string `json:"host"        jsonschema:"minLength=1"          jsonschema_description:"GitHub host. Only github.com is supported in v0.x."`
	Concurrency int    `json:"concurrency" jsonschema:"minimum=1,maximum=64" jsonschema_description:"Maximum concurrent requests per tool."`
}

// Cache configures the on-disk HTTP cache.
type Cache struct {
	Dir         string            `json:"dir"          jsonschema_description:"Cache directory. Empty means $XDG_CACHE_HOME/relgap."`
	TTLOverride map[string]string `json:"ttl_override" jsonschema_description:"Per-source TTL overrides as Go durations, e.g. {endoflife: 24h}."`
}

// Scan configures pin discovery.
type Scan struct {
	Roots      []string  `json:"roots"       jsonschema_description:"Directories scanned by drift and pins when no roots are given."`
	Exclude    []string  `json:"exclude"     jsonschema_description:"Glob patterns excluded from scanning."`
	Detectors  Detectors `json:"detectors"`
	ExtraFiles []string  `json:"extra_files" jsonschema_description:"Extra glob patterns scanned by the generic detector."`
}

// Detectors toggles opt-in pin detectors.
type Detectors struct {
	CIActions bool `json:"ci_actions" jsonschema_description:"Detect tool versions in CI workflow files."`
	Generic   bool `json:"generic"    jsonschema_description:"Detect NAME_VERSION=x.y.z assignments in extra_files."`
}

// ToolOverride is a local registry entry.
type ToolOverride struct {
	Repo string `json:"repo" jsonschema:"required,pattern=^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$" jsonschema_description:"GitHub owner/repo."`
}

// IgnoreRule suppresses a tool, optionally only under matching paths.
type IgnoreRule struct {
	Tool string `json:"tool"           jsonschema:"required,minLength=1"`
	Path string `json:"path,omitempty"                                   jsonschema_description:"Glob; when set, the tool is ignored only under matching paths."`
}

// Policy holds CI defaults for --fail-on and related flags.
type Policy struct {
	FailOn        []string `json:"fail_on"        jsonschema:"enum=breaking,enum=deprecation,enum=eol,enum=vuln,enum=drift,enum=outdated"`
	MaxBehind     string   `json:"max_behind"     jsonschema:"pattern=^((major|minor|patch|releases|days):[0-9]+)?$"                      jsonschema_description:"e.g. minor:2, releases:10, days:90."`
	MinConfidence float64  `json:"min_confidence" jsonschema:"minimum=0,maximum=1"                                                        jsonschema_description:"Entries below this confidence are hidden."`
	VulnSeverity  string   `json:"vuln_severity"  jsonschema:"enum=,enum=low,enum=medium,enum=high,enum=critical"                         jsonschema_description:"Lowest OSV rating --fail-on vuln counts; empty counts every advisory."`
}

// LLM configures the optional, off-by-default summarizer.
type LLM struct {
	Provider       string `json:"provider"         jsonschema:"enum=,enum=openai-compatible,enum=anthropic,enum=bedrock" jsonschema_description:"Empty disables the summarizer."`
	Model          string `json:"model"`
	BaseURL        string `json:"base_url"`
	APIKeyEnv      string `json:"api_key_env"                                                                            jsonschema_description:"Name of the env var holding the API key. Keys are never read from files."`
	AWSProfile     string `json:"aws_profile"`
	MaxInputTokens int    `json:"max_input_tokens" jsonschema:"minimum=1"`
}

const (
	defaultConcurrency    = 8
	defaultMinConfidence  = 0.5
	defaultMaxInputTokens = 30000
)

// Default returns the built-in configuration. Slices and maps are non-nil so that
// config show lists every key.
func Default() Config {
	return Config{
		SchemaVersion: SchemaVersion,
		GitHub:        GitHub{Host: "github.com", Concurrency: defaultConcurrency},
		Cache:         Cache{Dir: "", TTLOverride: map[string]string{}},
		Scan: Scan{
			Roots:      []string{},
			Exclude:    []string{"**/node_modules/**", "**/.terragrunt-cache/**"},
			Detectors:  Detectors{CIActions: false, Generic: false},
			ExtraFiles: []string{},
		},
		Tools:   map[string]ToolOverride{},
		Aliases: map[string]string{},
		Ignore:  []IgnoreRule{},
		Policy: Policy{
			FailOn:        []string{},
			MaxBehind:     "",
			MinConfidence: defaultMinConfidence,
			VulnSeverity:  "",
		},
		LLM: LLM{MaxInputTokens: defaultMaxInputTokens},
	}
}

func toMap(c Config) (map[string]any, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var m map[string]any
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return m, nil
}
