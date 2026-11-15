package summarize

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxResponseBytes caps what relgap reads from a model endpoint.
const maxResponseBytes = 4 << 20

// postJSON sends in as JSON to url and decodes a 2xx answer into out. The error of any other
// status carries the API's error message, never a request header.
func postJSON(ctx context.Context, hc *http.Client, url string, header http.Header, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header = header.Clone()
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", req.URL.Redacted(), err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read %s: %w", req.URL.Redacted(), err)
	}
	if resp.StatusCode/100 != 2 { //nolint:mnd // status class
		return fmt.Errorf("call %s: %s%s", req.URL.Redacted(), resp.Status, apiMessage(b))
	}
	if err = json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode answer from %s: %w", req.URL.Redacted(), err)
	}
	return nil
}

// apiMessage returns ": <message>" from an OpenAI or Anthropic error body, or "".
func apiMessage(b []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &e) != nil || e.Error.Message == "" {
		return ""
	}
	return ": " + strings.TrimSpace(e.Error.Message)
}
