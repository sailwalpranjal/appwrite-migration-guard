// Package compare is amg's offline comparison engine: given two
// inventory.Inventory snapshots, it produces a deterministic list of
// findings classified as PASS/WARN/BLOCK. It never makes network calls
// and never mutates its inputs (spec section 31: comparison must work
// fully offline from local manifests).
package compare

import (
	"fmt"
	"sort"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

// Severity is one of the three verification states used throughout amg.
// There is deliberately no numeric score.
type Severity string

const (
	SeverityPass  Severity = "PASS"
	SeverityWarn  Severity = "WARN"
	SeverityBlock Severity = "BLOCK"
)

// rank orders severities so the worst can be found with a single pass.
func (s Severity) rank() int {
	switch s {
	case SeverityBlock:
		return 2
	case SeverityWarn:
		return 1
	default:
		return 0
	}
}

// Finding is one classified difference (or documented non-difference)
// between a matched pair of resources, or an unmatched resource on either
// side. Rule is a short, stable machine-readable name — see the rule_*
// constants below — so callers (and tests) can branch on it without
// parsing Message.
type Finding struct {
	Severity     Severity               `json:"severity"`
	Rule         string                 `json:"rule"`
	ResourceType inventory.ResourceType `json:"resource_type"`
	ResourceID   string                 `json:"resource_id"`
	ParentID     string                 `json:"parent_id,omitempty"`
	Message      string                 `json:"message"`
}

// Rule names. Keeping these as named constants (rather than inline
// strings scattered through compareMatched) is what makes each rule
// individually testable and documentable — see docs/comparison-model.md.
const (
	RuleMissingResource      = "missing_resource"
	RuleUnexpectedResource   = "unexpected_resource"
	RuleParentChanged        = "parent_changed"
	RuleNameChanged          = "name_changed"
	RulePermissionChanged    = "permission_changed"
	RuleConfigChanged        = "config_changed"
	RuleContentChanged       = "content_changed"
	RuleContentUnverified    = "content_unverified"
	RuleRowCountMismatch     = "row_count_mismatch"
	RuleRowCountUnconfirmed  = "row_count_unconfirmed"
	RuleRowCountUnverified   = "row_count_unverified"
	RuleRowContentChanged    = "row_content_changed"
	RuleRowPermissionChanged = "row_permission_changed"
	RuleRowSampleMissing     = "row_sample_missing"
	RuleRowSampleUnexpected  = "row_sample_unexpected"
	RuleRowSampleUnverified  = "row_sample_unverified"
)

// ResultSchemaVersion is bumped whenever Result's JSON shape changes in a
// way that could break a consumer (e.g. `amg report`) written against an
// older version — spec section 26: "version the report schema so future
// changes are manageable." Callers should treat an unrecognized
// (future) version as unsupported rather than guessing at its shape.
const ResultSchemaVersion = 1

// Result is the full, deterministic output of one Compare call.
type Result struct {
	SchemaVersion int       `json:"schema_version"`
	SourceLabel   string    `json:"source_label"`
	DestLabel     string    `json:"dest_label"`
	Findings      []Finding `json:"findings"`
}

// Overall returns the worst Severity across all findings, or
// SeverityPass if there are none. Unlike cli.Checklist (used for
// operational checks like `doctor`), an empty Findings list here
// genuinely means "compared everything, found no differences" — Compare
// only returns a Result once it has finished, so there is no
// "incomplete run" case to guard against the way there is for Checklist.
func (r *Result) Overall() Severity {
	worst := SeverityPass
	for _, f := range r.Findings {
		if f.Severity.rank() > worst.rank() {
			worst = f.Severity
		}
	}
	return worst
}

func resourceKey(t inventory.ResourceType, id string) string {
	return string(t) + ":" + id
}

func indexByKey(inv *inventory.Inventory) map[string]inventory.Resource {
	idx := make(map[string]inventory.Resource, len(inv.Resources))
	for _, r := range inv.Resources {
		idx[resourceKey(r.Type, r.ID)] = r
	}
	return idx
}

// Compare produces a deterministic diff between source and dest. Resource
// matching is by (Type, ID) only — not position, and not ParentID (a
// resource that moved to a different parent is itself reported via
// RuleParentChanged rather than treated as two unrelated resources).
func Compare(sourceLabel string, source *inventory.Inventory, destLabel string, dest *inventory.Inventory) *Result {
	res := &Result{SchemaVersion: ResultSchemaVersion, SourceLabel: sourceLabel, DestLabel: destLabel}

	srcIdx := indexByKey(source)
	dstIdx := indexByKey(dest)

	for k, s := range srcIdx {
		d, ok := dstIdx[k]
		if !ok {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityBlock, Rule: RuleMissingResource,
				ResourceType: s.Type, ResourceID: s.ID, ParentID: s.ParentID,
				Message: fmt.Sprintf("%s %q exists in %s but is missing in %s", s.Type, s.ID, sourceLabel, destLabel),
			})
			continue
		}
		res.Findings = append(res.Findings, compareMatched(s, d)...)
	}

	for k, d := range dstIdx {
		if _, ok := srcIdx[k]; !ok {
			res.Findings = append(res.Findings, Finding{
				Severity: SeverityWarn, Rule: RuleUnexpectedResource,
				ResourceType: d.Type, ResourceID: d.ID, ParentID: d.ParentID,
				Message: fmt.Sprintf("%s %q exists in %s but was not present in %s", d.Type, d.ID, destLabel, sourceLabel),
			})
		}
	}

	sortFindings(res.Findings)
	return res
}

