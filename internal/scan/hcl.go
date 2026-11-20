package scan

import (
	"bytes"
	"errors"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty/cty"

	"github.com/sassdavid/relgap/internal/forge"
)

func isTerraform(rel string) bool {
	return strings.HasSuffix(rel, ".tf") || strings.HasSuffix(rel, ".tf.json")
}

// isTerragrunt matches every .hcl file except the provider lock file.
func isTerragrunt(rel string) bool {
	return path.Ext(rel) == ".hcl" && path.Base(rel) != tfLockFile
}

func parseHCL(f file) (*hcl.File, error) {
	var (
		hf    *hcl.File
		diags hcl.Diagnostics
	)
	if strings.HasSuffix(f.rel, ".json") {
		hf, diags = hcljson.Parse(f.data, f.rel)
	} else {
		hf, diags = hclsyntax.ParseConfig(f.data, f.rel, hcl.InitialPos)
	}
	if diags.HasErrors() {
		return nil, errors.New(firstError(diags))
	}
	return hf, nil
}

// firstError formats the first error as "line N: summary: detail", without the file name the
// caller already reports.
func firstError(diags hcl.Diagnostics) string {
	for _, d := range diags {
		if d.Severity != hcl.DiagError {
			continue
		}
		msg := d.Summary
		if d.Detail != "" {
			msg += ": " + d.Detail
		}
		if d.Subject != nil {
			return "line " + strconv.Itoa(d.Subject.Start.Line) + ": " + msg
		}
		return msg
	}
	return diags.Error()
}

// parseProblem returns err when the file holds a version attribute. A file that only holds
// module sources reports no parse error, like any other file without version attributes.
func parseProblem(err error, versioned bool) error {
	if versioned {
		return err
	}
	return nil
}

// staticString evaluates expr without variables or functions.
func staticString(expr hcl.Expression) (string, bool) {
	v, diags := expr.Value(nil)
	if diags.HasErrors() || v.IsNull() || !v.IsKnown() || v.Type() != cty.String {
		return "", false
	}
	return v.AsString(), true
}

// detectTerraform reads terraform { required_version, required_providers } and module blocks.
// A provider without a source is hashicorp/<name>, as in Terraform.
func detectTerraform(f file) ([]Pin, error) {
	required := bytes.Contains(f.data, []byte("required_"))
	if !required && !bytes.Contains(f.data, []byte("module")) {
		return nil, nil
	}
	hf, err := parseHCL(f)
	if err != nil {
		return nil, parseProblem(err, required)
	}
	top, _, _ := hf.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "terraform"}, {Type: "module", LabelNames: []string{"name"}},
	}})
	var pins []Pin
	for _, block := range top.Blocks {
		if block.Type == "module" {
			if p, ok := modulePin(block); ok {
				pins = append(pins, p)
			}
			continue
		}
		pins = append(pins, terraformBlockPins(block)...)
	}
	return pins, nil
}

// terraformBlockPins reads required_version and required_providers in a terraform block.
func terraformBlockPins(block *hcl.Block) []Pin {
	var pins []Pin
	content, _, _ := block.Body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: "required_version"}},
		Blocks:     []hcl.BlockHeaderSchema{{Type: "required_providers"}},
	})
	if attr, ok := content.Attributes["required_version"]; ok {
		if v, isString := staticString(attr.Expr); isString {
			pins = append(pins, Pin{
				RawName: "required_version", Kind: KindTool, Value: v,
				Location: Location{Line: attr.Expr.Range().Start.Line}, lookup: toolTerraform,
			})
		}
	}
	for _, rp := range content.Blocks {
		attrs, _ := rp.Body.JustAttributes()
		for _, attr := range attrs {
			if p, ok := providerPin(attr); ok {
				pins = append(pins, p)
			}
		}
	}
	return pins
}

// modulePin reads a module block: a registry source with a version is a tfmodule pin, and a git
// source with ?ref= is a pin of the repository.
func modulePin(block *hcl.Block) (Pin, bool) {
	content, _, _ := block.Body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: keySource}, {Name: keyVersion}},
	})
	src, ok := attrString(content, keySource)
	if !ok {
		return Pin{}, false
	}
	if v, hasVersion := attrString(content, keyVersion); hasVersion {
		name, isRegistry := registryModule(src)
		if !isRegistry {
			return Pin{}, false
		}
		return tfModulePin(src, name, v, content.Attributes[keyVersion].Expr.Range().Start.Line), true
	}
	return gitSourcePin(src, content.Attributes[keySource].Expr.Range().Start.Line)
}

func attrString(content *hcl.BodyContent, name string) (string, bool) {
	attr, ok := content.Attributes[name]
	if !ok {
		return "", false
	}
	return staticString(attr.Expr)
}

const keySource = "source"

var registryPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// registryModule returns the registry address of a module source, [<host>/]<ns>/<name>/<provider>
// with an optional //subdir, and reports whether src is one.
func registryModule(src string) (string, bool) {
	if strings.Contains(src, "::") || strings.Contains(src, "://") || strings.Contains(src, "?") {
		return "", false
	}
	addr, _, _ := strings.Cut(src, "//")
	parts := strings.Split(addr, "/")
	const short, withHost = 3, 4
	switch {
	case len(parts) == withHost && strings.Contains(parts[0], "."):
		parts = parts[1:]
	case len(parts) == short && !strings.Contains(parts[0], "."):
	default:
		return "", false
	}
	for _, part := range parts {
		if !registryPart.MatchString(part) {
			return "", false
		}
	}
	return providerName(addr), true
}

func tfModulePin(raw, name, version string, line int) Pin {
	return Pin{
		Tool: "tfmodule:" + name, RawName: raw, Kind: KindTFModule, Value: version,
		Location: Location{Line: line},
	}
}

// gitSourcePin reads a git module source with ?ref=. A repository on github.com is a
// github:owner/repo pin; one on another host is named in [Scanner.finish], which knows the
// GitLab hosts of the config, and stays unresolved on any other host.
func gitSourcePin(src string, line int) (Pin, bool) {
	addr, query, _ := strings.Cut(strings.TrimPrefix(src, "git::"), "?")
	q, err := url.ParseQuery(query)
	if err != nil || q.Get("ref") == "" {
		return Pin{}, false
	}
	p := Pin{RawName: withoutParam(src, "ref"), Kind: KindTool, Value: q.Get("ref"), Location: Location{Line: line}}
	if repo, ok := githubSource(addr); ok {
		p.lookup = prefixGitHub + repo
	} else if host, path, isRemote := gitRemote(addr); isRemote && host != forge.GitHub {
		p.git = &gitSource{host: host, path: path}
	}
	return p, true
}

// githubSource returns owner/repo of a github.com module address in the forms Terraform accepts:
// github.com/o/r, https://github.com/o/r.git, git@github.com:o/r.git and
// ssh://git@github.com/o/r.git, each with an optional //subdir.
func githubSource(addr string) (string, bool) {
	host, repo, ok := gitRemote(addr)
	if !ok || host != forge.GitHub || strings.Count(repo, "/") != 1 || !registryRepo.MatchString(repo) {
		return "", false
	}
	return repo, true
}

var (
	remotePart = regexp.MustCompile(`^[A-Za-z0-9_.~-]+$`)
	remoteHost = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
)

// gitRemote returns the lowercase host and the repository path of a git address in the forms
// Terraform and Renovate write: https://host/path.git, ssh://git@host[:port]/path, git@host:path
// and host/path, each with an optional git:: prefix and //subdir. The host must have a dot. A
// forge's SSH host, such as ssh.bitbucket.org, comes back as its web host.
func gitRemote(addr string) (string, string, bool) {
	s := strings.TrimPrefix(addr, "git::")
	scheme := ""
	for _, sc := range []string{"https://", "http://", "ssh://", "git://"} {
		if rest, ok := strings.CutPrefix(s, sc); ok {
			s, scheme = rest, sc
			break
		}
	}
	if at := strings.IndexByte(s, '@'); at >= 0 && (strings.IndexByte(s, '/') < 0 || at < strings.IndexByte(s, '/')) {
		s = s[at+1:]
	}
	var host, repo string
	if scheme == "" {
		i := strings.IndexAny(s, ":/")
		if i <= 0 {
			return "", "", false
		}
		host, repo = s[:i], s[i+1:]
	} else {
		host, repo, _ = strings.Cut(s, "/")
		host, _, _ = strings.Cut(host, ":")
	}
	if i := strings.Index(repo, "//"); i >= 0 {
		repo = repo[:i]
	}
	repo = strings.TrimSuffix(strings.TrimSuffix(strings.Trim(repo, "/"), ".git"), "/")
	parts := strings.Split(repo, "/")
	if !remoteHost.MatchString(host) || len(parts) < 2 ||
		slices.ContainsFunc(parts, func(p string) bool { return !remotePart.MatchString(p) }) {
		return "", "", false
	}
	return forge.Canonical(host), repo, true
}

var registryRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// terragruntSourcePin reads terraform { source } in a Terragrunt file: tfr://[host]/<module>?version=
// is a tfmodule pin, and a git source with ?ref= is a pin of the repository.
func terragruntSourcePin(attr *hcl.Attribute) (Pin, bool) {
	src, ok := staticString(attr.Expr)
	if !ok {
		return Pin{}, false
	}
	line := attr.Expr.Range().Start.Line
	rest, isTFR := strings.CutPrefix(src, "tfr://")
	if !isTFR {
		return gitSourcePin(src, line)
	}
	addr, query, _ := strings.Cut(rest, "?")
	q, err := url.ParseQuery(query)
	if err != nil || q.Get(keyVersion) == "" {
		return Pin{}, false
	}
	host, module, _ := strings.Cut(addr, "/")
	if host != "" {
		module = host + "/" + module
	}
	name, ok := registryModule(module)
	if !ok {
		return Pin{}, false
	}
	return tfModulePin(withoutParam(src, keyVersion), name, q.Get(keyVersion), line), true
}

