package compare

import (
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func tableWithSamples(id string, samples []inventory.RowSample) inventory.Resource {
	return inventory.Resource{Type: inventory.ResourceTable, ID: id, ParentID: "db1", Name: "Widgets", RowSamples: samples}
}

func TestCompare_RowContentChanged_Blocks(t *testing.T) {
	src := inv(tableWithSamples("t1", []inventory.RowSample{{ID: "r1", Digest: "aaa"}}))
	dst := inv(tableWithSamples("t1", []inventory.RowSample{{ID: "r1", Digest: "bbb"}}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleRowContentChanged) == nil {
		t.Fatal("expected a row_content_changed finding")
	}
}

func TestCompare_RowPermissionChanged_Blocks(t *testing.T) {
	src := inv(tableWithSamples("t1", []inventory.RowSample{{ID: "r1", Digest: "aaa", Permissions: []string{"read(\"any\")"}}}))
	dst := inv(tableWithSamples("t1", []inventory.RowSample{{ID: "r1", Digest: "aaa", Permissions: []string{"read(\"users\")"}}}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleRowPermissionChanged) == nil {
		t.Fatal("expected a row_permission_changed finding")
	}
}

func TestCompare_RowSampleIdentical_Pass(t *testing.T) {
	samples := []inventory.RowSample{{ID: "r1", Digest: "aaa", Permissions: []string{"read(\"any\")"}}}
	src := inv(tableWithSamples("t1", samples))
	dst := inv(tableWithSamples("t1", samples))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_RowSampleMissing_Warns(t *testing.T) {
	src := inv(tableWithSamples("t1", []inventory.RowSample{{ID: "r1", Digest: "aaa"}}))
	dst := inv(tableWithSamples("t1", nil))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityWarn {
		t.Fatalf("expected WARN, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleRowSampleMissing) == nil {
		t.Fatal("expected a row_sample_missing finding")
	}
}

func TestCompare_RowSampleUnexpected_Warns(t *testing.T) {
	src := inv(tableWithSamples("t1", nil))
	dst := inv(tableWithSamples("t1", []inventory.RowSample{{ID: "r1", Digest: "aaa"}}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityWarn {
		t.Fatalf("expected WARN, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleRowSampleUnexpected) == nil {
		t.Fatal("expected a row_sample_unexpected finding")
	}
}

func TestCompare_RowSampleUnverified_Warns(t *testing.T) {
	s := tableWithSamples("t1", nil)
	s.SampleError = "forbidden"
	d := tableWithSamples("t1", []inventory.RowSample{{ID: "r1", Digest: "aaa"}})

	res := Compare("source", inv(s), "dest", inv(d))
	if res.Overall() != SeverityWarn {
		t.Fatalf("expected WARN, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleRowSampleUnverified) == nil {
		t.Fatal("expected a row_sample_unverified finding")
	}
	// Must not also report row_sample_unexpected for the same table once
	// sampling itself is known to have failed on one side.
	if findRule(res.Findings, RuleRowSampleUnexpected) != nil {
		t.Fatal("did not expect a row_sample_unexpected finding alongside row_sample_unverified")
	}
}

func TestCompare_NoSamplesEitherSide_Pass(t *testing.T) {
	src := inv(tableWithSamples("t1", nil))
	dst := inv(tableWithSamples("t1", nil))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS when sampling was never enabled, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_RowSamples_Deterministic(t *testing.T) {
	samples := make([]inventory.RowSample, 20)
	dstSamples := make([]inventory.RowSample, 20)
	for i := range samples {
		id := string(rune('a' + i))
		samples[i] = inventory.RowSample{ID: id, Digest: "same"}
		dstSamples[i] = inventory.RowSample{ID: id, Digest: "different-" + id} // every row changed
	}
	src := inv(tableWithSamples("t1", samples))
	dst := inv(tableWithSamples("t1", dstSamples))

	r1 := Compare("source", src, "dest", dst)
	r2 := Compare("source", src, "dest", dst)
	if len(r1.Findings) != 20 || len(r2.Findings) != 20 {
		t.Fatalf("expected 20 findings each run, got %d and %d", len(r1.Findings), len(r2.Findings))
	}
	for i := range r1.Findings {
		if r1.Findings[i] != r2.Findings[i] {
			t.Fatalf("non-deterministic ordering at index %d: %+v vs %+v", i, r1.Findings[i], r2.Findings[i])
		}
	}
}
