package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/compare"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func TestWriteHTML_EscapesResourceNamesAndMessages(t *testing.T) {
	// A resource whose name/message contains something that would be
	// dangerous if rendered unescaped — this is what an attacker-named
	// Appwrite resource (or just an unlucky project) could look like.
	res := &compare.Result{
		SchemaVersion: compare.ResultSchemaVersion,
		SourceLabel:   `<script>alert("src")</script>`,
		DestLabel:     "destination",
		Findings: []compare.Finding{
			{
				Severity: compare.SeverityBlock, Rule: compare.RuleMissingResource,
				ResourceType: inventory.ResourceTable, ResourceID: `"><img src=x onerror=alert(1)>`,
				Message: `table "<script>alert('xss')</script>" is missing`,
			},
		},
	}

	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	for _, dangerous := range []string{
		"<script>alert(\"src\")</script>",
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert(1)>",
	} {
		if strings.Contains(out, dangerous) {
			t.Fatalf("unescaped dangerous content found in HTML output: %q\nfull output:\n%s", dangerous, out)
		}
	}
	// The escaped forms should be present instead.
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("expected escaped <script> tag in output:\n%s", out)
	}
}

func TestWriteHTML_NeverReferencesExternalResources(t *testing.T) {
	res := &compare.Result{SourceLabel: "a", DestLabel: "b"}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	for _, forbidden := range []string{"http://", "https://", "<script src", `<link rel="stylesheet" href`} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("output references an external resource (%q), which breaks offline viewing:\n%s", forbidden, out)
		}
	}
}

func TestWriteHTML_SeverityAlwaysShownAsText(t *testing.T) {
	res := &compare.Result{
		SourceLabel: "a", DestLabel: "b",
		Findings: []compare.Finding{
			{Severity: compare.SeverityBlock, Rule: "x", ResourceType: inventory.ResourceTable, ResourceID: "t1", Message: "m"},
			{Severity: compare.SeverityWarn, Rule: "y", ResourceType: inventory.ResourceTable, ResourceID: "t2", Message: "m"},
		},
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, ">BLOCK<") || !strings.Contains(out, ">WARN<") {
		t.Fatalf("expected literal PASS/WARN/BLOCK text in output (not color-only), got:\n%s", out)
	}
}

func TestWriteHTML_EmptyFindings_ShowsNoDifferences(t *testing.T) {
	res := &compare.Result{SourceLabel: "a", DestLabel: "b"}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "No differences found") {
		t.Fatalf("expected an explicit empty-state message, got:\n%s", out)
	}
	if !strings.Contains(out, "badge-PASS") {
		t.Fatalf("expected a PASS badge for zero findings, got:\n%s", out)
	}
}

func TestWriteHTML_ValidDoctypeAndTitle(t *testing.T) {
	res := &compare.Result{SourceLabel: "prod", DestLabel: "staging"}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "<!doctype html>") {
		t.Fatalf("expected output to start with <!doctype html>, got:\n%s", out[:min(200, len(out))])
	}
	if !strings.Contains(out, "<title>amg report: prod vs staging</title>") {
		t.Fatalf("expected a meaningful <title>, got:\n%s", out)
	}
	if !strings.Contains(out, `lang="en"`) {
		t.Fatal("expected <html lang=\"en\"> for accessibility")
	}
}

// TestWriteHTML_FilterToolbarPresentWithFindings is a regression guard
// for the client-side filter/search toolbar added after a design review
// flagged the report as having no way to navigate a large findings list.
// Each row must carry the data attributes the vanilla-JS filter reads.
func TestWriteHTML_FilterToolbarPresentWithFindings(t *testing.T) {
	res := &compare.Result{
		SourceLabel: "a", DestLabel: "b",
		Findings: []compare.Finding{
			{Severity: compare.SeverityBlock, Rule: "missing_resource", ResourceType: inventory.ResourceTable, ResourceID: "t1", Message: "gone"},
		},
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`data-severity="PASS"`, `data-severity="WARN"`, `data-severity="BLOCK"`,
		`id="search"`, `id="result-count"`,
		`data-severity="BLOCK" data-search="block missing_resource table t1  gone"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected filter toolbar output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestWriteHTML_FilterToolbarAbsentWithoutFindings ensures the toolbar
// (and its JS, which assumes #findings-table exists) is only emitted
// when there's something to filter — matching the existing "No
// differences found" empty state.
func TestWriteHTML_FilterToolbarAbsentWithoutFindings(t *testing.T) {
	res := &compare.Result{SourceLabel: "a", DestLabel: "b"}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, `id="toolbar"`) {
		t.Fatalf("expected no filter toolbar when there are no findings, got:\n%s", out)
	}
}

// TestWriteHTML_ResourceBreakdown_CountsBySeverityAndType is a
// regression guard for the per-resource-type findings breakdown table,
// which must count WARN/BLOCK per ResourceType and never emit a row for
// PASS (Findings never carries SeverityPass — see resourceTypeCount's
// doc comment).
func TestWriteHTML_ResourceBreakdown_CountsBySeverityAndType(t *testing.T) {
	res := &compare.Result{
		SourceLabel: "a", DestLabel: "b",
		Findings: []compare.Finding{
			{Severity: compare.SeverityBlock, Rule: "r1", ResourceType: inventory.ResourceTable, ResourceID: "t1", Message: "m"},
			{Severity: compare.SeverityBlock, Rule: "r2", ResourceType: inventory.ResourceTable, ResourceID: "t2", Message: "m"},
			{Severity: compare.SeverityWarn, Rule: "r3", ResourceType: inventory.ResourceBucket, ResourceID: "b1", Message: "m"},
		},
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "<td>table</td><td>0</td><td>2</td>") {
		t.Fatalf("expected table row with 0 warn / 2 block, got:\n%s", out)
	}
	if !strings.Contains(out, "<td>bucket</td><td>1</td><td>0</td>") {
		t.Fatalf("expected bucket row with 1 warn / 0 block, got:\n%s", out)
	}
}

// TestWriteHTML_ResourceBreakdown_AbsentWhenNoFindings ensures the
// breakdown table (and its caption explaining PASS is never a row) only
// renders when there's something to break down.
func TestWriteHTML_ResourceBreakdown_AbsentWhenNoFindings(t *testing.T) {
	res := &compare.Result{SourceLabel: "a", DestLabel: "b"}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, `class="breakdown"`) {
		t.Fatalf("expected no breakdown table when there are no findings, got:\n%s", out)
	}
}

// TestWriteHTML_ThemeToggleAndResponsiveTable checks the explicit
// light/dark toggle machinery and the table's overflow-x wrapper (a
// design-review finding: an unwrapped table breaks on narrow viewports).
func TestWriteHTML_ThemeToggleAndResponsiveTable(t *testing.T) {
	res := &compare.Result{
		SourceLabel: "a", DestLabel: "b",
		Findings: []compare.Finding{
			{Severity: compare.SeverityBlock, Rule: "r1", ResourceType: inventory.ResourceTable, ResourceID: "t1", Message: "m"},
		},
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`id="theme-toggle"`, "localStorage", "data-theme", `class="table-wrap"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}
