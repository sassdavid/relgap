package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/sassdavid/relgap/internal/httpx"
)

// Errors wrapped by [Client] methods.
var (
	ErrNotAllowed = errors.New("registry not allowed")
	ErrBadRef     = errors.New("not an image reference")
)

// Docker Hub's names: the one in image references, the host of its registry API and the host
// that issues its anonymous tokens.
const (
	DockerHub     = "docker.io"
	DockerHubAPI  = "registry-1.docker.io"
	DockerHubAuth = "auth.docker.io"
)

// Source label keys, newest first.
const (
	LabelSource     = "org.opencontainers.image.source"
	LabelVCSURL     = "org.label-schema.vcs-url"
	maxPages        = 100
	tagsPerPage     = 1000
	manifestAccept  = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
	linuxOS         = "linux"
	defaultArch     = "amd64"
	unknownPlatform = "unknown"
)

// DefaultRegistries returns the registries relgap reads tags from without configuration.
func DefaultRegistries() []string {
	return []string{DockerHub, "ghcr.io", "registry.k8s.io", "quay.io", "public.ecr.aws", "mcr.microsoft.com"}
}

// Ref is an image name split into its registry and repository.
type Ref struct {
	Registry   string // as written, with docker.io for Docker Hub
	Repository string // with library/ for a one-part Docker Hub name
}

// String returns registry/repository.
func (r Ref) String() string { return r.Registry + "/" + r.Repository }

// apiHost returns the host that serves the registry API.
func (r Ref) apiHost() string {
	if r.Registry == DockerHub {
		return DockerHubAPI
	}
	return r.Registry
}

// ParseRef splits an image name without tag or digest, as Docker does: the first part is a
// registry when it has a dot or a colon or is localhost, and Docker Hub otherwise.
func ParseRef(name string) (Ref, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " @") {
		return Ref{}, fmt.Errorf("%w: %q", ErrBadRef, name)
	}
	first, rest, hasSlash := strings.Cut(name, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		host := strings.ToLower(first)
		if host == "index.docker.io" || host == DockerHubAPI {
			host = DockerHub
		}
		if host == DockerHub && !strings.Contains(rest, "/") {
			rest = "library/" + rest
		}
		if rest == "" {
			return Ref{}, fmt.Errorf("%w: %q", ErrBadRef, name)
		}
		return Ref{Registry: host, Repository: rest}, nil
	}
	if !hasSlash {
		return Ref{Registry: DockerHub, Repository: "library/" + name}, nil
	}
	return Ref{Registry: DockerHub, Repository: name}, nil
}

// Options configures [New].
type Options struct {
	Registries []string // registries the client may contact; nil means [DefaultRegistries]
	Scheme     string   // https unless a test sets http
}

// Client lists tags and reads source labels from image registries with anonymous tokens.
type Client struct {
	hc      *http.Client
	allowed []string
	scheme  string

	mu     sync.Mutex
	tokens map[string]string // by API host and scope
}

// New returns a client that sends requests through hc.
func New(hc *http.Client, o Options) *Client {
	allowed := o.Registries
	if allowed == nil {
		allowed = DefaultRegistries()
	}
	scheme := o.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return &Client{hc: hc, allowed: allowed, scheme: scheme, tokens: map[string]string{}}
}

// Check returns an error wrapping [ErrNotAllowed] unless the client may contact r's registry.
func (c *Client) Check(r Ref) error {
	if !slices.ContainsFunc(c.allowed, func(a string) bool { return strings.EqualFold(a, r.Registry) }) {
		return fmt.Errorf("%w: %s; relgap reads %s, and other registries only when images.registries lists them",
			ErrNotAllowed, r.Registry, strings.Join(c.allowed, ", "))
	}
	return nil
}

func (c *Client) base(r Ref) string {
	return c.scheme + "://" + r.apiHost() + "/v2/" + r.Repository
}

