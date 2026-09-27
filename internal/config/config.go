// Package config loads Appwrite connection settings from the process
// environment (and an optional local .env file). It never reads or writes
// credentials anywhere else, and never logs their values.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/errs"
)

// Environment holds the connection details for a single Appwrite endpoint
// (either a standalone target, or one side of a source/destination pair).
type Environment struct {
	Label     string // human-readable label, e.g. "source", "destination", "target"
	Endpoint  string // e.g. https://cloud.appwrite.io/v1 or https://appwrite.example.com/v1
	ProjectID string
	APIKey    string
}

// Validate returns a *errs.Error with Kind=KindConfiguration if required
// fields are missing. It never includes the API key value in the error.
func (e Environment) Validate() error {
	var missing []string
	if e.Endpoint == "" {
		missing = append(missing, envName(e.Label, "ENDPOINT"))
	}
	if e.ProjectID == "" {
		missing = append(missing, envName(e.Label, "PROJECT_ID"))
	}
	if e.APIKey == "" {
		missing = append(missing, envName(e.Label, "API_KEY"))
	}
	if len(missing) > 0 {
		return errs.New(errs.KindConfiguration, "config.Validate",
			fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", ")))
	}
	return nil
}

func envName(label, suffix string) string {
	switch label {
	case "source":
		return "AMG_SOURCE_" + suffix
	case "destination":
		return "AMG_DEST_" + suffix
	default:
		return "APPWRITE_" + suffix
	}
}

// LoadDotEnv reads a .env-style file at path, if it exists, and applies any
// KEY=VALUE lines to the process environment for keys that are not already
// set. It is intentionally minimal: no interpolation, no multi-line values.
// Missing files are not an error.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errs.New(errs.KindConfiguration, "config.LoadDotEnv", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			// A leading UTF-8 byte-order-mark rune (U+FEFF) on the
			// file's first line would otherwise silently prefix the
			// first variable's key, so os.LookupEnv never matches the
			// real name and Environment.Validate reports it as missing
			// — a confusing failure since the value is visibly right
			// there in the file. Windows editors (Notepad, and some
			// VS Code configurations) commonly save UTF-8 with a BOM,
			// and this is amg's documented Windows config file.
			line = strings.TrimPrefix(line, string(rune(0xFEFF)))
			firstLine = false
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, value)
	}
	if err := scanner.Err(); err != nil {
		return errs.New(errs.KindConfiguration, "config.LoadDotEnv", err)
	}
	return nil
}

// Target loads a single Appwrite environment from the generic APPWRITE_*
// variables. Used by commands that operate against one environment
// (inventory, snapshot).
func Target() Environment {
	return Environment{
		Label:     "target",
		Endpoint:  strings.TrimRight(os.Getenv("APPWRITE_ENDPOINT"), "/"),
		ProjectID: os.Getenv("APPWRITE_PROJECT_ID"),
		APIKey:    os.Getenv("APPWRITE_API_KEY"),
	}
}

// Source loads the source-side Appwrite environment from AMG_SOURCE_*
// variables. Used by commands that compare two environments (preflight,
// verify).
func Source() Environment {
	return Environment{
		Label:     "source",
		Endpoint:  strings.TrimRight(os.Getenv("AMG_SOURCE_ENDPOINT"), "/"),
		ProjectID: os.Getenv("AMG_SOURCE_PROJECT_ID"),
		APIKey:    os.Getenv("AMG_SOURCE_API_KEY"),
	}
}

// Destination loads the destination-side Appwrite environment from
// AMG_DEST_* variables.
func Destination() Environment {
	return Environment{
		Label:     "destination",
		Endpoint:  strings.TrimRight(os.Getenv("AMG_DEST_ENDPOINT"), "/"),
		ProjectID: os.Getenv("AMG_DEST_PROJECT_ID"),
		APIKey:    os.Getenv("AMG_DEST_API_KEY"),
	}
}
