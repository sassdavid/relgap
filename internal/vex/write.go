package vex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// idPrefix starts the @id of a document relgap writes. vexctl and go-vex name documents the same
// way when the author gives no @id.
const idPrefix = "https://openvex.dev/docs/public/vex-"

// Write writes d as indented JSON with its @context, version 1 when it has none, and an @id
// that hashes its content without the timestamp, as govulncheck does: the same statements give
// the same @id.
func Write(w io.Writer, d Document) error {
	d.Context = Context
	if d.Version == 0 {
		d.Version = 1
	}
	if d.Statements == nil {
		d.Statements = []Statement{}
	}
	plain := d
	plain.ID, plain.Timestamp = "", time.Time{}
	b, err := json.Marshal(plain)
	if err != nil {
		return fmt.Errorf("hash vex document: %w", err)
	}
	sum := sha256.Sum256(b)
	d.ID = idPrefix + hex.EncodeToString(sum[:])
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err = enc.Encode(d); err != nil {
		return fmt.Errorf("write vex document: %w", err)
	}
	return nil
}
