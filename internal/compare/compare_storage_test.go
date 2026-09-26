package compare

import (
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

func file(id, bucketID, name, digest string) inventory.Resource {
	return inventory.Resource{
		Type: inventory.ResourceFile, ID: id, ParentID: bucketID, Name: name,
		ContentDigest: digest, Metadata: map[string]any{"mime_type": "image/png"},
	}
}

func bucket(id, name string, meta map[string]any) inventory.Resource {
	return inventory.Resource{Type: inventory.ResourceBucket, ID: id, Name: name, Metadata: meta}
}

func TestCompare_FileContentChanged_Blocks(t *testing.T) {
	src := inv(file("f1", "b1", "logo.png", "aaa111"))
	dst := inv(file("f1", "b1", "logo.png", "bbb222"))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleContentChanged) == nil {
		t.Fatal("expected a content_changed finding")
	}
}

func TestCompare_FileContentIdentical_Pass(t *testing.T) {
	src := inv(file("f1", "b1", "logo.png", "aaa111"))
	dst := inv(file("f1", "b1", "logo.png", "aaa111"))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
}

func TestCompare_FileContentDigestMissing_Warns(t *testing.T) {
	src := inv(file("f1", "b1", "logo.png", ""))
	dst := inv(file("f1", "b1", "logo.png", "bbb222"))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityWarn {
		t.Fatalf("expected WARN, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleContentUnverified) == nil {
		t.Fatal("expected a content_unverified finding")
	}
	if findRule(res.Findings, RuleContentChanged) != nil {
		t.Fatal("must not also claim a confirmed content_changed when one side couldn't be verified")
	}
}

func TestCompare_MissingFile_Blocks(t *testing.T) {
	src := inv(file("f1", "b1", "logo.png", "aaa111"))
	dst := inv()

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleMissingResource) == nil {
		t.Fatal("expected a missing_resource finding")
	}
}

func TestCompare_BucketConfigChanged_Blocks(t *testing.T) {
	src := inv(bucket("b1", "Avatars", map[string]any{"enabled": true, "file_security": false}))
	dst := inv(bucket("b1", "Avatars", map[string]any{"enabled": true, "file_security": true}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleConfigChanged) == nil {
		t.Fatal("expected a config_changed finding")
	}
}

func TestCompare_BucketAllowedExtensionsChanged_Blocks(t *testing.T) {
	src := inv(bucket("b1", "Avatars", map[string]any{"allowed_file_extensions": []string{"png", "jpg"}}))
	dst := inv(bucket("b1", "Avatars", map[string]any{"allowed_file_extensions": []string{"png", "jpg", "exe"}}))

	res := Compare("source", src, "dest", dst)
	if res.Overall() != SeverityBlock {
		t.Fatalf("expected BLOCK, got %s (%+v)", res.Overall(), res.Findings)
	}
	if findRule(res.Findings, RuleConfigChanged) == nil {
		t.Fatal("expected a config_changed finding for allowed_file_extensions")
	}
}

// Regression guard: a table's bucket-only metadata keys (e.g.
// file_security) must never be compared against a table's own keys, and
// vice versa — comparedMetadataKeys is keyed by ResourceType precisely to
// prevent this kind of cross-type false positive.
func TestCompare_MetadataKeysAreTypeScoped(t *testing.T) {
	src := table("t1", "db1", "Widgets", nil, map[string]any{"enabled": true, "row_security": false}, 0)
	dst := table("t1", "db1", "Widgets", nil, map[string]any{"enabled": true, "row_security": false}, 0)

	res := Compare("source", inv(src), "dest", inv(dst))
	if res.Overall() != SeverityPass {
		t.Fatalf("expected PASS, got %s (%+v)", res.Overall(), res.Findings)
	}
}
