package misex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sassdavid/relgap/internal/execx"
)

// ErrNotInstalled is returned when mise isn't on PATH.
var ErrNotInstalled = errors.New("mise is not installed")

// RegistryEntry is one tool in `mise registry --json`, which lists aliases as extra entries too.
type RegistryEntry struct {
	Short    string   `json:"short"`
	Backends []string `json:"backends"`
	Aliases  []string `json:"aliases,omitempty"`
}

// Runner starts a command; [execx.Runner] implements it.
type Runner interface {
	Run(ctx context.Context, c execx.Cmd) (*execx.Result, error)
}

// Client runs mise.
type Client struct {
	r Runner
}

// New returns a client that runs mise through r.
func New(r Runner) *Client { return &Client{r: r} }

const registryTimeout = 30 * time.Second

// Registry returns the output of `mise registry --json`. mise has no JSON stability policy, so
// unknown fields are ignored.
func (c *Client) Registry(ctx context.Context) ([]RegistryEntry, error) {
	res, err := c.r.Run(ctx, execx.Cmd{Name: "mise", Args: []string{"registry", "--json"}, Timeout: registryTimeout})
	if errors.Is(err, execx.ErrNotFound) {
		return nil, ErrNotInstalled
	}
	if err != nil {
		return nil, fmt.Errorf("mise registry: %w", err)
	}
	var entries []RegistryEntry
	if err = json.Unmarshal(res.Stdout, &entries); err != nil {
		return nil, fmt.Errorf("decode mise registry --json: %w", err)
	}
	return entries, nil
}

const whichTimeout = 10 * time.Second

// Which returns the path of the executable a mise shim named bin runs, from `mise which`.
func (c *Client) Which(ctx context.Context, bin string) (string, error) {
	res, err := c.r.Run(ctx, execx.Cmd{Name: "mise", Args: []string{"which", bin}, Timeout: whichTimeout})
	if errors.Is(err, execx.ErrNotFound) {
		return "", ErrNotInstalled
	}
	if err != nil {
		return "", fmt.Errorf("mise which %s: %w", bin, err)
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}
