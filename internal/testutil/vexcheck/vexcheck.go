package vexcheck

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// schema is openvex_json_schema.json of openvex/spec 93139bd (CC0, see
// openvex_json_schema_0.2.0.LICENSE), the schema of OpenVEX v0.2.0. Tests use it; release
// binaries don't embed it.
//
//go:embed openvex_json_schema_0.2.0.json
var schema []byte

// SchemaID is the $id of the embedded schema.
const SchemaID = "https://github.com/openvex/spec/openvex_json_schema_0.2.0.json"

// Validate returns an error that lists every way data breaks the OpenVEX v0.2.0 schema, or nil.
// Formats such as iri and date-time are checked too.
func Validate(data []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return fmt.Errorf("parse openvex schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err = c.AddResource(SchemaID, doc); err != nil {
		return fmt.Errorf("add openvex schema: %w", err)
	}
	sch, err := c.Compile(SchemaID)
	if err != nil {
		return fmt.Errorf("compile openvex schema: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse openvex document: %w", err)
	}
	return sch.Validate(inst) //nolint:wrapcheck // the validation error is the message
}
