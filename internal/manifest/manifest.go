// Package manifest persists an inventory.Inventory to disk (and reads it
// back) as a versioned, deterministic JSON document. Manifests are the
// only input the comparison engine (internal/compare) needs, so
// `amg compare` can run fully offline (spec section 31).
package manifest

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/inventory"
)

// SchemaVersion is bumped whenever the on-disk Manifest shape changes in a
// way that could break an older amg reading a newer manifest, or vice
// versa. Read() does not currently reject unknown versions — it only
// records what it saw — since there is only one version so far.
const SchemaVersion = 1

// Manifest is the on-disk envelope around an Inventory: who/what it was
// captured from, and when, independent of the inventory's own fields.
type Manifest struct {
	SchemaVersion int                  `json:"schema_version"`
	RunID         string               `json:"run_id"`
	Label         string               `json:"label"`
	CapturedAt    time.Time            `json:"captured_at"`
	Inventory     *inventory.Inventory `json:"inventory"`
}

// New wraps inv in a Manifest with a freshly generated RunID.
func New(label string, inv *inventory.Inventory) (*Manifest, error) {
	id, err := newRunID()
	if err != nil {
		return nil, errs.New(errs.KindConfiguration, "manifest.New", err)
	}
	return &Manifest{
		SchemaVersion: SchemaVersion,
		RunID:         id,
		Label:         label,
		CapturedAt:    time.Now().UTC(),
		Inventory:     inv,
	}, nil
}

func newRunID() (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate run ID suffix: %w", err)
	}
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405Z"), hex.EncodeToString(suffix[:])), nil
}

// DefaultRunDir returns .amg/runs/<runID> under baseDir (typically the
// current working directory).
func DefaultRunDir(baseDir, runID string) string {
	return filepath.Join(baseDir, ".amg", "runs", runID)
}

// Write marshals m as indented, deterministic JSON (the Inventory it
// wraps is already sorted — see inventory.Inventory.Sort) and writes it to
// path, creating parent directories as needed. It never includes
// credentials: Manifest and Inventory hold no API key field.
func Write(path string, m *Manifest) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.New(errs.KindConfiguration, "manifest.Write", fmt.Errorf("create directory %q: %w", dir, err))
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return errs.New(errs.KindInvalidResponse, "manifest.Write", fmt.Errorf("encode manifest: %w", err))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return errs.New(errs.KindConfiguration, "manifest.Write", fmt.Errorf("write %q: %w", path, err))
	}
	return nil
}

// Read loads and decodes a Manifest previously written by Write.
func Read(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errs.New(errs.KindNotFound, "manifest.Read", err)
		}
		return nil, errs.New(errs.KindConfiguration, "manifest.Read", err)
	}
	b = trimUTF8BOM(b)
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, errs.New(errs.KindInvalidResponse, "manifest.Read", fmt.Errorf("decode %q: %w", path, err))
	}
	// Fail closed on a manifest from a newer amg: a schema version this
	// build doesn't recognize could carry fields (or reinterpreted
	// meanings of existing fields) it cannot know about, and silently
	// proceeding risks comparing on a wrong assumption rather than
	// refusing outright. Spec principle: prefer a loud failure over a
	// guess that could produce false confidence.
	if m.SchemaVersion > SchemaVersion {
		return nil, errs.New(errs.KindInvalidResponse, "manifest.Read",
			fmt.Errorf("%q has schema_version %d, newer than this build supports (%d) — upgrade amg before reading it", path, m.SchemaVersion, SchemaVersion))
	}
	return &m, nil
}

// trimUTF8BOM strips a leading UTF-8 byte-order mark, if present.
// encoding/json treats a BOM as invalid JSON and refuses to decode it,
// but a BOM-prefixed file is a real thing amg will be handed: on
// Windows, PowerShell's `>` redirection (the natural way to capture
// `amg snapshot`'s or a JSON tool's output, and PowerShell is amg's
// documented Windows shell) writes UTF-8 output with a BOM by default.
// Silently tolerating it here means a manifest someone captured that
// way still reads correctly instead of failing to decode.
func trimUTF8BOM(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
}
