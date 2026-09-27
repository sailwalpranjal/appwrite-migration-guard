package compare

import (
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func user(id, name string, meta map[string]any) inventory.Resource {
	return inventory.Resource{Type: inventory.ResourceUser, ID: id, Name: name, Metadata: meta}
}

func TestCompare_UserStatusChanged_Blocks(t *testing.T) {
	src := inv(user("u1", "Alice", map[string]any{"enabled": true}))
	dst := inv(user("u1", "Alice", map[string]any{"enabled": false}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleConfigChanged) == nil {
		t.Fatal("expected a config_changed finding for user status")
	}
}

func TestCompare_UserLabelsChanged_Blocks(t *testing.T) {
	src := inv(user("u1", "Alice", map[string]any{"labels": []string{"vip"}}))
	dst := inv(user("u1", "Alice", map[string]any{"labels": []string{}}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
}

// Regression guard: list-valued metadata (labels,
// allowed_file_extensions, ...) must compare as an order-independent
// set, in both its native []string form and its []any form after a
// manifest round-trips through JSON — Appwrite does not guarantee list
// ordering, so a naive string comparison produced spurious BLOCK
// findings for semantically identical users/buckets.
func TestCompare_UserLabelsReordered_Pass(t *testing.T) {
	src := inv(user("u1", "Alice", map[string]any{"labels": []string{"vip", "beta"}}))
	dst := inv(user("u1", "Alice", map[string]any{"labels": []string{"beta", "vip"}}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS for reordered labels, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_UserLabelsReordered_JSONRoundTrip_Pass(t *testing.T) {
	// Simulate what Metadata looks like after Read()ing a manifest:
	// json.Unmarshal decodes a JSON array into []any, not []string.
	src := inv(user("u1", "Alice", map[string]any{"labels": []any{"vip", "beta"}}))
	dst := inv(user("u1", "Alice", map[string]any{"labels": []any{"beta", "vip"}}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS for reordered labels post-JSON-round-trip, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_UserMissing_Blocks(t *testing.T) {
	src := inv(user("u1", "Alice", nil))
	dst := inv()

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleMissingResource) == nil {
		t.Fatal("expected a missing_resource finding")
	}
}

func TestCompare_UserIdentical_Pass(t *testing.T) {
	meta := map[string]any{"enabled": true, "email_verification": true, "phone_verification": false, "mfa": false, "labels": []string{"vip"}}
	src := inv(user("u1", "Alice", meta))
	dst := inv(user("u1", "Alice", meta))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
}

// Regression guard: user metadata keys must never be checked against
// another resource type's keys (e.g. a table's row_security) and vice
// versa — comparedMetadataKeys is type-scoped precisely to prevent this.
func TestCompare_UserMetadataKeysAreTypeScoped(t *testing.T) {
	src := table("t1", "db1", "Widgets", nil, map[string]any{"enabled": true, "row_security": false}, 0)
	dst := table("t1", "db1", "Widgets", nil, map[string]any{"enabled": true, "row_security": false}, 0)
	res := Compare("source", inv(src), "dest", inv(dst))
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}

	srcU := user("u1", "Alice", map[string]any{"enabled": true})
	dstU := user("u1", "Alice", map[string]any{"enabled": true})
	res2 := Compare("source", inv(srcU), "dest", inv(dstU))
	if res2.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res2.Overall(), res2.Findings)
	}
}
