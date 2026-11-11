// Command manifestcheck validates kentik-api.json, the machine-readable API
// coverage manifest read by the kentikcommunity indexer.
//
// It checks the document shape, the allowed enum values, that every
// (service, version) pair is unique, and that schema_source.ref matches the
// api-schema-public version pinned in go.mod.
//
// Usage:
//
//	go run ./internal/cmd/manifestcheck kentik-api.json
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const schemaModule = "github.com/kentik/api-schema-public"

// Manifest mirrors the kentik-api.json document.
type Manifest struct {
	SchemaSource struct {
		Repo string `json:"repo"`
		Ref  string `json:"ref"`
	} `json:"schema_source"`
	Services []Service `json:"services"`
	Legacy   []struct {
		API    string `json:"api"`
		Status string `json:"status"`
	} `json:"legacy"`
}

// Service is one service version entry.
type Service struct {
	Service   string   `json:"service"`
	Version   string   `json:"version"`
	Stability string   `json:"stability"`
	Status    string   `json:"status"`
	Surface   []string `json:"surface"`
}

var (
	stabilities = map[string]bool{"stable": true, "beta": true, "alpha": true}
	statuses    = map[string]bool{"supported": true, "partial": true, "experimental": true, "deprecated": true}
	versionRE   = regexp.MustCompile(`^v[0-9]+((alpha|beta)[0-9]+)?$`)
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: manifestcheck <kentik-api.json>")
		os.Exit(2)
	}
	if err := check(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "kentik-api.json invalid:", err)
		os.Exit(1)
	}
	fmt.Println("kentik-api.json OK")
}

func check(path string) error {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	var errs []error
	if m.SchemaSource.Repo != schemaModule {
		errs = append(errs, fmt.Errorf("schema_source.repo = %q, want %q", m.SchemaSource.Repo, schemaModule))
	}
	if want, err := pinnedVersion(filepath.Join(filepath.Dir(path), "go.mod")); err != nil {
		errs = append(errs, err)
	} else if m.SchemaSource.Ref != want {
		errs = append(errs, fmt.Errorf("schema_source.ref = %q, go.mod pins %q", m.SchemaSource.Ref, want))
	}
	if len(m.Services) == 0 {
		errs = append(errs, errors.New("services is empty"))
	}
	seen := map[string]bool{}
	for i, s := range m.Services {
		key := s.Service + "@" + s.Version
		switch {
		case s.Service == "":
			errs = append(errs, fmt.Errorf("services[%d]: empty service", i))
		case !versionRE.MatchString(s.Version):
			errs = append(errs, fmt.Errorf("%s: bad version", key))
		case !stabilities[s.Stability]:
			errs = append(errs, fmt.Errorf("%s: bad stability %q", key, s.Stability))
		case !statuses[s.Status]:
			errs = append(errs, fmt.Errorf("%s: bad status %q", key, s.Status))
		case len(s.Surface) == 0:
			errs = append(errs, fmt.Errorf("%s: empty surface", key))
		case seen[key]:
			errs = append(errs, fmt.Errorf("%s: duplicate entry", key))
		}
		seen[key] = true
		if s.Stability != "stable" && s.Status == "supported" {
			errs = append(errs, fmt.Errorf("%s: %s versions must be experimental or deprecated", key, s.Stability))
		}
	}
	for i, l := range m.Legacy {
		if l.API == "" || !statuses[l.Status] {
			errs = append(errs, fmt.Errorf("legacy[%d]: bad entry", i))
		}
	}
	return errors.Join(errs...)
}

func pinnedVersion(goMod string) (string, error) {
	raw, err := os.ReadFile(filepath.Clean(goMod))
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(raw)) {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == schemaModule {
			return f[1], nil
		}
		if len(f) >= 3 && f[0] == "require" && f[1] == schemaModule {
			return f[2], nil
		}
	}
	return "", fmt.Errorf("%s not required in %s", schemaModule, goMod)
}
