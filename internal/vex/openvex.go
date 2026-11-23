package vex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Context is the @context of an OpenVEX v0.2.0 document.
const Context = "https://openvex.dev/ns/v0.2.0"

// OpenVEX statuses.
const (
	StatusNotAffected        = "not_affected"
	StatusAffected           = "affected"
	StatusFixed              = "fixed"
	StatusUnderInvestigation = "under_investigation"
)

// Document is an OpenVEX v0.2.0 document, with the fields relgap reads and writes.
type Document struct {
	Context    string      `json:"@context"`
	ID         string      `json:"@id"`
	Author     string      `json:"author"`
	Timestamp  time.Time   `json:"timestamp"`
	Version    int         `json:"version"`
	Tooling    string      `json:"tooling,omitempty"`
	Statements []Statement `json:"statements"`
}

// Statement says how a vulnerability affects products.
type Statement struct {
	Vulnerability   Vulnerability `json:"vulnerability"`
	Timestamp       *time.Time    `json:"timestamp,omitempty"`
	Products        []Component   `json:"products,omitempty"`
	Status          string        `json:"status"`
	Justification   string        `json:"justification,omitempty"`
	ImpactStatement string        `json:"impact_statement,omitempty"`
	ActionStatement string        `json:"action_statement,omitempty"`
}

// Vulnerability names the vulnerability a statement is about.
type Vulnerability struct {
	ID          string   `json:"@id,omitempty"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// Component is a product or a subcomponent, named by an IRI such as a purl, or by identifiers.
type Component struct {
	ID            string            `json:"@id,omitempty"`
	Identifiers   map[string]string `json:"identifiers,omitempty"`
	Subcomponents []Component       `json:"subcomponents,omitempty"`
}

// File is a document read from a path.
type File struct {
	Path     string // slash-separated
	Document Document
}

// Read reads the OpenVEX v0.2.0 document at path.
func Read(path string) (File, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read vex document: %w", err)
	}
	doc, err := Parse(b)
	if err != nil {
		return File{}, fmt.Errorf("read vex document %s: %w", path, err)
	}
	// Reports show the path slash-separated, as they show every other path.
	return File{Path: filepath.ToSlash(path), Document: doc}, nil
}

// Parse decodes an OpenVEX v0.2.0 document. Another @context, such as that of the older
// v0.0.1 format whose vulnerability is a string, is an error.
func Parse(b []byte) (Document, error) {
	var head struct {
		Context string `json:"@context"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return Document{}, fmt.Errorf("parse openvex: %w", err)
	}
	if strings.TrimSuffix(head.Context, "/") != Context {
		return Document{}, fmt.Errorf("@context is %q, want %s", head.Context, Context)
	}
	var doc Document
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("parse openvex: %w", err)
	}
	for i, s := range doc.Statements {
		if s.Vulnerability.Name == "" {
			return Document{}, fmt.Errorf("statement %d has no vulnerability name", i+1)
		}
		switch s.Status {
		case StatusNotAffected, StatusAffected, StatusFixed, StatusUnderInvestigation:
		default:
			return Document{}, fmt.Errorf("statement %d has status %q, want not_affected, affected, fixed or "+
				"under_investigation", i+1, s.Status)
		}
	}
	return doc, nil
}