// compareMatched compares two resources that exist on both sides under
// the same (Type, ID).
//
// $createdAt/$updatedAt are deliberately never compared: Appwrite's
// TablesDB create endpoints (Create.php for databases/tables) accept no
// client-supplied timestamp — these fields are always server-assigned at
// creation time. Two independently-created resources are therefore
// structurally guaranteed to differ here regardless of migration
// correctness, so treating this as a difference would only ever produce
// noise. See docs/migration-semantics.md.
func compareMatched(s, d inventory.Resource) []Finding {
	var findings []Finding
	add := func(sev Severity, rule, msg string) {
		findings = append(findings, Finding{
			Severity: sev, Rule: rule,
			ResourceType: s.Type, ResourceID: s.ID, ParentID: s.ParentID,
			Message: msg,
		})
	}

	if s.ParentID != d.ParentID {
		add(SeverityBlock, RuleParentChanged, fmt.Sprintf("%s %q moved from parent %q to %q", s.Type, s.ID, s.ParentID, d.ParentID))
	}
	if s.Name != d.Name {
		add(SeverityBlock, RuleNameChanged, fmt.Sprintf("%s %q name changed: %q -> %q", s.Type, s.ID, s.Name, d.Name))
	}
	if !equalStringSets(s.Permissions, d.Permissions) {
		add(SeverityBlock, RulePermissionChanged, fmt.Sprintf("%s %q permissions changed: %v -> %v", s.Type, s.ID, s.Permissions, d.Permissions))
	}
	for _, mk := range comparedMetadataKeys[s.Type] {
		sv, sok := s.Metadata[mk]
		dv, dok := d.Metadata[mk]
		if sok != dok || !equalMetadataValue(sv, dv) {
			add(SeverityBlock, RuleConfigChanged, fmt.Sprintf("%s %q metadata %q changed: %v -> %v", s.Type, s.ID, mk, sv, dv))
		}
	}

	switch s.Type {
	case inventory.ResourceTable:
		findings = append(findings, compareRowCounts(s, d)...)
		findings = append(findings, compareRowSamples(s, d)...)
	case inventory.ResourceFile:
		switch {
		case s.ContentDigest == "" || d.ContentDigest == "":
			add(SeverityWarn, RuleContentUnverified, fmt.Sprintf("file %q content signature missing on at least one side (source=%q dest=%q); content could not be verified", s.ID, s.ContentDigest, d.ContentDigest))
		case s.ContentDigest != d.ContentDigest:
			add(SeverityBlock, RuleContentChanged, fmt.Sprintf("file %q content signature changed: %s -> %s", s.ID, s.ContentDigest, d.ContentDigest))
		}
	}

	return findings
}

// comparedMetadataKeys lists, per ResourceType, which inventory.Resource
// Metadata keys are treated as configuration drift (RuleConfigChanged)
// rather than ignored. Fields not listed here (e.g. a table's
// bytes_used, which fluctuates independently of anything a migration
// controls) are deliberately never compared.
var comparedMetadataKeys = map[inventory.ResourceType][]string{
	inventory.ResourceDatabase: {"enabled", "type", "status"},
	inventory.ResourceTable:    {"enabled", "row_security"},
	inventory.ResourceBucket:   {"enabled", "file_security", "maximum_file_size", "allowed_file_extensions", "compression", "encryption", "antivirus"},
	inventory.ResourceFile:     {"mime_type"},
	inventory.ResourceUser:     {"enabled", "email_verification", "phone_verification", "mfa", "labels"},
}

func compareRowCounts(s, d inventory.Resource) []Finding {
	base := Finding{ResourceType: s.Type, ResourceID: s.ID, ParentID: s.ParentID}

	switch {
	case s.CountError != "" || d.CountError != "":
		base.Severity = SeverityWarn
		base.Rule = RuleRowCountUnverified
		base.Message = fmt.Sprintf("table %q row count could not be verified on both sides (source_error=%q dest_error=%q)", s.ID, s.CountError, d.CountError)
		return []Finding{base}

	case s.RowCount == d.RowCount:
		return nil

	case s.RowCountCapped || d.RowCountCapped:
		base.Severity = SeverityWarn
		base.Rule = RuleRowCountUnconfirmed
		base.Message = fmt.Sprintf("table %q row count is capped at %d on at least one side; cannot confirm equality (source=%d dest=%d)", s.ID, inventory.RowCountCap, s.RowCount, d.RowCount)
		return []Finding{base}

	default:
		base.Severity = SeverityBlock
		base.Rule = RuleRowCountMismatch
		base.Message = fmt.Sprintf("table %q row count differs: source=%d dest=%d", s.ID, s.RowCount, d.RowCount)
		return []Finding{base}
	}
}