// URL returns the tags list URL of r, which says where its versions come from.
func (c *Client) URL(r Ref) string { return c.base(r) + "/tags/list" }

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="?next"?`)

// Tags lists every tag of r, following Link rel="next" on the registry's own host.
func (c *Client) Tags(ctx context.Context, r Ref) ([]string, error) {
	if err := c.Check(r); err != nil {
		return nil, err
	}
	var out []string
	next := fmt.Sprintf("%s?n=%d", c.URL(r), tagsPerPage)
	for i := 0; next != "" && i < maxPages; i++ {
		body, h, err := c.get(ctx, r, next, "application/json")
		if err != nil {
			return nil, fmt.Errorf("list tags of %s: %w", r, err)
		}
		var page struct {
			Tags []string `json:"tags"`
		}
		if err = json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode tags of %s: %w", r, err)
		}
		out = append(out, page.Tags...)
		next = ""
		if m := nextLink.FindStringSubmatch(h.Get("Link")); m != nil {
			if next, err = resolve(c.URL(r), m[1]); err != nil {
				return nil, fmt.Errorf("list tags of %s: %w", r, err)
			}
		}
	}
	return out, nil
}

func resolve(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", base, err)
	}
	u, err := b.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parse link %s: %w", ref, err)
	}
	return u.String(), nil
}

type descriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Annotations map[string]string `json:"annotations"`
	Platform    *struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
	} `json:"platform"`
}

type manifest struct {
	MediaType   string            `json:"mediaType"`
	Annotations map[string]string `json:"annotations"`
	Manifests   []descriptor      `json:"manifests"`
	Config      descriptor        `json:"config"`
}

// Source returns the source repository URL that the image r:tag names in its annotations or
// labels, or "" when it names none. It reads the manifest, then the linux/amd64 manifest of an
// index, then the image config, and stops at the first that has a source.
func (c *Client) Source(ctx context.Context, r Ref, tag string) (string, error) {
	if err := c.Check(r); err != nil {
		return "", err
	}
	m, err := c.manifest(ctx, r, tag)
	if err != nil {
		return "", err
	}
	if s := source(m.Annotations); s != "" {
		return s, nil
	}
	if len(m.Manifests) > 0 {
		d, ok := pick(m.Manifests)
		if !ok {
			return "", nil
		}
		if s := source(d.Annotations); s != "" {
			return s, nil
		}
		if m, err = c.manifest(ctx, r, d.Digest); err != nil {
			return "", err
		}
		if s := source(m.Annotations); s != "" {
			return s, nil
		}
	}
	if m.Config.Digest == "" {
		return "", nil
	}
	body, _, err := c.get(ctx, r, c.base(r)+"/blobs/"+m.Config.Digest, "")
	if err != nil {
		return "", fmt.Errorf("read config of %s:%s: %w", r, tag, err)
	}
	var cfg struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err = json.Unmarshal(body, &cfg); err != nil {
		return "", fmt.Errorf("decode config of %s:%s: %w", r, tag, err)
	}
	return source(cfg.Config.Labels), nil
}

func (c *Client) manifest(ctx context.Context, r Ref, reference string) (manifest, error) {
	body, _, err := c.get(ctx, r, c.base(r)+"/manifests/"+reference, manifestAccept)
	if err != nil {
		return manifest{}, fmt.Errorf("read manifest of %s:%s: %w", r, reference, err)
	}
	var m manifest
	if err = json.Unmarshal(body, &m); err != nil {
		return manifest{}, fmt.Errorf("decode manifest of %s:%s: %w", r, reference, err)
	}
	return m, nil
}

func source(labels map[string]string) string {
	for _, k := range []string{LabelSource, LabelVCSURL} {
		if v := strings.TrimSpace(labels[k]); v != "" {
			return v
		}
	}
	return ""
}

// pick returns the linux/amd64 image of an index, else its first image; attestation manifests
// have the platform unknown/unknown and are skipped.
func pick(ds []descriptor) (descriptor, bool) {
	var first *descriptor
	for i := range ds {
		d := &ds[i]
		if d.Platform != nil && d.Platform.OS == unknownPlatform {
			continue
		}
		if d.Platform != nil && d.Platform.OS == linuxOS && d.Platform.Architecture == defaultArch {
			return *d, true
		}
		if first == nil {
			first = d
		}
	}
	if first == nil {
		return descriptor{}, false
	}
	return *first, true
}

// get fetches rawURL from r's registry. On a 401 with a Bearer challenge it fetches an anonymous
// token from the challenge's token URL and tries once more. Requests carry the anonymous marker, so
// the cache keys them like requests without a token.
func (c *Client) get(ctx context.Context, r Ref, rawURL, accept string) ([]byte, http.Header, error) {
	if !httpx.SameOrigin(c.base(r), rawURL) {
		return nil, nil, fmt.Errorf("refusing to follow %s off %s", rawURL, r.apiHost())
	}
	ctx = httpx.WithAnonymousToken(ctx)
	scope := "repository:" + r.Repository + ":pull"
	h := http.Header{}
	if accept != "" {
		h.Set("Accept", accept)
	}
	if tok := c.token(r.apiHost(), scope); tok != "" {
		h.Set("Authorization", "Bearer "+tok)
	}
	body, rh, err := httpx.Get(ctx, c.hc, rawURL, h)
	se, isStatus := errors.AsType[*httpx.StatusError](err)
	if !isStatus || se.Status != http.StatusUnauthorized || h.Get("Authorization") != "" {
		return body, rh, err //nolint:wrapcheck // httpx names the URL
	}
	tok, err := c.fetchToken(ctx, r, se.Header.Get("WWW-Authenticate"), scope)
	if err != nil {
		return nil, nil, err
	}
	h.Set("Authorization", "Bearer "+tok)
	return httpx.Get(ctx, c.hc, rawURL, h) //nolint:wrapcheck // httpx names the URL
}

func (c *Client) token(host, scope string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokens[host+" "+scope]
}

var challengeParam = regexp.MustCompile(`(\w+)="([^"]*)"`)

