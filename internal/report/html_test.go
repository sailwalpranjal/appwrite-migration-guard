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