// withoutParam drops a query parameter from a module source, so the raw name of a pin stays the
// same when only its version changes, and digest --base pairs the two sides.
func withoutParam(src, key string) string {
	addr, query, ok := strings.Cut(src, "?")
	if !ok {
		return src
	}
	var kept []string
	for kv := range strings.SplitSeq(query, "&") {
		if k, _, _ := strings.Cut(kv, "="); k != key && kv != "" {
			kept = append(kept, kv)
		}
	}
	if len(kept) == 0 {
		return addr
	}
	return addr + "?" + strings.Join(kept, "&")
}

func providerPin(attr *hcl.Attribute) (Pin, bool) {
	source := "hashicorp/" + attr.Name
	if v, ok := staticString(attr.Expr); ok { // the pre-0.13 form: aws = "~> 6.0"
		return tfProviderPin(source, v, attr.Expr.Range().Start.Line), true
	}
	items, diags := hcl.ExprMap(attr.Expr)
	if diags.HasErrors() {
		return Pin{}, false
	}
	version, line := "", 0
	for _, kv := range items {
		key := hcl.ExprAsKeyword(kv.Key)
		if key == "" {
			key, _ = staticString(kv.Key)
		}
		switch key {
		case keySource:
			if s, ok := staticString(kv.Value); ok {
				source = s
			}
		case keyVersion:
			version, _ = staticString(kv.Value)
			line = kv.Value.Range().Start.Line
		default:
		}
	}
	if version == "" {
		return Pin{}, false
	}
	return tfProviderPin(source, version, line), true
}

func tfProviderPin(source, version string, line int) Pin {
	return Pin{
		Tool: "tfprovider:" + providerName(source), RawName: source, Kind: KindTFProvider, Value: version,
		Location: Location{Line: line},
	}
}

// providerName drops the public registry hosts, so registry.terraform.io/hashicorp/aws and
// hashicorp/aws name the same provider. Other hosts stay.
func providerName(source string) string {
	for _, host := range []string{"registry.terraform.io/", "registry.opentofu.org/"} {
		if rest, ok := strings.CutPrefix(strings.ToLower(source), host); ok {
			return rest
		}
	}
	return strings.ToLower(source)
}

// detectTFLock reads the exact provider versions in .terraform.lock.hcl.
func detectTFLock(f file) ([]Pin, error) {
	hf, err := parseHCL(f)
	if err != nil {
		return nil, err
	}
	top, _, _ := hf.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "provider", LabelNames: []string{"address"}}},
	})
	var pins []Pin
	for _, block := range top.Blocks {
		content, _, _ := block.Body.PartialContent(
			&hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: keyVersion}}},
		)
		attr, ok := content.Attributes["version"]
		if !ok {
			continue
		}
		if v, isString := staticString(attr.Expr); isString {
			pins = append(pins, tfProviderPin(block.Labels[0], v, attr.Expr.Range().Start.Line))
		}
	}
	return pins, nil
}

// terragruntTool returns the tool a terragrunt version constraint attribute pins.
func terragruntTool(attr string) string {
	if attr == "terraform_version_constraint" {
		return toolTerraform
	}
	return "terragrunt"
}

// detectTerragrunt reads the two version constraint attributes at the top level of an .hcl file
// and the source of its terraform block. Only literal strings count: terragrunt functions and
// locals aren't evaluated.
func detectTerragrunt(f file) ([]Pin, error) {
	constraint := bytes.Contains(f.data, []byte("_version_constraint"))
	if !constraint && !bytes.Contains(f.data, []byte(keySource)) {
		return nil, nil
	}
	hf, err := parseHCL(f)
	if err != nil {
		return nil, parseProblem(err, constraint)
	}
	content, _, _ := hf.Body.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "terraform_version_constraint"}, {Name: "terragrunt_version_constraint"},
		},
		Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}},
	})
	var pins []Pin
	for _, block := range content.Blocks {
		tf, _, _ := block.Body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: keySource}}})
		if attr, ok := tf.Attributes[keySource]; ok {
			if p, isPin := terragruntSourcePin(attr); isPin {
				pins = append(pins, p)
			}
		}
	}
	for name, attr := range content.Attributes {
		if v, ok := staticString(attr.Expr); ok {
			pins = append(pins, Pin{
				RawName: name, Kind: KindTool, Value: v,
				Location: Location{Line: attr.Expr.Range().Start.Line}, lookup: terragruntTool(name),
			})
		}
	}
	return pins, nil
}
