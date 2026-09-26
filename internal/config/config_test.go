package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
)

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		old, had := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, old)
			}
		})
	}
}

func TestTarget_MissingFields(t *testing.T) {
	clearEnv(t, "APPWRITE_ENDPOINT", "APPWRITE_PROJECT_ID", "APPWRITE_API_KEY")
	env := Target()
	err := env.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errs.IsKind(err, errs.KindConfiguration) {
		t.Fatalf("expected KindConfiguration, got %v", err)
	}
}

func TestTarget_Complete(t *testing.T) {
	clearEnv(t, "APPWRITE_ENDPOINT", "APPWRITE_PROJECT_ID", "APPWRITE_API_KEY")
	os.Setenv("APPWRITE_ENDPOINT", "https://cloud.appwrite.io/v1/")
	os.Setenv("APPWRITE_PROJECT_ID", "proj1")
	os.Setenv("APPWRITE_API_KEY", "key1")

	env := Target()
	if err := env.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Endpoint != "https://cloud.appwrite.io/v1" {
		t.Fatalf("expected trailing slash trimmed, got %q", env.Endpoint)
	}
}

func TestSourceDestination_Independent(t *testing.T) {
	clearEnv(t, "AMG_SOURCE_ENDPOINT", "AMG_SOURCE_PROJECT_ID", "AMG_SOURCE_API_KEY",
		"AMG_DEST_ENDPOINT", "AMG_DEST_PROJECT_ID", "AMG_DEST_API_KEY")
	os.Setenv("AMG_SOURCE_ENDPOINT", "https://src.example.com/v1")
	os.Setenv("AMG_SOURCE_PROJECT_ID", "src-proj")
	os.Setenv("AMG_SOURCE_API_KEY", "src-key")

	src := Source()
	if err := src.Validate(); err != nil {
		t.Fatalf("unexpected error for source: %v", err)
	}

	dst := Destination()
	if err := dst.Validate(); err == nil {
		t.Fatal("expected destination to be incomplete")
	}
}

func TestLoadDotEnv_DoesNotOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("APPWRITE_PROJECT_ID=from-file\nAPPWRITE_ENDPOINT=https://example.com/v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	clearEnv(t, "APPWRITE_PROJECT_ID", "APPWRITE_ENDPOINT")
	os.Setenv("APPWRITE_PROJECT_ID", "from-env")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv("APPWRITE_PROJECT_ID"); got != "from-env" {
		t.Fatalf("expected existing env var preserved, got %q", got)
	}
	if got := os.Getenv("APPWRITE_ENDPOINT"); got != "https://example.com/v1" {
		t.Fatalf("expected value from .env, got %q", got)
	}
}

func TestLoadDotEnv_MissingFileIsNotError(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env")); err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
}
