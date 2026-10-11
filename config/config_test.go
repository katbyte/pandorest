package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// services is a repository's service list, as its main hands it to pandorest.
var services = []Service{
	{Name: "emby", Package: "emby", Output: "sdk/emby", Naming: PathNaming, TagSuffix: "Service"},
	{Name: "jellyfin", Package: "jf", Output: "sdk/jf", Naming: OperationIDNaming},
	{Name: "sonarr", Package: "sonarr", Output: "lib/sonarr", Dir: "api-definitions", Naming: PathNaming},
}

func TestSelect(t *testing.T) {
	t.Parallel()

	all, err := Select(services, "")
	if err != nil || len(all) != len(services) {
		t.Errorf("Select(\"\") = %d services, %v", len(all), err)
	}
	one, err := Select(services, " jellyfin ")
	if err != nil || len(one) != 1 || one[0].Package != "jf" {
		t.Errorf("Select(jellyfin) = %+v, %v", one, err)
	}
	if _, err := Select(services, "emby,plex"); err == nil || !strings.Contains(err.Error(), `unknown service "plex" (have emby, jellyfin, sonarr)`) {
		t.Errorf("Select with an unknown service = %v", err)
	}
	if _, ok := Find(nil, "emby"); ok {
		t.Error("Find in no services found one")
	}
}

// A service's document and definitions are named by the document's version,
// and the highest version present is the one resolved: an older document
// stays beside it, a newer one takes over by being added.
func TestResolveTakesTheHighestVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	svc, ok := Find(services, "emby")
	if !ok {
		t.Fatal("no emby service")
	}
	svc = svc.In(root)
	if _, err := svc.Resolve(); err == nil || !strings.Contains(err.Error(), "no document api-defs/emby-openapi-<version>.json") {
		t.Errorf("no document = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "api-defs"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"4.9.0.30", "4.10.0.40", "4.10.0.9"} {
		if err := os.WriteFile(filepath.Join(root, svc.SpecPath(v)), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := svc.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	// the paths stay repository-relative; they are recorded in the definitions
	if resolved.Version != "4.10.0.40" || resolved.Spec != filepath.Join("api-defs", "emby-openapi-4.10.0.40.json") || resolved.Definitions != filepath.Join("api-defs", "emby-4.10.0.40") {
		t.Errorf("Resolve = %+v", resolved)
	}
	if got := resolved.Path(resolved.Spec); got != filepath.Join(root, "api-defs", "emby-openapi-4.10.0.40.json") {
		t.Errorf("Path(Spec) = %q", got)
	}
	versions, err := resolved.Versions()
	if err != nil || strings.Join(versions, " ") != "4.9.0.30 4.10.0.9 4.10.0.40" {
		t.Errorf("Versions = %v, %v (want numeric order, lowest first)", versions, err)
	}
	if svc.Path(svc.Output) != filepath.Join(root, "sdk", "emby") {
		t.Errorf("Path(Output) = %q", svc.Path(svc.Output))
	}
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"4.10", "4.9", 1},
		{"4.9", "4.10", -1},
		{"12.0.0", "12.0.0", 0},
		{"3", "3.1", -1},
		{"4.10.0.40", "4.10.0.9", 1},
		{"beta", "alpha", 1},
	} {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// A service with Outputs is resolved once per vendored version, each to the
// package directory Outputs names, and Outputs and the documents must name
// each other.
func TestOutputs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "api-defs"), 0o750); err != nil {
		t.Fatal(err)
	}
	svc := Service{Name: "emby", Package: "emby", Outputs: map[string]string{"4.10.0.40": "sdk/emby/v1", "4.11.0.5": "sdk/emby/v2"}}.In(root)
	for _, v := range []string{"4.10.0.40", "4.11.0.5"} {
		if err := os.WriteFile(filepath.Join(root, svc.SpecPath(v)), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	all, err := svc.ResolveAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Version != "4.10.0.40" || all[0].Output != "sdk/emby/v1" || all[0].Definitions != filepath.Join("api-defs", "emby-4.10.0.40") || all[1].Version != "4.11.0.5" || all[1].Output != "sdk/emby/v2" {
		t.Errorf("ResolveAll = %+v", all)
	}
	if one, err := svc.Resolve(); err != nil || one.Version != "4.11.0.5" || one.Output != "sdk/emby/v2" {
		t.Errorf("Resolve = %+v, %v, want the highest with its own package", one, err)
	}

	// a service without Outputs resolves to its highest version alone
	if all, err := (Service{Name: "emby", Output: "sdk/emby"}).In(root).ResolveAll(); err != nil || len(all) != 1 || all[0].Version != "4.11.0.5" || all[0].Output != "sdk/emby" {
		t.Errorf("ResolveAll without Outputs = %+v, %v", all, err)
	}

	for name, tc := range map[string]struct {
		svc  Service
		want string
	}{
		"Output beside Outputs":  {Service{Name: "emby", Output: "sdk/emby", Outputs: map[string]string{"4.10.0.40": "a", "4.11.0.5": "b"}}, "emby: Output and Outputs are both set"},
		"a version not vendored": {Service{Name: "emby", Outputs: map[string]string{"4.10.0.40": "a", "4.11.0.5": "b", "4.12.0.1": "c"}}, "emby: Outputs names 4.12.0.1, and no document api-defs/emby-openapi-4.12.0.1.json is vendored"},
		"a document not placed":  {Service{Name: "emby", Outputs: map[string]string{"4.11.0.5": "b"}}, "emby: api-defs/emby-openapi-4.10.0.40.json is vendored and Outputs does not say where its package goes"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := tc.svc.In(root).ResolveAll(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ResolveAll = %v, want containing %q", err, tc.want)
			}
			if _, err := tc.svc.In(root).Resolve(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Resolve = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// A service whose Dir is set keeps its documents and definitions there
// rather than under DefaultDir.
func TestResolveInItsOwnDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	svc, _ := Find(services, "sonarr")
	svc = svc.In(root)
	if _, err := svc.Resolve(); err == nil || !strings.Contains(err.Error(), "no document api-definitions/sonarr-openapi-<version>.json") {
		t.Errorf("no document = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "api-definitions"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, svc.SpecPath("5.0.1")), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := svc.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Spec != filepath.Join("api-definitions", "sonarr-openapi-5.0.1.json") || resolved.Definitions != filepath.Join("api-definitions", "sonarr-5.0.1") {
		t.Errorf("Resolve = %+v", resolved)
	}
}
