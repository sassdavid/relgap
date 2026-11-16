package summarize

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// The wire APIs of the bedrock-mantle endpoint, the values of llm.api.
const (
	APIMessages        = "messages"
	APIChatCompletions = "chat-completions"
)

const (
	// mantleSigningName is the SigV4 service name of bedrock-mantle; "bedrock" is rejected.
	mantleSigningName = "bedrock-mantle"
	// mantleAnthropicVersion is the anthropic-version header that bedrock-mantle documents.
	mantleAnthropicVersion = "2023-06-01"
)

// MantleOptions configures [NewMantle].
type MantleOptions struct {
	Model       string
	API         string // APIMessages or APIChatCompletions; empty picks by model ID, see [MantleAPI]
	BaseURL     string // empty means [MantleBaseURL]
	Region      string
	APIKey      string                  // an Amazon Bedrock API key; empty signs with SigV4
	Credentials aws.CredentialsProvider // for SigV4
	Now         func() time.Time        // nil means time.Now
}

// Mantle calls the bedrock-mantle endpoint of Amazon Bedrock with the Anthropic Messages or the
// OpenAI Chat Completions API.
type Mantle struct {
	api string
	p   Provider
}

// MantleAPI returns api, or for an empty api, messages for Anthropic model IDs and
// chat-completions for the rest.
func MantleAPI(api, model string) string {
	if api != "" {
		return api
	}
	if strings.HasPrefix(model, "anthropic.") {
		return APIMessages
	}
	return APIChatCompletions
}

// MantleBaseURL returns the regional base URL that api is served under.
func MantleBaseURL(region, api string) string {
	host := "https://bedrock-mantle." + region + ".api.aws"
	if api == APIMessages {
		return host + "/anthropic"
	}
	return host + "/v1"
}

// NewMantle returns a provider for o. Requests go through httpx with e's transport, signed with
// SigV4 unless o has an API key.
func NewMantle(o MantleOptions, e Env) (*Mantle, error) {
	api := MantleAPI(o.API, o.Model)
	if api != APIMessages && api != APIChatCompletions {
		return nil, fmt.Errorf("unknown llm.api %q; use %s or %s", o.API, APIMessages, APIChatCompletions)
	}
	base := o.BaseURL
	if o.Region == "" && (base == "" || o.APIKey == "") {
		return nil, errors.New("no region for the bedrock-mantle endpoint")
	}
	if base == "" {
		base = MantleBaseURL(o.Region, api)
	}
	if o.APIKey == "" {
		if o.Credentials == nil {
			return nil, errors.New("no AWS credentials to sign bedrock-mantle requests")
		}
		next := e.Transport
		if next == nil {
			next = http.DefaultTransport
		}
		now := o.Now
		if now == nil {
			now = time.Now
		}
		e.Transport = &sigV4Transport{
			creds:  o.Credentials,
			signer: v4.NewSigner(),
			region: o.Region,
			now:    now,
			next:   next,
		}
	}
	hc := newHTTPClient(e)
	if api == APIMessages {
		a := NewAnthropic(hc, base, o.APIKey, o.Model)
		a.version = mantleAnthropicVersion
		return &Mantle{api: api, p: a}, nil
	}
	return &Mantle{api: api, p: NewOpenAI(hc, base, o.APIKey, o.Model)}, nil
}

// API returns the wire API m sends.
func (m *Mantle) API() string { return m.api }

// Complete sends p and returns the text of the answer.
func (m *Mantle) Complete(ctx context.Context, p Prompt) (string, error) {
	out, err := m.p.Complete(ctx, p)
	if err != nil {
		return "", fmt.Errorf("bedrock-mantle %s: %w", m.api, err)
	}
	return out, nil
}

// sigV4Transport signs each request for bedrock-mantle. It sits under the httpx retries, so every
// attempt gets a fresh signature and, when they expire, fresh credentials.
type sigV4Transport struct {
	creds  aws.CredentialsProvider
	signer *v4.Signer
	region string
	now    func() time.Time
	next   http.RoundTripper
}

func (t *sigV4Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		body = b
	}
	creds, err := t.creds.Retrieve(req.Context())
	if err != nil {
		return nil, fmt.Errorf("retrieve AWS credentials: %w", err)
	}
	r := req.Clone(req.Context())
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	sum := sha256.Sum256(body)
	err = t.signer.SignHTTP(req.Context(), creds, r, hex.EncodeToString(sum[:]), mantleSigningName, t.region, t.now())
	if err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}
	return t.next.RoundTrip(r)
}
