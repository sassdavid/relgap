package mcpserver

import (
	"slices"

	"github.com/sassdavid/relgap/internal/classify"
	"github.com/sassdavid/relgap/internal/mcpkit"
	"github.com/sassdavid/relgap/internal/report"
)

// digestPager splits a one-tool digest by entry. Kinds fill pages in the order
// [classify.Kinds] lists them, so breaking changes and deprecations come first.
func digestPager(d *report.Digest) mcpkit.Pager {
	if len(d.Tools) != 1 {
		return nil
	}
	tool := d.Tools[0]
	return mcpkit.SlicePager(tool.Entries, "entries",
		func(e classify.Entry) int { return slices.Index(classify.Kinds(), e.Kind) },
		func(keep []classify.Entry) any {
			t := tool
			t.Entries = keep
			return &report.Digest{Tools: []report.ToolDigest{t}}
		})
}

// outdatedPager splits an outdated report by pin: failed pins first, then pins behind latest,
// then the rest.
func outdatedPager(o *report.Outdated) mcpkit.Pager {
	return mcpkit.SlicePager(o.Pins, "pins",
		func(p report.OutdatedPin) int {
			switch p.Status {
			case report.StatusError:
				return 0
			case report.StatusOutdated:
				return 1
			case report.StatusCurrent, report.StatusUnknown, report.StatusSkipped:
			}
			return 2 //nolint:mnd // the rest
		},
		func(keep []report.OutdatedPin) any {
			out := *o
			out.Pins = keep
			return &out
		})
}

// driftPager splits a drift report by tool: tools that drift first.
func driftPager(d *report.Drift) mcpkit.Pager {
	return mcpkit.SlicePager(d.Tools, "tools",
		func(t report.DriftTool) int {
			if t.Drift {
				return 0
			}
			return 1
		},
		func(keep []report.DriftTool) any {
			out := *d
			out.Tools = keep
			return &out
		})
}