// compareRowSamples compares the bounded row-content samples attached to
// two matched table resources (see inventory.Resource.RowSamples). This
// is a best-effort check: a row outside the sampled window is invisible
// to it, so a missing/unexpected sampled row ID is WARN, not BLOCK — it
// means "this specific check couldn't confirm the row," not "the row is
// definitely gone." A confirmed digest or permission difference *within*
// the sample is a real, BLOCK-level finding.
func compareRowSamples(s, d inventory.Resource) []Finding {
	table := Finding{ResourceType: s.Type, ResourceID: s.ID, ParentID: s.ParentID}

	if s.SampleError != "" || d.SampleError != "" {
		f := table
		f.Severity = SeverityWarn
		f.Rule = RuleRowSampleUnverified
		f.Message = fmt.Sprintf("table %q row sampling could not be verified on both sides (source_error=%q dest_error=%q)", s.ID, s.SampleError, d.SampleError)
		return []Finding{f}
	}
	if len(s.RowSamples) == 0 && len(d.RowSamples) == 0 {
		return nil
	}

	srcByID := make(map[string]inventory.RowSample, len(s.RowSamples))
	srcIDs := make([]string, 0, len(s.RowSamples))
	for _, rs := range s.RowSamples {
		srcByID[rs.ID] = rs
		srcIDs = append(srcIDs, rs.ID)
	}
	sort.Strings(srcIDs)

	dstByID := make(map[string]inventory.RowSample, len(d.RowSamples))
	dstIDs := make([]string, 0, len(d.RowSamples))
	for _, rs := range d.RowSamples {
		dstByID[rs.ID] = rs
		dstIDs = append(dstIDs, rs.ID)
	}
	sort.Strings(dstIDs)

	var findings []Finding
	for _, id := range srcIDs {
		sr := srcByID[id]
		dr, ok := dstByID[id]
		if !ok {
			f := table
			f.Severity = SeverityWarn
			f.Rule = RuleRowSampleMissing
			f.Message = fmt.Sprintf("table %q: sampled row %q was not found on the destination side (outside the sampled window, or genuinely missing)", s.ID, id)
			findings = append(findings, f)
			continue
		}
		if !equalStringSets(sr.Permissions, dr.Permissions) {
			f := table
			f.Severity = SeverityBlock
			f.Rule = RuleRowPermissionChanged
			f.Message = fmt.Sprintf("table %q: row %q permissions changed: %v -> %v", s.ID, id, sr.Permissions, dr.Permissions)
			findings = append(findings, f)
		}
		if sr.Digest != dr.Digest {
			f := table
			f.Severity = SeverityBlock
			f.Rule = RuleRowContentChanged
			f.Message = fmt.Sprintf("table %q: row %q content changed (digest %s -> %s)", s.ID, id, sr.Digest, dr.Digest)
			findings = append(findings, f)
		}
	}
	for _, id := range dstIDs {
		if _, ok := srcByID[id]; !ok {
			f := table
			f.Severity = SeverityWarn
			f.Rule = RuleRowSampleUnexpected
			f.Message = fmt.Sprintf("table %q: row %q found on the destination side but was not in the source sample", s.ID, id)
			findings = append(findings, f)
		}
	}

	return findings
}

// equalMetadataValue compares two Resource.Metadata values for a
// config_changed check. List-valued fields (a table's allowed file
// extensions, a user's labels, ...) are compared as order-independent
// sets — Appwrite does not guarantee list ordering, and a manifest
// round-tripped through JSON turns a Go []string into []any, so both
// representations are normalized before comparing. Everything else falls
// back to a plain formatted-string comparison.
func equalMetadataValue(a, b any) bool {
	as, aok := toStringSlice(a)
	bs, bok := toStringSlice(b)
	if aok && bok {
		return equalStringSets(as, bs)
	}
	if aok != bok {
		// One side decoded as a list and the other didn't (e.g. one is
		// nil/absent) — treat as equal only if both are empty.
		return len(as)+len(bs) == 0 && fmt.Sprint(a) == fmt.Sprint(b)
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// toStringSlice extracts a []string from a Metadata value that is either
// a native []string (a freshly-collected Resource) or []any of strings
// (the same value after a JSON manifest round trip). ok is false for any
// other shape, including nil/absent.
func toStringSlice(v any) ([]string, bool) {
	switch x := v.(type) {
	case []string:
		return x, true
	case []any:
		out := make([]string, len(x))
		for i, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out[i] = s
		}
		return out, true
	default:
		return nil, false
	}
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac, bc := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

// sortFindings orders findings deterministically so two runs over
// unchanged inputs produce byte-identical JSON output.
func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.ResourceType != b.ResourceType {
			return a.ResourceType < b.ResourceType
		}
		if a.ParentID != b.ParentID {
			return a.ParentID < b.ParentID
		}
		if a.ResourceID != b.ResourceID {
			return a.ResourceID < b.ResourceID
		}
		return a.Rule < b.Rule
	})
}
