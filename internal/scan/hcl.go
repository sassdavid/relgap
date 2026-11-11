package scan

import (
	"bytes"
	"errors"
	"path"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty/cty"
)

func isTerraform(rel string) bool {
	return strings.HasSuffix(rel, ".tf") || strings.HasSuffix(rel, ".tf.json")
}

// isTerragrunt matches every .hcl file except the provider lock file.
func isTerragrunt(rel string) bool {
	return path.Ext(rel) == ".hcl" && path.Base(rel) != ".terraform.lock.hcl"
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

// staticString evaluates expr without variables or functions.
func staticString(expr hcl.Expression) (string, bool) {
	v, diags := expr.Value(nil)
	if diags.HasErrors() || v.IsNull() || !v.IsKnown() || v.Type() != cty.String {
		return "", false
	}
	return v.AsString(), true
}

// detectTerraform reads terraform { required_version, required_providers } blocks. A provider
// without a source is hashicorp/<name>, as in Terraform.
func detectTerraform(f file) ([]Pin, error) {
	if !bytes.Contains(f.data, []byte("required_")) {
		return nil, nil
	}
	hf, err := parseHCL(f)
	if err != nil {
		return nil, err
	}
	top, _, _ := hf.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}}})
	var pins []Pin
	for _, block := range top.Blocks {
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
	}
	return pins, nil
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
		case "source":
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

// detectTerragrunt reads the two version constraint attributes at the top level of an .hcl file.
// Only literal strings count: terragrunt functions and locals aren't evaluated.
func detectTerragrunt(f file) ([]Pin, error) {
	if !bytes.Contains(f.data, []byte("_version_constraint")) {
		return nil, nil
	}
	hf, err := parseHCL(f)
	if err != nil {
		return nil, err
	}
	content, _, _ := hf.Body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{
		{Name: "terraform_version_constraint"}, {Name: "terragrunt_version_constraint"},
	}})
	var pins []Pin
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
