// Command gen generates the service accessors, the service registry and kentik-api.json from the protos
// in github.com/kentik/api-schema-public at the version pinned in go.mod.
//
// It reads proto/kentik/** (plus protovendor/ for google/api imports) from the module cache, so it runs
// offline once `go mod download` has fetched the module. Run it with `make generate`.
//
// Outputs (relative to -root):
//
//	kentikapi/zz_registry.go               every service and method (idempotency, HTTP mapping, paging)
//	kentikapi/zz_accessors.go              Client.<Api><Version>() for every stable version, plus aliases
//	kentikapi/experimental/zz_accessors.go accessors for alpha/beta versions without a stable equivalent
//	kentik-api.json                        coverage manifest
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	root := flag.String("root", ".", "repository root")
	schemaDir := flag.String("schema", "", "api-schema-public checkout (default: module cache dir of the go.mod pin)")
	ref := flag.String("ref", "", "schema ref recorded in outputs (default: go.mod pin)")
	flag.Parse()
	if err := run(context.Background(), *root, *schemaDir, *ref); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, root, schemaDir, ref string) error {
	if schemaDir == "" || ref == "" {
		dir, ver, err := moduleDir(ctx, root)
		if err != nil {
			return err
		}
		if schemaDir == "" {
			schemaDir = dir
		}
		if ref == "" {
			ref = ver
		}
	}
	s, err := LoadSchema(ctx, schemaDir, ref)
	if err != nil {
		return err
	}
	if err := checkNames(s); err != nil {
		return err
	}
	outputs := []struct {
		path   string
		render func() ([]byte, error)
	}{
		{"kentikapi/zz_registry.go", func() ([]byte, error) { return RenderRegistry(s) }},
		{"kentikapi/zz_accessors.go", func() ([]byte, error) { return RenderAccessors(s, "kentikapi") }},
		{"kentikapi/experimental/zz_accessors.go", func() ([]byte, error) { return RenderAccessors(s, "experimental") }},
		{"kentik-api.json", func() ([]byte, error) { return RenderManifest(s) }},
	}
	for _, o := range outputs {
		b, err := o.render()
		if err != nil {
			return fmt.Errorf("%s: %w", o.path, err)
		}
		if err := writeIfChanged(filepath.Join(root, o.path), b); err != nil {
			return err
		}
	}
	return nil
}

// moduleDir resolves the module cache directory and version of api-schema-public as pinned in go.mod.
func moduleDir(ctx context.Context, root string) (dir, version string, err error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}\t{{.Version}}", goModule)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("go list -m %s: %w: %s", goModule, err, stderr.String())
	}
	dir, version, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !ok || dir == "" {
		return "", "", fmt.Errorf("module %s not downloaded; run go mod download", goModule)
	}
	return dir, version, nil
}

// reserved are identifiers on kentikapi.Client that generated accessors must not collide with.
var reserved = map[string]bool{
	"Alerting": true, "Cloud": true, "CustomApplications": true, "CustomDimensions": true, "DeviceLabels": true,
	"Devices": true, "MyKentikPortal": true, "Plans": true, "Query": true, "SavedFilters": true, "Sites": true,
	"Tags": true, "Users": true, "Conn": true, "Close": true,
}

func checkNames(s *Schema) error {
	seen := map[string]string{}
	add := func(name, what string) error {
		if reserved[name] {
			return fmt.Errorf("generated accessor %s (%s) collides with a hand-written Client member", name, what)
		}
		if prev, ok := seen[name]; ok {
			return fmt.Errorf("generated accessor %s used by both %s and %s", name, prev, what)
		}
		seen[name] = what
		return nil
	}
	for _, v := range allVersions(s) {
		if v.Exposure == exposureNone {
			continue
		}
		if err := add(accessorName(v), v.Dir); err != nil {
			return err
		}
		if v.IsAlias {
			if err := add(aliasName(v.API), v.Dir+" alias"); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeIfChanged(path string, b []byte) error {
	if old, err := os.ReadFile(filepath.Clean(path)); err == nil && bytes.Equal(old, b) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
