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
