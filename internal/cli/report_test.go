package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestChecklist_Overall_EmptyIsBlock(t *testing.T) {
	var c Checklist
	if c.Overall() != StatusBlock {
		t.Fatalf("expected empty checklist to be BLOCK (incomplete run), got %s", c.Overall())
	}
}

func TestChecklist_Overall_WorstWins(t *testing.T) {
	var c Checklist
	c.Pass("a")
	c.Warn("b", "detail")
	if c.Overall() != StatusWarn {
		t.Fatalf("expected WARN, got %s", c.Overall())
	}
	c.Block("c", "detail")
	if c.Overall() != StatusBlock {
		t.Fatalf("expected BLOCK, got %s", c.Overall())
	}
}

func TestChecklist_ExitCode(t *testing.T) {
	cases := []struct {
		status Status
		code   int
	}{{StatusPass, ExitOK}, {StatusWarn, ExitWarn}, {StatusBlock, ExitBlock}}
	for _, tc := range cases {
		var c Checklist
		c.Add(tc.status, "x", "")
		if got := c.ExitCode(); got != tc.code {
			t.Fatalf("status %s: expected exit %d, got %d", tc.status, tc.code, got)
		}
	}
}

func TestChecklist_WriteTerminal_NoColorOnlyLabels(t *testing.T) {
	var c Checklist
	c.Pass("Endpoint reachable")
	c.Block("Missing destination bucket", "bucket 'avatars' not found")

	var buf bytes.Buffer
	c.WriteTerminal(&buf, "Appwrite Migration Guard")
	out := buf.String()

	for _, want := range []string{"PASS", "BLOCK", "Endpoint reachable", "Missing destination bucket", "bucket 'avatars' not found", "Result: BLOCK"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

// TestChecklistResult_WriteTerminal_IgnoresMismatchedStoredOverall is a
// regression guard: ChecklistResult.WriteTerminal must print "Result:"
// derived from Checks, never the (possibly stale or hand-edited)
// deserialized Overall field — see overallOfChecks's doc comment.
func TestChecklistResult_WriteTerminal_IgnoresMismatchedStoredOverall(t *testing.T) {
	res := ChecklistResult{
		Overall: StatusPass, // deliberately wrong
		Checks:  []Check{{Status: StatusBlock, Title: "Endpoint reachable", Detail: "connection refused"}},
	}
	var buf bytes.Buffer
	res.WriteTerminal(&buf, "Appwrite Migration Guard")
	out := buf.String()
	if !strings.Contains(out, "Result: BLOCK") {
		t.Fatalf("expected the derived BLOCK result, not the stored PASS field, got:\n%s", out)
	}
}

func TestOverallOfChecks(t *testing.T) {
	cases := []struct {
		name   string
		checks []Check
		want   Status
	}{
		{"empty is block", nil, StatusBlock},
		{"all pass", []Check{{Status: StatusPass}, {Status: StatusPass}}, StatusPass},
		{"warn wins over pass", []Check{{Status: StatusPass}, {Status: StatusWarn}}, StatusWarn},
		{"block wins over warn and pass", []Check{{Status: StatusPass}, {Status: StatusWarn}, {Status: StatusBlock}}, StatusBlock},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := overallOfChecks(tc.checks); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
