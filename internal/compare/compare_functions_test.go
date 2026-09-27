package compare

import (
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func function(id, name string, execute []string, meta map[string]any) inventory.Resource {
	return inventory.Resource{Type: inventory.ResourceFunction, ID: id, Name: name, Permissions: execute, Metadata: meta}
}

func TestCompare_FunctionScheduleChanged_Blocks(t *testing.T) {
	src := inv(function("fn1", "Cleanup", nil, map[string]any{"schedule": "0 0 * * *"}))
	dst := inv(function("fn1", "Cleanup", nil, map[string]any{"schedule": "0 12 * * *"}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleConfigChanged) == nil {
		t.Fatal("expected a config_changed finding for schedule")
	}
}

func TestCompare_FunctionExecutePermissionsChanged_Blocks(t *testing.T) {
	src := inv(function("fn1", "SendEmail", []string{"users"}, nil))
	dst := inv(function("fn1", "SendEmail", []string{"any"}, nil))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RulePermissionChanged) == nil {
		t.Fatal("expected a permission_changed finding for execute permissions")
	}
}

func TestCompare_FunctionMissing_Blocks(t *testing.T) {
	src := inv(function("fn1", "SendEmail", nil, nil))
	dst := inv()

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleMissingResource) == nil {
		t.Fatal("expected a missing_resource finding")
	}
}

func TestCompare_FunctionIdentical_Pass(t *testing.T) {
	meta := map[string]any{"enabled": true, "logging": true, "runtime": "node-18.0", "schedule": "", "timeout": 15, "entrypoint": "main.js"}
	src := inv(function("fn1", "SendEmail", []string{"users"}, meta))
	dst := inv(function("fn1", "SendEmail", []string{"users"}, meta))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
}

// Regression guard: found via self-review. deployment_retention was
// collected into Metadata but not included in comparedMetadataKeys, so
// a real config difference there would have silently passed.
func TestCompare_FunctionDeploymentRetentionChanged_Blocks(t *testing.T) {
	src := inv(function("fn1", "Cleanup", nil, map[string]any{"deployment_retention": 25}))
	dst := inv(function("fn1", "Cleanup", nil, map[string]any{"deployment_retention": 1}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_FunctionVersionChanged_Blocks(t *testing.T) {
	src := inv(function("fn1", "Cleanup", nil, map[string]any{"version": "v2"}))
	dst := inv(function("fn1", "Cleanup", nil, map[string]any{"version": "v3"}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_FunctionEventsReordered_Pass(t *testing.T) {
	src := inv(function("fn1", "SendEmail", nil, map[string]any{"events": []string{"account.create", "users.update"}}))
	dst := inv(function("fn1", "SendEmail", nil, map[string]any{"events": []string{"users.update", "account.create"}}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS for reordered events, got %s (%+v)", res.Overall(), res.Findings)
	}
}
