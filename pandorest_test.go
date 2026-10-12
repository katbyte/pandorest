package pandorest_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katbyte/pandorest"
	"github.com/katbyte/pandorest/config"
	"github.com/katbyte/pandorest/generator"
	"github.com/katbyte/pandorest/importer/workarounds"
	"github.com/katbyte/pandorest/openapi"
)

// miniDocument is the first service's document: a paged list that takes a
// searchTerm it does not declare, which a workaround adds, and a lookup.
const miniDocument = `{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1.9"}, "paths": {
  "/Items": {"get": {"operationId": "GetItems", "tags": ["Items"], "parameters": [
      {"name": "startIndex", "in": "query", "schema": {"type": "integer"}},
      {"name": "limit", "in": "query", "schema": {"type": "integer"}}],
    "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/ItemResult"}}}}}}},
  "/Items/{id}": {"get": {"operationId": "GetItem", "tags": ["Items"], "parameters": [{"name": "id", "in": "path", "schema": {"type": "string"}}],
    "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}}}}}},
  "components": {"schemas": {
    "Item": {"type": "object", "properties": {"Id": {"type": "string"}, "Name": {"type": "string"}}},
    "ItemResult": {"type": "object", "properties": {"Items": {"type": "array", "items": {"$ref": "#/components/schemas/Item"}}, "TotalRecordCount": {"type": "integer"}}}}}}`

// otherDocument is the second service's, kept in a directory of its own.
const otherDocument = `{"openapi": "3.0.1", "info": {"title": "Other", "version": "3"}, "paths": {
  "/things/{id}": {"delete": {"operationId": "x1", "tags": ["Things"], "parameters": [{"name": "id", "in": "path", "schema": {"type": "integer"}}],
    "responses": {"204": {"description": "gone"}}}}}}`

// searchTerm declares the parameter Mini's list takes.
type searchTerm struct{}

func (searchTerm) Name() string    { return "mini-search-term" }
func (searchTerm) Service() string { return "mini" }
func (searchTerm) Bug() string {
	return "GET /Items does not declare searchTerm; the server filters by it"
}

func (searchTerm) Apply(spec *openapi.Spec) error {
	return workarounds.AddParameters(spec, []string{"GET /Items"}, workarounds.Param{Name: "searchTerm", In: openapi.InQuery, Type: openapi.TypeString})
}

// repository is what a repository's main hands pandorest: two services, one
// with a workaround, a base client of its own and its documents under the
// default directory, the other with none of those, built on pandorest's own
// client the way the service says it is talked to.
var repository = pandorest.Config{
	Services: []config.Service{
		{
			Name: "mini", Package: "mini", Output: "sdk/mini", Naming: config.OperationIDNaming, ClientImport: "example.org/repo/sdk/client", Auth: "Token",
			Notes:  "Dates are strings.",
			Paging: &config.Paging{Start: "startIndex", Limit: "limit", Items: "Items", Total: "TotalRecordCount"},
		},
		{Name: "other", Package: "other", Output: "lib/other", Dir: "docs", Naming: config.PathNaming, Auth: "APIKey", ClientService: "service"},
	},
	Workarounds: []workarounds.Workaround{searchTerm{}},
}

func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var out, log bytes.Buffer
	err = pandorest.Run(repository, args, &out, &log)

	return out.String(), log.String(), err
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// newRepository is a repository with each service's document vendored and
// nothing imported or generated yet.
func newRepository(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api-defs", "mini-openapi-1.9.json"), miniDocument)
	writeFile(t, filepath.Join(root, "docs", "other-openapi-3.json"), otherDocument)

	return root
}

// listFiles reads a directory's files by name.
func listFiles(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // a directory under test
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}

	return out
}

