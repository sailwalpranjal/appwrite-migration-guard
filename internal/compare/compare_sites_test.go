package compare

import (
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func site(id, name string, meta map[string]any) inventory.Resource {
	return inventory.Resource{Type: inventory.ResourceSite, ID: id, Name: name, Metadata: meta}
}

func TestCompare_SiteFrameworkChanged_Blocks(t *testing.T) {
	src := inv(site("site1", "Marketing", map[string]any{"framework": "nextjs"}))
	dst := inv(site("site1", "Marketing", map[string]any{"framework": "astro"}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleConfigChanged) == nil {
		t.Fatal("expected a config_changed finding for framework")
	}
}

func TestCompare_SiteMissing_Blocks(t *testing.T) {
	src := inv(site("site1", "Marketing", nil))
	dst := inv()

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleMissingResource) == nil {
		t.Fatal("expected a missing_resource finding")
	}
}

func TestCompare_SiteIdentical_Pass(t *testing.T) {
	meta := map[string]any{"enabled": true, "framework": "nextjs", "build_command": "npm run build", "output_directory": "dist"}
	src := inv(site("site1", "Marketing", meta))
	dst := inv(site("site1", "Marketing", meta))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
}

// Regression guard: a site's metadata keys must never be checked
// against another resource type's keys (e.g. a function's runtime) and
// vice versa.
func TestCompare_SiteMetadataKeysAreTypeScoped(t *testing.T) {
	srcFn := function("fn1", "SendEmail", nil, map[string]any{"runtime": "node-18.0"})
	dstFn := function("fn1", "SendEmail", nil, map[string]any{"runtime": "node-18.0"})
	if res := Compare("source", inv(srcFn), "dest", inv(dstFn)); res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}

	srcSite := site("site1", "Marketing", map[string]any{"framework": "nextjs"})
	dstSite := site("site1", "Marketing", map[string]any{"framework": "nextjs"})
	if res := Compare("source", inv(srcSite), "dest", inv(dstSite)); res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
}
