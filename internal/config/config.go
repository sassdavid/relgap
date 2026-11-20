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
	Ignore        []IgnoreRule            `json:"ignore"                             jsonschema_description:"Pins that are never reported: by tool, by raw name or by path, alone or together."`
	Policy        Policy                  `json:"policy"`
	LLM           LLM                     `json:"llm"`
	GitLab        GitLab                  `json:"gitlab"`
	Images        Images                  `json:"images"`
}

// GitHub configures the GitHub API client.
type GitHub struct {
	Host        string `json:"host"        jsonschema:"minLength=1"          jsonschema_description:"Reserved for GitHub Enterprise Server. relgap doesn't read it yet and always calls github.com."`
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
	ExtraFiles []string  `json:"extra_files" jsonschema_description:"Extra glob patterns read by the renovate-comment detector, and by the generic detector when it is on."`
}

// Detectors toggles opt-in pin detectors.
type Detectors struct {
	CIActions bool `json:"ci_actions" jsonschema_description:"Also report the uses: pins of GitHub Actions in workflow files. Tool versions in CI files are read either way."`
	Generic   bool `json:"generic"    jsonschema_description:"Detect NAME_VERSION=x.y.z assignments in extra_files."`
}

// ToolOverride is a local registry entry.
type ToolOverride struct {
	Repo string `json:"repo"           jsonschema:"required,pattern=^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+$" jsonschema_description:"owner/repo on GitHub, workspace/repo on Bitbucket Cloud, or the project path on GitLab."`
	Host string `json:"host,omitempty" jsonschema:"pattern=^[A-Za-z0-9.-]+$"                              jsonschema_description:"Host of repo: github.com (the default), bitbucket.org, gitlab.com or a host in gitlab.hosts."`
}

// GitLab configures self-managed GitLab hosts.
type GitLab struct {
	Hosts []GitLabHost `json:"hosts" jsonschema_description:"Self-managed GitLab hosts. A git source on a host that isn't listed stays unresolved."`
}

// GitLabHost is one self-managed GitLab host.
type GitLabHost struct {
	Host     string `json:"host"                jsonschema:"required,pattern=^[A-Za-z0-9.-]+(:[0-9]+)?$" jsonschema_description:"Host name, such as gitlab.example.com."`
	TokenEnv string `json:"token_env,omitempty" jsonschema:"pattern=^[A-Za-z_][A-Za-z0-9_]*$"            jsonschema_description:"Env var with this host's token, sent in PRIVATE-TOKEN to this host only. Empty means GITLAB_TOKEN when GITLAB_HOST names this host."`
}

// Images configures the registries outdated reads image and oci:// chart tags from.
type Images struct {
	Registries []string `json:"registries" jsonschema_description:"Registries to read images and oci:// charts from besides Docker Hub, ghcr.io, registry.k8s.io, quay.io, public.ecr.aws and mcr.microsoft.com. relgap asks every registry for anonymous tokens only."`
}

// IgnoreRule suppresses the pins that match every field it sets: a tool, a raw name, a path.
type IgnoreRule struct {
	Tool string `json:"tool,omitempty" jsonschema:"minLength=1,anyof_required=tool" jsonschema_description:"Tool name, as pins and digests show it, to leave out of reports."`
	Name string `json:"name,omitempty" jsonschema:"minLength=1,anyof_required=name" jsonschema_description:"Glob on the pin's raw name, as relgap pins shows it; * matches any characters. Matches unresolved pins too."`
	Path string `json:"path,omitempty" jsonschema:"anyof_required=path"             jsonschema_description:"Glob on the file path; with tool or name, the rule applies only under matching paths."`
}

// Policy holds CI defaults for --fail-on and related flags.
type Policy struct {
	FailOn        []string `json:"fail_on"        jsonschema:"enum=breaking,enum=deprecation,enum=eol,enum=vuln,enum=drift,enum=outdated" jsonschema_description:"--fail-on values each command uses when the flag is not given; a command ignores the values it doesn't check."`
	MaxBehind     string   `json:"max_behind"     jsonschema:"pattern=^((major|minor|patch|releases|days):[0-9]+)?$"                      jsonschema_description:"e.g. minor:2, releases:10, days:90."`
	MinConfidence float64  `json:"min_confidence" jsonschema:"minimum=0,maximum=1"                                                        jsonschema_description:"Entries below this confidence are hidden."`
	VulnSeverity  string   `json:"vuln_severity"  jsonschema:"enum=,enum=low,enum=medium,enum=high,enum=critical"                         jsonschema_description:"Lowest OSV rating --fail-on vuln counts; empty counts every advisory."`
}

// LLM configures the optional, off-by-default summarizer.
type LLM struct {
	Provider       string `json:"provider"         jsonschema:"enum=,enum=openai-compatible,enum=anthropic,enum=bedrock,enum=bedrock-mantle" jsonschema_description:"Empty disables the summarizer."`
	Model          string `json:"model"                                                                                                      jsonschema_description:"Model ID the provider runs. Required when provider is set."`
	BaseURL        string `json:"base_url"                                                                                                   jsonschema_description:"API base URL. Required for openai-compatible; anthropic and bedrock-mantle have a default."`
	APIKeyEnv      string `json:"api_key_env"                                                                                                jsonschema_description:"Name of the env var holding the API key. Keys are never read from files."`
	API            string `json:"api"              jsonschema:"enum=,enum=messages,enum=chat-completions"                                    jsonschema_description:"bedrock-mantle only. Empty means messages for anthropic.* model IDs and chat-completions otherwise."`
	AWSProfile     string `json:"aws_profile"                                                                                                jsonschema_description:"AWS profile for bedrock and bedrock-mantle. Empty means the default credentials."`
	MaxInputTokens int    `json:"max_input_tokens" jsonschema:"minimum=1"                                                                    jsonschema_description:"Largest prompt, in estimated tokens. Lower-confidence entries are dropped first to fit."`
	Timeout        string `json:"timeout"          jsonschema:"pattern=^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"                           jsonschema_description:"Hard limit on the model call, as a Go duration. A timeout is a warning."`
}

const (
	defaultConcurrency    = 8
	defaultMinConfidence  = 0.5
	defaultMaxInputTokens = 30000
	defaultLLMTimeout     = "120s"
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
		GitLab:  GitLab{Hosts: []GitLabHost{}},
		Images:  Images{Registries: []string{}},
		Ignore:  []IgnoreRule{},
		Policy: Policy{
			FailOn:        []string{},
			MaxBehind:     "",
			MinConfidence: defaultMinConfidence,
			VulnSeverity:  "",
		},
		LLM: LLM{MaxInputTokens: defaultMaxInputTokens, Timeout: defaultLLMTimeout},
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