func TestUsage(t *testing.T) {
	t.Parallel()

	if _, _, err := runCmd(t); err == nil || !strings.Contains(err.Error(), "usage: pandorest") {
		t.Errorf("no command = %v", err)
	}
	if _, _, err := runCmd(t, "frobnicate", "-root", newRepository(t)); err == nil || !strings.Contains(err.Error(), `unknown command "frobnicate"`) {
		t.Errorf("unknown command = %v", err)
	}
	if _, _, err := runCmd(t, "check", "-service", "plex"); err == nil || !strings.Contains(err.Error(), `unknown service "plex" (have mini, other)`) {
		t.Errorf("unknown service = %v", err)
	}
	if _, _, err := runCmd(t, "diff", "-old", "x"); err == nil || !strings.Contains(err.Error(), "pass both -old and -new") {
		t.Errorf("diff with only -old = %v", err)
	}
	if _, stderr, err := runCmd(t, "import", "-h"); err == nil || !strings.Contains(stderr, "(default all: mini, other)") {
		t.Errorf("import -h = %v:\n%s", err, stderr)
	}
	if _, _, err := runCmd(t, "import", "-root", t.TempDir()); err == nil || !strings.Contains(err.Error(), "mini: no document api-defs/mini-openapi-<version>.json") {
		t.Errorf("import with no document = %v", err)
	}
}