// fetchToken asks the token URL of a Bearer challenge for an anonymous pull token. The request
// carries Cache-Control: no-store, so the token never reaches the cache.
func (c *Client) fetchToken(ctx context.Context, r Ref, challenge, scope string) (string, error) {
	kind, params, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(kind, "Bearer") {
		return "", fmt.Errorf("%s: %w (no Bearer challenge)", r, httpx.ErrUnauthorized)
	}
	p := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(params, -1) {
		p[strings.ToLower(m[1])] = m[2]
	}
	raw := p["realm"] // prose:ignore // the challenge parameter's name
	tokenURL, err := url.Parse(raw)
	if err != nil || tokenURL.Scheme != c.scheme || tokenURL.Host == "" {
		return "", fmt.Errorf("%s: %w (token URL %q)", r, httpx.ErrUnauthorized, raw)
	}
	q := tokenURL.Query()
	if s := p["service"]; s != "" {
		q.Set("service", s)
	}
	q.Set("scope", scope)
	if s := p["scope"]; s != "" {
		q.Set("scope", s)
	}
	tokenURL.RawQuery = q.Encode()
	body, _, err := httpx.Get(ctx, c.hc, tokenURL.String(), http.Header{
		"Accept": {"application/json"}, "Cache-Control": {"no-store"},
	})
	if err != nil {
		return "", fmt.Errorf("anonymous token for %s: %w", r, err)
	}
	var t struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err = json.Unmarshal(body, &t); err != nil {
		return "", fmt.Errorf("decode anonymous token for %s: %w", r, err)
	}
	tok := t.Token
	if tok == "" {
		tok = t.AccessToken
	}
	if tok == "" {
		return "", fmt.Errorf("anonymous token for %s: %w (empty token)", r, httpx.ErrUnauthorized)
	}
	c.mu.Lock()
	c.tokens[r.apiHost()+" "+scope] = tok
	c.mu.Unlock()
	return tok, nil
}
