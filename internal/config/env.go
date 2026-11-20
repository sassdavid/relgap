package config

// EnvVars returns the environment variables relgap reads besides RELGAP_CONFIG and the
// RELGAP_<KEY> config variables. A test in scripts/gendocs checks the list against the code.
func EnvVars() []EnvVar {
	return []EnvVar{
		{"GH_TOKEN", "GitHub token, tried first. Without a token relgap reads gh's config file, then runs " +
			"gh auth token, and otherwise sends unauthenticated requests (60 per hour)."},
		{"GITHUB_TOKEN", "GitHub token, tried when GH_TOKEN is not set."},
		{"BITBUCKET_API_TOKEN", "Bitbucket Cloud API token or access token, sent as Bearer to api.bitbucket.org " +
			"only. Without it relgap sends anonymous requests (60 per hour per IP)."},
		{"GITLAB_TOKEN", "GitLab token, sent in PRIVATE-TOKEN only to the host in GITLAB_HOST (default gitlab.com), " +
			"unless a gitlab.hosts entry names its own token_env."},
		{"GITLAB_ACCESS_TOKEN", "GitLab token, tried when GITLAB_TOKEN is not set."},
		{"GITLAB_HOST", "The GitLab host that GITLAB_TOKEN belongs to; default gitlab.com."},
		{"CI_JOB_TOKEN", "GitLab CI job token, sent in JOB-TOKEN only to CI_SERVER_HOST, when that host has no " +
			"other token."},
		{"CI_SERVER_HOST", "The GitLab host CI_JOB_TOKEN belongs to; GitLab CI sets it."},
		{"ANTHROPIC_API_KEY", "API key for --summarize with the anthropic provider when llm.api_key_env is empty."},
		{"AWS_*", "The AWS SDK reads its usual variables for --summarize with the bedrock provider, such as " +
			"AWS_REGION and AWS_PROFILE."},
		{"GITHUB_REPOSITORY", "Repository for the github-pr-comment notifier when the config names none."},
		{"GITHUB_API_URL", "GitHub API URL for the github-pr-comment notifier when the config names none."},
		{"GITHUB_EVENT_PATH", "Event file that digest --pull-request and github-pr-comment read the pull " +
			"request from in GitHub Actions."},
		{
			"HOME",
			"Home directory: the config and cache directories, and mise's data directory for outdated --binaries.",
		},
		{"XDG_CONFIG_HOME", "Base of the user config file: $XDG_CONFIG_HOME/relgap/config.yaml."},
		{"XDG_CACHE_HOME", "Base of the cache directory: $XDG_CACHE_HOME/relgap."},
		{"XDG_DATA_HOME", "Base of mise's data directory for outdated --binaries when MISE_DATA_DIR is not set."},
		{"MISE_DATA_DIR", "mise's data directory, whose installs outdated --binaries looks at."},
		{"NO_COLOR", "Turns color off with --color auto."},
		{"CLICOLOR_FORCE", "Turns color on with --color auto, even when output is not a terminal."},
		{"PATH", "Where outdated --binaries looks for installed tools, and where mise, gh and git are looked up."},
		{"PATHEXT", "File extensions of programs on Windows, for the same lookup."},
	}
}