// A repository goes from its vendored documents to definitions and packages,
// and the checks a repository runs in CI pass on what that leaves.
func TestImportGenerateCheck(t *testing.T) {
	t.Parallel()

	root := newRepository(t)

	stdout, stderr, err := runCmd(t, "resolve", "-root", root)
	if want := "mini\t1.9\t" + filepath.Join("api-defs", "mini-openapi-1.9.json") + "\t" + filepath.Join("api-defs", "mini-1.9") + "\nother\t3\t" + filepath.Join("docs", "other-openapi-3.json") + "\t" + filepath.Join("docs", "other-3") + "\n"; err != nil || stdout != want {
		t.Errorf("resolve = %v:\n%s%s", err, stdout, stderr)
	}

	stdout, stderr, err = runCmd(t, "import", "-root", root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "mini: 2 operations, 2 models, 0 constants, 1 workarounds -> "+filepath.Join(root, "api-defs", "mini-1.9")) || !strings.Contains(stdout, "other: 1 operations, 0 models, 0 constants, 0 workarounds -> "+filepath.Join(root, "docs", "other-3")) {
		t.Errorf("import reported:\n%s", stdout)
	}
	if stderr != "pandorest: mini: applied workaround mini-search-term\n" {
		t.Errorf("import logged:\n%s", stderr)
	}
	if service := listFiles(t, filepath.Join(root, "api-defs", "mini-1.9"))["Service.json"]; !strings.Contains(service, `"mini-search-term"`) || !strings.Contains(service, `"Auth": "Token"`) {
		t.Errorf("Service.json does not record the workaround and the auth:\n%s", service)
	}

	stdout, _, err = runCmd(t, "generate", "-root", root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "mini: 9 files -> "+filepath.Join(root, "sdk", "mini")) || !strings.Contains(stdout, "other: 5 files -> "+filepath.Join(root, "lib", "other")) {
		t.Errorf("generate reported:\n%s", stdout)
	}
	mini := listFiles(t, filepath.Join(root, "sdk", "mini"))
	// every file is known as generated; all carry the header but doc.go, whose package comment a linter must be able to see
	for name, src := range mini {
		if !generator.Generated([]byte(src)) || strings.HasPrefix(src, generator.Header) == (name == "doc.go") {
			t.Errorf("%s is not marked as a generated file should be:\n%s", name, strings.SplitN(src, "\n", 2)[0])
		}
	}
	// each service is built on its own base client, with its own auth and notes
	if src := mini["items_method_getitems.go"]; !strings.Contains(src, `"example.org/repo/sdk/client"`) || !strings.Contains(src, `out.Append("searchTerm", o.SearchTerm)`) || !strings.Contains(src, "func (c Client) GetItemsComplete(") {
		t.Errorf("mini's list lacks its base client, the workaround's parameter or its pager:\n%s", src)
	}
	if doc := mini["doc.go"]; !strings.Contains(doc, "from the definitions in api-defs/mini-1.9, which were imported from api-defs/mini-openapi-1.9.json.") || !strings.Contains(doc, "// Dates are strings.\n") {
		t.Errorf("mini's doc.go lacks its source or its notes:\n%s", doc)
	}
	other := listFiles(t, filepath.Join(root, "lib", "other"))
	if src := other["client.go"]; !strings.Contains(src, `"`+config.DefaultClient+`"`) || !strings.Contains(src, "c, err := service.New(baseURL, client.APIKey(token), append(opts[:len(opts):len(opts)], client.WithSecrets(token))...)") {
		t.Errorf("other's client.go is not built on pandorest's client, with its auth, the service's way:\n%s", src)
	}
	if src := mini["client.go"]; !strings.Contains(src, "c, err := client.New(baseURL, client.Token(token), append(opts[:len(opts):len(opts)], client.WithSecrets(token))...)") {
		t.Errorf("mini's client.go is not built on its own base client:\n%s", src)
	}
	if _, ok := other["things_method_deletethingsbyid.go"]; !ok {
		t.Errorf("other's operation is not named by its path: %v", other)
	}

	// the two CI checks: the definitions match the documents and every
	// operation has a method, and nothing differs
	stdout, _, err = runCmd(t, "check", "-root", root, "-quiet")
	if err != nil || !strings.Contains(stdout, "mini: 2 operations, every one has a method in "+filepath.Join(root, "sdk", "mini")) || !strings.Contains(stdout, "other: 1 operations, every one has a method in "+filepath.Join(root, "lib", "other")) {
		t.Errorf("check = %v:\n%s", err, stdout)
	}

	// a hand-written file naming one method is counted, and -uncalled names the rest; the generated files and their tests are not hand-written
	if err := os.WriteFile(filepath.Join(root, "tools.go"), []byte("package tools\n\nfunc use(c interface{ GetItems() }) { c.GetItems() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = runCmd(t, "check", "-root", root, "-quiet", "-uncalled")
	if err != nil || !strings.Contains(stdout, "mini: 1 of 4 methods are named by a hand-written file in the repository\n  never named: GetItem, GetItemsComplete, GetItemsEach\n") || !strings.Contains(stdout, "other: 0 of 1 methods are named") {
		t.Errorf("check -uncalled = %v:\n%s", err, stdout)
	}
	// a test naming a method is not a tool wrapping it
	if err := os.WriteFile(filepath.Join(root, "tools_test.go"), []byte("package tools\n\nfunc useInTest(c interface{ GetItemsComplete() }) { c.GetItemsComplete() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = runCmd(t, "check", "-root", root, "-quiet")
	if err != nil || !strings.Contains(stdout, "mini: 1 of 4 methods are named by a hand-written file in the repository\n") {
		t.Errorf("check with a test naming a method = %v:\n%s", err, stdout)
	}
	// -callers limits the count to one directory, so the SDK's own suite does not count as a tool wrapping a method
	if err := os.MkdirAll(filepath.Join(root, "tools"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tools", "wrap.go"), []byte("package tools\n\nfunc wrap(c interface{ GetItem() }) { c.GetItem() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, _, err = runCmd(t, "check", "-root", root, "-quiet", "-callers", "tools")
	if err != nil || !strings.Contains(stdout, "mini: 1 of 4 methods are named by a hand-written file under tools\n") {
		t.Errorf("check -callers = %v:\n%s", err, stdout)
	}
	stdout, stderr, err = runCmd(t, "diff", "-root", root, "-quiet", "-exit-code")
	if err != nil || stdout != "mini: no changes\nother: no changes\n" || stderr != "" {
		t.Errorf("diff = %v:\n%s%s", err, stdout, stderr)
	}

	// importing and generating again changes nothing, byte for byte
	if _, _, err := runCmd(t, "import", "-root", root, "-service", "mini, other"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmd(t, "generate", "-root", root); err != nil {
		t.Fatal(err)
	}
	for name, src := range listFiles(t, filepath.Join(root, "sdk", "mini")) {
		if mini[name] != src {
			t.Errorf("%s differs on a second generation", name)
		}
	}

	// a method that goes missing fails the check, by name
	if err := os.Remove(filepath.Join(root, "sdk", "mini", "items_method_getitem.go")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmd(t, "check", "-root", root, "-service", "mini", "-quiet"); err == nil || !strings.Contains(err.Error(), "GetItem (GET /Items/{id})") {
		t.Errorf("check with a method gone = %v", err)
	}
}

// A service with Outputs gets an SDK for every version it vendors, each a
// package of its own, and -version picks one of them.
func TestEveryVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "api-defs", "multi-openapi-1.9.json"), miniDocument)
	writeFile(t, filepath.Join(root, "api-defs", "multi-openapi-2.0.json"), strings.Replace(miniDocument, `"version": "1.9"`, `"version": "2.0"`, 1))
	cfg := pandorest.Config{Services: []config.Service{{
		Name: "multi", Package: "multi", Naming: config.OperationIDNaming, Auth: "APIKey", ClientService: "service",
		Outputs: map[string]string{"1.9": "sdk/multi/v1", "2.0": "sdk/multi/v2"},
	}}}
	run := func(args ...string) (string, error) {
		var out, log bytes.Buffer
		err := pandorest.Run(cfg, append(args, "-root", root, "-quiet"), &out, &log)

		return out.String(), err
	}

	stdout, err := run("resolve")
	if want := "multi\t1.9\t" + filepath.Join("api-defs", "multi-openapi-1.9.json") + "\t" + filepath.Join("api-defs", "multi-1.9") + "\nmulti\t2.0\t" + filepath.Join("api-defs", "multi-openapi-2.0.json") + "\t" + filepath.Join("api-defs", "multi-2.0") + "\n"; err != nil || stdout != want {
		t.Errorf("resolve = %v:\n%s", err, stdout)
	}
	for _, cmd := range []string{"import", "generate", "check"} {
		// check says two things per version
		want := 2
		if cmd == "check" {
			want = 4
		}
		if stdout, err := run(cmd); err != nil || strings.Count(stdout, "multi: ") != want {
			t.Fatalf("%s = %v:\n%s", cmd, err, stdout)
		}
	}
	v1, v2 := listFiles(t, filepath.Join(root, "sdk", "multi", "v1")), listFiles(t, filepath.Join(root, "sdk", "multi", "v2"))
	if !strings.Contains(v1["client.go"], `const APIVersion = "1.9"`) || !strings.Contains(v2["client.go"], `const APIVersion = "2.0"`) || !strings.Contains(v2["doc.go"], "api-defs/multi-2.0") {
		t.Errorf("each package is not its own version:\n%s\n%s", v1["client.go"], v2["client.go"])
	}

	// one version at a time
	if stdout, err := run("generate", "-version", "2.0"); err != nil || strings.Count(stdout, "multi: ") != 1 || !strings.Contains(stdout, filepath.Join("sdk", "multi", "v2")) {
		t.Errorf("generate -version 2.0 = %v:\n%s", err, stdout)
	}
	if _, err := run("generate", "-version", "3.0"); err == nil || !strings.Contains(err.Error(), "multi: no version 3.0 is vendored") {
		t.Errorf("generate -version 3.0 = %v", err)
	}
}

// A grouped service gets its packages, with the import path read from the
// repository's go.mod, and check walks them.
func TestGroupedRepository(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.org/repo\n\ngo 1.27\n")
	writeFile(t, filepath.Join(root, "api-defs", "mini-openapi-1.9.json"), miniDocument)
	cfg := pandorest.Config{Services: []config.Service{{Name: "mini", Package: "mini", Output: "sdk/mini", Naming: config.OperationIDNaming, Auth: "APIKey", ClientService: "service", Grouped: true}}, Workarounds: []workarounds.Workaround{searchTerm{}}}
	run := func(args ...string) (string, error) {
		var out, log bytes.Buffer
		err := pandorest.Run(cfg, append(args, "-root", root, "-quiet"), &out, &log)

		return out.String(), err
	}
	for _, cmd := range []string{"import", "generate", "check"} {
		if stdout, err := run(cmd); err != nil {
			t.Fatalf("%s = %v:\n%s", cmd, err, stdout)
		}
	}
	if method := read(t, filepath.Join(root, "sdk", "mini", "items"), "method_getitems.go"); !strings.Contains(method, `"example.org/repo/sdk/mini"`) {
		t.Errorf("the items package does not import the types package by the module's path:\n%s", method)
	}
	if aggregate := read(t, filepath.Join(root, "sdk", "mini", "client"), "client.go"); !strings.Contains(aggregate, "items.Client") {
		t.Errorf("client lacks the Items field:\n%s", aggregate)
	}

	// without go.mod or Module, generate says what it needs; with Module it needs no go.mod
	if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
	if _, err := run("generate"); err == nil || !strings.Contains(err.Error(), "the grouped layout needs the import path of sdk/mini, and neither the config (Module) nor go.mod says it") {
		t.Errorf("generate without go.mod = %v", err)
	}
	cfg.Services[0].Module = "example.org/other"
	if stdout, err := run("generate"); err != nil {
		t.Fatalf("generate with Module = %v:\n%s", err, stdout)
	}
	if method := read(t, filepath.Join(root, "sdk", "mini", "items"), "method_getitems.go"); !strings.Contains(method, `"example.org/other/sdk/mini"`) {
		t.Errorf("the items package does not import the types package under Module:\n%s", method)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()

	return listFiles(t, dir)[name]
}

// A refreshed document is vendored beside the old one and takes over by being
// the higher version; the diff says what it changes, and the definitions are
// stale until it is imported.
func TestRefreshedDocument(t *testing.T) {
	t.Parallel()

	root := newRepository(t)
	for _, cmd := range []string{"import", "generate"} {
		if _, _, err := runCmd(t, cmd, "-root", root); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(root, "sdk", "mini", "extra.go"), "package mini\n")

	// 1.10 is above 1.9: it drops the lookup and gives the list a filter
	refreshed := strings.Replace(miniDocument, `"version": "1.9"`, `"version": "1.10"`, 1)
	refreshed = strings.Replace(refreshed, `{"name": "limit", "in": "query", "schema": {"type": "integer"}}`, `{"name": "limit", "in": "query", "schema": {"type": "integer"}}, {"name": "tag", "in": "query", "schema": {"type": "string"}}`, 1)
	start, end := strings.Index(refreshed, `"/Items/{id}"`), strings.Index(refreshed, `"components"`)
	refreshed = strings.TrimRight(strings.TrimSpace(refreshed[:start]), ",") + "},\n  " + refreshed[end:]
	writeFile(t, filepath.Join(root, "api-defs", "mini-openapi-1.10.json"), refreshed)

	stdout, _, err := runCmd(t, "resolve", "-root", root, "-service", "mini")
	if err != nil || !strings.HasPrefix(stdout, "mini\t1.10\t") {
		t.Errorf("resolve after a refresh = %v: %s", err, stdout)
	}
	// its definitions are not imported yet, so there is nothing to check against
	if _, _, err := runCmd(t, "check", "-root", root, "-service", "mini", "-quiet"); err == nil {
		t.Error("check passed with no definitions for the refreshed document")
	}

	if _, _, err := runCmd(t, "import", "-root", root, "-service", "mini", "-quiet"); err != nil {
		t.Fatal(err)
	}
	older, newer := filepath.Join(root, "api-defs", "mini-1.9"), filepath.Join(root, "api-defs", "mini-1.10")
	stdout, _, err = runCmd(t, "diff", "-old", older, "-new", newer)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mini: 0 added, 1 removed, 1 changed (1 breaking)", "- operation GetItem (GET /Items/{id}) [breaking]", "+ option query tag: String", "document: Mini 1.9 -> Mini 1.10"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the diff lacks %q:\n%s", want, stdout)
		}
	}
	if _, _, err := runCmd(t, "diff", "-old", older, "-new", newer, "-exit-code"); !errors.Is(err, pandorest.ErrChanges) {
		t.Errorf("diff -exit-code with changes = %v", err)
	}
	if stdout, _, err := runCmd(t, "diff", "-old", older, "-new", older, "-exit-code"); err != nil || stdout != "mini: no changes\n" {
		t.Errorf("diff of a directory with itself = %v: %s", err, stdout)
	}
	if _, _, err := runCmd(t, "diff", "-old", older, "-new", filepath.Join(root, "nowhere")); err == nil {
		t.Error("diff with a directory that is not there succeeded")
	}

	// generating from it removes the dropped operation's files and leaves the
	// hand-written one beside them
	if _, _, err := runCmd(t, "generate", "-root", root, "-service", "mini"); err != nil {
		t.Fatal(err)
	}
	files := listFiles(t, filepath.Join(root, "sdk", "mini"))
	if _, ok := files["items_method_getitem.go"]; ok {
		t.Error("the dropped operation's method survived")
	}
	if _, ok := files["items_method_getitem_test.go"]; ok {
		t.Error("the dropped operation's test survived")
	}
	if _, ok := files["extra.go"]; !ok {
		t.Error("the hand-written file was removed")
	}
	if _, _, err := runCmd(t, "check", "-root", root, "-quiet"); err != nil {
		t.Errorf("check after importing and generating the refreshed document = %v", err)
	}

	// a document edited after its import leaves the definitions stale: the
	// check fails saying what differs, and the diff exits with it
	writeFile(t, filepath.Join(root, "api-defs", "mini-openapi-1.10.json"), strings.Replace(refreshed, `"name": "tag"`, `"name": "label"`, 1))
	if _, _, err := runCmd(t, "check", "-root", root, "-service", "mini", "-quiet"); err == nil || !strings.Contains(err.Error(), "no longer matches") || !strings.Contains(err.Error(), "+ option query label: String") {
		t.Errorf("check with stale definitions = %v", err)
	}
	if stdout, _, err := runCmd(t, "diff", "-root", root, "-service", "mini", "-quiet", "-exit-code"); !errors.Is(err, pandorest.ErrChanges) || !strings.Contains(stdout, "- option query tag: String [breaking]") {
		t.Errorf("diff with stale definitions = %v:\n%s", err, stdout)
	}
	if _, _, err := runCmd(t, "diff", "-root", root, "-service", "mini", "-quiet"); err != nil {
		t.Errorf("diff without -exit-code = %v, want the report and no error", err)
	}
}

// A workaround whose bug the document no longer has fails the import, naming
// itself, rather than doing nothing from then on.
func TestWorkaroundNoLongerApplies(t *testing.T) {
	t.Parallel()

	root := newRepository(t)
	fixed := strings.Replace(miniDocument, `{"name": "limit", "in": "query", "schema": {"type": "integer"}}`, `{"name": "limit", "in": "query", "schema": {"type": "integer"}}, {"name": "searchTerm", "in": "query", "schema": {"type": "string"}}`, 1)
	writeFile(t, filepath.Join(root, "api-defs", "mini-openapi-1.9.json"), fixed)

	if _, _, err := runCmd(t, "import", "-root", root); err == nil || !strings.Contains(err.Error(), "workaround mini-search-term no longer applies, so remove it") {
		t.Errorf("import of a fixed document = %v", err)
	}
	// the workarounds pass their own test against the document as vendored
	load := func(string) (*openapi.Spec, error) { return openapi.Parse([]byte(miniDocument)) }
	if err := workarounds.Verify(repository.Workarounds, load); err != nil {
		t.Errorf("Verify = %v", err)
	}
}
