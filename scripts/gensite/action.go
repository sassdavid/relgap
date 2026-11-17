package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/sassdavid/relgap/internal/github"
	"github.com/sassdavid/relgap/internal/httpx"
)

const (
	actionRepo = "sassdavid/relgap-action"
	// actionPinFile is the example workflow whose uses: line pins relgap-action; Renovate moves it.
	actionPinFile = "docs/ci/relgap-action.yml"
	// actionCopy is relgap-action's action.yml at the pinned commit, with a header line naming it.
	actionCopy = "scripts/gensite/relgap-action.yml"
)

var (
	actionUses   = regexp.MustCompile(`uses:\s*` + regexp.QuoteMeta(actionRepo) + `@([0-9a-f]{40})\s*#\s*(v\S+)`)
	actionHeader = regexp.MustCompile(
		`^# ` + regexp.QuoteMeta(actionRepo) + ` action\.yml at ([0-9a-f]{40}) \((v\S+)\)`,
	)
)

// actionPin is the commit and release that the example workflow pins.
type actionPin struct{ sha, release string }

// readActionPin returns the pin in [actionPinFile]; every uses: line must name the same commit.
func readActionPin(root string) (actionPin, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(actionPinFile)))
	if err != nil {
		return actionPin{}, fmt.Errorf("read %s: %w", actionPinFile, err)
	}
	ms := actionUses.FindAllSubmatch(b, -1)
	if len(ms) == 0 {
		return actionPin{}, fmt.Errorf("%s: no uses: %s@<sha> # <release> line", actionPinFile, actionRepo)
	}
	pin := actionPin{string(ms[0][1]), string(ms[0][2])}
	for _, m := range ms[1:] {
		if string(m[1]) != pin.sha {
			return actionPin{}, fmt.Errorf("%s pins %s at both %s and %s", actionPinFile, actionRepo, pin.sha, m[1])
		}
	}
	return pin, nil
}

// fetchAction copies action.yml at the pinned commit to [actionCopy], through the GitHub
// contents API.
func fetchAction(ctx context.Context, root string) error {
	pin, err := readActionPin(root)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	token, _ := github.Token(ctx, github.TokenOptions{Lookup: os.LookupEnv})
	gh, err := github.New(httpx.New(httpx.Options{Logger: logger, UserAgent: "relgap-gensite"}),
		github.Options{Token: token, Logger: logger})
	if err != nil {
		return fmt.Errorf("github client: %w", err)
	}
	body, err := gh.File(ctx, actionRepo, pin.sha, "action.yml")
	if err != nil {
		return fmt.Errorf("fetch %s action.yml at %s: %w", actionRepo, pin.sha, err)
	}
	header := fmt.Sprintf("# %s action.yml at %s (%s). Written by go run ./scripts/gensite -fetch-action; "+
		"don't edit.\n", actionRepo, pin.sha, pin.release)
	_, err = writeIfChanged(root, actionCopy, append([]byte(header), body...))
	return err
}

// actionMeta is the part of action.yml the table shows, with inputs and outputs in file order.
type actionMeta struct {
	Inputs  yaml.Node `yaml:"inputs"`
	Outputs yaml.Node `yaml:"outputs"`
}

type actionField struct {
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
}

// actionPart writes the inputs and outputs tables of relgap-action from [actionCopy], after
// checking that the copy is of the pinned commit.
func actionPart(root string, files map[string][]byte) error {
	pin, err := readActionPin(root)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(actionCopy)))
	if err != nil {
		return fmt.Errorf("read %s: %w", actionCopy, err)
	}
	first, _, _ := bytes.Cut(b, []byte("\n"))
	m := actionHeader.FindSubmatch(first)
	switch {
	case m == nil:
		return fmt.Errorf("%s: the first line doesn't name the commit; run mise run docs:action",
			actionCopy)
	case string(m[1]) != pin.sha:
		return fmt.Errorf("%s is of %s, but %s pins %s (%s); run mise run docs:action",
			actionCopy, m[1], actionPinFile, pin.sha, pin.release)
	}
	var meta actionMeta
	if err = yaml.Unmarshal(b, &meta); err != nil {
		return fmt.Errorf("decode %s: %w", actionCopy, err)
	}
	inputs := &table{head: []string{"Input", "Default", colDescription}}
	if err = eachField(&meta.Inputs, func(name string, f actionField) {
		inputs.add(code(name), defaultCell(f.Default), prose(f.Description))
	}); err != nil {
		return err
	}
	outputs := &table{head: []string{"Output", colDescription}}
	if err = eachField(&meta.Outputs, func(name string, f actionField) {
		outputs.add(code(name), prose(f.Description))
	}); err != nil {
		return err
	}
	if len(inputs.rows) == 0 || len(outputs.rows) == 0 {
		return fmt.Errorf("%s has no inputs or no outputs", actionCopy)
	}
	files[partsDir+"relgap-action.md"] = part(fmt.Sprintf(
		"From `action.yml` of [%s %s](https://github.com/%s/tree/%s).\n\n### Inputs\n\n%s\n### Outputs\n\n%s",
		actionRepo, pin.release, actionRepo, pin.release, inputs, outputs))
	return nil
}

// defaultCell shows a default as code; one with an expression keeps Vue away from its braces.
func defaultCell(v string) string {
	if strings.Contains(v, "{{") {
		return "<code v-pre>" + v + "</code>"
	}
	return code(v)
}

// eachField calls fn for each key of a YAML mapping, in file order.
func eachField(n *yaml.Node, fn func(string, actionField)) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("action.yml: inputs and outputs must be mappings")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		var f actionField
		if err := n.Content[i+1].Decode(&f); err != nil {
			return fmt.Errorf("action.yml %s: %w", n.Content[i].Value, err)
		}
		fn(n.Content[i].Value, f)
	}
	return nil
}
