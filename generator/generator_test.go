package generator_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/pandorest/config"
	"github.com/katbyte/pandorest/definitions"
	"github.com/katbyte/pandorest/generator"
	"github.com/katbyte/pandorest/importer"
	"github.com/katbyte/pandorest/importer/workarounds"
	"github.com/katbyte/pandorest/jsonrpc"
	"github.com/katbyte/pandorest/openapi"
)

// miniSpec has one of every method shape the generator writes.
const miniSpec = `{
  "openapi": "3.0.1",
  "info": {"title": "Mini", "version": "1"},
  "paths": {
    "/Items": {"get": {"operationId": "GetItems", "tags": ["Items"], "summary": "Gets items. More words.", "parameters": [
        {"name": "userId", "in": "query", "schema": {"type": "string"}},
        {"name": "startIndex", "in": "query", "schema": {"type": "integer"}},
        {"name": "limit", "in": "query", "schema": {"type": "integer"}},
        {"name": "ticks", "in": "query", "schema": {"type": "integer", "format": "int64"}},
        {"name": "rating", "in": "query", "schema": {"type": "number"}},
        {"name": "isFavorite", "in": "query", "schema": {"type": "boolean"}},
        {"name": "kind", "in": "query", "schema": {"$ref": "#/components/schemas/Kind"}},
        {"name": "kinds", "in": "query", "schema": {"type": "array", "items": {"$ref": "#/components/schemas/Kind"}}},
        {"name": "years", "in": "query", "schema": {"type": "array", "items": {"type": "integer"}}},
        {"name": "ids", "in": "query", "explode": false, "schema": {"type": "array", "items": {"type": "string"}}},
        {"name": "old", "in": "query", "deprecated": true, "schema": {"type": "string"}},
        {"name": "X-Client", "in": "header", "schema": {"type": "string"}},
        {"name": "streamOptions", "in": "query", "schema": {"type": "object", "additionalProperties": {"type": "string"}}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/QueryResult"}}}}}}},
    "/Items/{itemId}": {
      "get": {"operationId": "GetItem", "tags": ["Items"], "parameters": [{"name": "itemId", "in": "path", "schema": {"type": "string"}}],
        "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}}}},
      "post": {"operationId": "UpdateItem", "tags": ["Items"], "parameters": [{"name": "itemId", "in": "path", "schema": {"type": "string"}}],
        "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}},
        "responses": {"204": {"description": "ok"}}},
      "delete": {"operationId": "DeleteItem", "tags": ["Items"], "deprecated": true, "parameters": [{"name": "itemId", "in": "path", "schema": {"type": "string"}}],
        "responses": {"204": {"description": "ok"}}}},
    "/Items/{itemId}/Images/{imageType}/{index}": {
      "get": {"operationId": "GetItemImage", "tags": ["Image"], "parameters": [
          {"name": "itemId", "in": "path", "schema": {"type": "string"}},
          {"name": "imageType", "in": "path", "schema": {"$ref": "#/components/schemas/Kind"}},
          {"name": "index", "in": "path", "schema": {"type": "integer"}}],
        "responses": {"200": {"content": {"image/*": {"schema": {"type": "string", "format": "binary"}}}}}},
      "post": {"operationId": "SetItemImage", "tags": ["Image"], "parameters": [
          {"name": "itemId", "in": "path", "schema": {"type": "string"}},
          {"name": "imageType", "in": "path", "schema": {"$ref": "#/components/schemas/Kind"}},
          {"name": "index", "in": "path", "schema": {"type": "integer"}}],
        "requestBody": {"content": {"image/*": {"schema": {"type": "string", "format": "binary"}}}},
        "responses": {"204": {"description": "ok"}}}},
    "/Sync/{jobId}/{ratio}": {"put": {"operationId": "PutSync", "tags": ["Sync"], "parameters": [
          {"name": "jobId", "in": "path", "schema": {"type": "integer", "format": "int64"}},
          {"name": "ratio", "in": "path", "schema": {"type": "number"}}],
        "requestBody": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Item"}}}}},
        "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"type": "string"}}}}}}}},
    "/Keys": {"get": {"operationId": "GetKeys", "tags": ["Sync"], "responses": {"200": {"content": {"application/json": {}}}}}},
    "/Name": {"get": {"operationId": "GetName", "tags": ["Sync"], "responses": {"200": {"content": {"application/json": {"schema": {"type": "string"}}}}}}},
    "/Kinds": {"get": {"operationId": "GetKind", "tags": ["Sync"], "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Kind"}}}}}}},
    "/Union": {"post": {"operationId": "PostUnion", "tags": ["Sync"],
        "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Union"}}}},
        "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Union"}}}}}}},
    "/Linux": {"get": {"operationId": "GetLinux", "tags": ["Sync"], "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Linux"}}}}}}}
  },
  "components": {"schemas": {
    "Kind": {"type": "string", "enum": ["Movie", "Series"]},
    "Union": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/Item"}]},
    "Linux": {"type": "object"},
    "Item": {"type": "object", "properties": {
      "Id": {"type": "string"}, "IsFolder": {"type": "boolean", "nullable": true}, "Parent": {"$ref": "#/components/schemas/Item"},
      "Kind": {"$ref": "#/components/schemas/Kind"}, "Children": {"type": "array", "items": {"$ref": "#/components/schemas/Item"}},
      "Blur": {"type": "object", "properties": {"Primary": {"type": "object", "additionalProperties": {"type": "string"}}}},
      "Rating": {"type": "number", "format": "float"}, "Season": {"type": "integer", "nullable": true}, "Episode": {"type": "integer", "nullable": true}}},
    "QueryResult": {"type": "object", "properties": {"Items": {"type": "array", "items": {"$ref": "#/components/schemas/Item"}}, "TotalRecordCount": {"type": "integer"}}}
  }}
}`

func miniDefinitions(t *testing.T) *definitions.Service {
	t.Helper()

	spec, err := openapi.Parse([]byte(miniSpec))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := importer.FromSpec(miniConfig, spec, []string{"mini-fix"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	return svc
}

var miniConfig = config.Service{
	Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "Bearer", KeepNull: []string{"Item.Season"},
	Paging: &config.Paging{Start: "startIndex", Limit: "limit", Items: "Items", Total: "TotalRecordCount"},
}

// testClient is the base client the generated package is built on:
// pandorest's own.
const testClient = config.DefaultClient

// generateInModule writes the package under testdata in this directory, so
// it builds against the module's base client, and removes it afterwards.
func generateInModule(t *testing.T, svc *definitions.Service, opts generator.Options) string {
	t.Helper()

	// kept when asked, each flat package under the test's own name, for the lint of what the generator writes (make lint-generated);
	// a grouped package stays at testdata/mini, which its import path names
	keep := os.Getenv("PANDOREST_KEEP_TESTDATA") != ""
	if opts.Dir == "" {
		opts.Dir = filepath.Join("testdata", "mini")
		if keep && !opts.Grouped {
			opts.Dir = filepath.Join("testdata", strings.ToLower(t.Name()))
		}
	}
	opts.Definitions = "api-defs/mini"
	opts.ClientImport = testClient
	if err := os.RemoveAll(opts.Dir); err != nil {
		t.Fatal(err)
	}
	if !keep {
		t.Cleanup(func() { _ = os.RemoveAll("testdata") })
	}
	if _, err := generator.Generate(svc, opts); err != nil {
		t.Fatal(err)
	}

	return opts.Dir
}

// goTool runs the go command on the generated package.
func goTool(t *testing.T, dir string, args ...string) {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "go", append(args, "./"+filepath.ToSlash(dir))...).CombinedOutput() //nolint:gosec // a fixed command on the test's own package
	if err != nil {
		t.Fatalf("go %s on the generated package: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // a generated test file
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

// An operation a workaround gave an Accept option leaves what it asks for to
// its caller, the way any header option is the caller's: the option's value
// is the whole header, in place of JSON or of a file's own type, and the
// generated test holds the method to that.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestAcceptOption(t *testing.T) {
	spec, err := openapi.Parse([]byte(miniSpec))
	if err != nil {
		t.Fatal(err)
	}
	if err := workarounds.AddParameters(spec, []string{"GET /Items/{itemId}", "GET /Items/{itemId}/Images/{imageType}/{index}"}, workarounds.Param{Name: "Accept", In: openapi.InHeader, Type: openapi.TypeString, Description: "What to ask for."}); err != nil {
		t.Fatal(err)
	}
	svc, err := importer.FromSpec(miniConfig, spec, []string{"mini-fix"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	method := read(t, dir, "items_method_getitem.go")
	for _, want := range []string{"\t// What to ask for.\n\tAccept string\n", `out.Append("Accept", o.Accept)`, "func (c Client) GetItem(ctx context.Context, itemId string, options ...GetItemOptions) ("} {
		if !strings.Contains(method, want) {
			t.Errorf("GetItem has no %q:\n%s", want, method)
		}
	}

	// the file's own type is still what the method asks for when the option is left empty, and its test sets the option
	image, imageTest := read(t, dir, "image_method_getitemimage.go"), read(t, dir, "image_method_getitemimage_test.go")
	if !strings.Contains(image, `Accept: "image/*",`) || !strings.Contains(imageTest, `got != "v-Accept"`) || strings.Contains(imageTest, "asked for first") {
		t.Errorf("GetItemImage with an Accept option:\n%s\n%s", image, imageTest)
	}
}

// An operation whose answer is text read whole holds it in its model as it
// was sent, asks for its type, and has a test that sends text labelled JSON.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestTextAnswer(t *testing.T) {
	spec, err := openapi.Parse([]byte(miniSpec))
	if err != nil {
		t.Fatal(err)
	}
	text := workarounds.WrongAnswers(workarounds.About{Name: "mini-name-text", Service: "mini", Bug: "the name is sent bare"}, map[string]workarounds.Correction{
		"GET /Name": {Declared: workarounds.JSONString(), Answers: workarounds.Text("text/plain")},
	})
	if err := text.Apply(spec); err != nil {
		t.Fatal(err)
	}
	svc, err := importer.FromSpec(miniConfig, spec, []string{"mini-fix"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	method := read(t, dir, "sync_method_getname.go")
	for _, want := range []string{"// The operation answers text (text/plain), which Answer holds as it was sent.", "\tAnswer       *string\n", `Accept: "text/plain",`, "if err := resp.Text(result.Answer); err != nil {"} {
		if !strings.Contains(method, want) {
			t.Errorf("GetName has no %q:\n%s", want, method)
		}
	}
	if strings.Contains(method, "Unmarshal") || strings.Contains(method, "StreamResponse") {
		t.Errorf("GetName decodes or streams its text:\n%s", method)
	}

	test := read(t, dir, "sync_method_getname_test.go")
	if !strings.Contains(test, `newOperationServer(t, 200, "application/json", "some text, as sent\n")`) || !strings.Contains(test, `*result.Answer != "some text, as sent\n"`) || strings.Contains(test, "does not decode") {
		t.Errorf("GetName's test:\n%s", test)
	}
}

// The grouped layout: every type in the package at the root, a package per
// tag holding only its client and methods, client holding New and a Client
// with a field per tag, and the map a caller rewrites by. The packages build
// and their tests pass.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestGrouped(t *testing.T) {
	svc := miniDefinitions(t)
	dir := generateInModule(t, svc, generator.Options{Grouped: true, ImportPath: "github.com/katbyte/pandorest/generator/testdata/mini", ClientService: "service", Packages: map[string]string{"Image": "images", "Sync": "syncs"}})
	writeServiceFile(t, dir)
	goTool(t, dir+"/...", "vet")
	goTool(t, dir+"/...", "test")

	for _, want := range []string{
		"doc.go", "client.go", "helpers.go", "model_item.go", "model_queryresult.go", "constants_common.go", "operation_getitems.go", "operation_getitemimage.go",
		"items/doc.go", "items/client.go", "items/method_getitems.go", "items/method_getitems_test.go", "items/client_test.go", "images/method_setitemimage.go", "syncs/method_putsync.go",
		"client/doc.go", "client/client.go", "client/client_test.go", "groups.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(want))); err != nil {
			t.Errorf("no %s", want)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "common")); len(entries) > 0 {
		t.Error("the common group, which has no operations, got a package")
	}
	// a tag whose name the standard library uses is refused until Packages names it
	if _, err := generator.Render(svc, generator.Options{Grouped: true, ImportPath: "x/mini", ClientImport: testClient}); err == nil || !strings.Contains(err.Error(), `the tag Image would be package "image", which the standard library or this layout already uses: name it in Packages`) {
		t.Errorf("a tag named like a standard library package = %v", err)
	}
	if _, err := generator.Render(svc, generator.Options{Grouped: true, ImportPath: "x/mini", ClientImport: testClient, Packages: map[string]string{"Image": "images", "Sync": "syncs", "Ghost": "g"}}); err == nil || !strings.Contains(err.Error(), "Packages names the tag Ghost, which the document has no operations under") {
		t.Errorf("a stale Packages entry = %v", err)
	}

	// the types package names everything a caller spells, with the base client's helpers under its own name
	root := read(t, dir, "operation_getitems.go") + read(t, dir, "helpers.go") + read(t, dir, "client.go")
	for _, want := range []string{"type GetItemsOptions struct {", "type GetItemsResponse struct {", "type GetItemsCompleteResult struct {", "type Option = client.Option", "func IsNotFound(err error) bool { return client.IsNotFound(err) }", "func New(baseURL, token string, opts ...client.Option) (*client.Client, error) {", "return service.New(baseURL, client.Bearer(token), append(slices.Clip(opts), client.WithSecrets(token))...)"} {
		if !strings.Contains(root, want) {
			t.Errorf("the types package lacks %q", want)
		}
	}

	// a tag's package has the method, naming the types as the root's, and nothing a caller names
	method := read(t, dir, "items/method_getitems.go")
	for _, want := range []string{"package items", `"github.com/katbyte/pandorest/generator/testdata/mini"`, "func (c Client) GetItems(ctx context.Context, options ...mini.GetItemsOptions) (result mini.GetItemsResponse, err error) {", "func (c Client) GetItemsComplete(", "var answer mini.QueryResult"} {
		if !strings.Contains(method, want) {
			t.Errorf("items' method file lacks %q:\n%s", want, method)
		}
	}
	if strings.Contains(method, "type GetItems") {
		t.Errorf("items declares a type a caller would name:\n%s", method)
	}
	if doc := read(t, dir, "items/doc.go"); !generator.Generated([]byte(doc)) || strings.HasPrefix(doc, generator.Header) {
		t.Errorf("items' doc.go is not the one file without the header:\n%s", doc)
	}

	// the client of every tag
	aggregate := read(t, dir, "client/client.go")
	for _, want := range []string{"package client", `base "github.com/katbyte/pandorest/client"`, "\tClient *base.Client\n", " items.Client\n", " images.Client\n", "func New(baseURL, token string, opts ...base.Option) (*Client, error) {", "c, err := mini.New(baseURL, token, opts...)", "items.NewClient(c),"} {
		if !strings.Contains(aggregate, want) {
			t.Errorf("client lacks %q:\n%s", want, aggregate)
		}
	}
	if groups := read(t, dir, "groups.json"); !strings.Contains(groups, `"GetItems": "Items"`) || !strings.Contains(groups, `"GetItemsComplete": "Items"`) || !strings.Contains(groups, `"Package": "items"`) {
		t.Errorf("groups.json:\n%s", groups)
	}

	// under a directory not named for the package (a version's), the types package is a directory of its own
	versioned := generateInModule(t, svc, generator.Options{Grouped: true, ImportPath: "github.com/katbyte/pandorest/generator/testdata/mini", ClientService: "service", Packages: map[string]string{"Image": "images", "Sync": "syncs"}, Dir: filepath.Join("testdata", "1.0"), ClientPackage: "miniclient"})
	for _, want := range []string{"mini/doc.go", "mini/model_item.go", "mini/client.go", "items/method_getitems.go", "miniclient/client.go"} {
		if _, err := os.Stat(filepath.Join(versioned, filepath.FromSlash(want))); err != nil {
			t.Errorf("versioned: no %s", want)
		}
	}
	if method := read(t, versioned, "items/method_getitems.go"); !strings.Contains(method, `"github.com/katbyte/pandorest/generator/testdata/mini/mini"`) {
		t.Errorf("versioned: items does not import the types package under the version:\n%s", method)
	}
	// it imports testdata/mini as the last test to write there left it, so it is not kept for the lint
	if err := os.RemoveAll(versioned); err != nil {
		t.Fatal(err)
	}

	// regenerating flat on top removes the packages, and grouped on top of flat the prefixed files
	flat := generateInModule(t, svc, generator.Options{Dir: filepath.Join("testdata", "mini")})
	if _, err := os.Stat(filepath.Join(flat, "items")); err == nil {
		t.Error("the items package survived a flat regeneration")
	}
}

// writeServiceFile writes the hand-written file a generated package needs
// beside it: the client.Service the constructor makes its client from.
func writeServiceFile(t *testing.T, dir string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte("package mini\n\nimport \"github.com/katbyte/pandorest/client\"\n\nvar service = client.Service{Name: \"Mini\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

//nolint:paralleltest // writes the testdata package the other test would remove
func TestGenerate(t *testing.T) {
	dir := generateInModule(t, miniDefinitions(t), generator.Options{Notes: "Dates are strings.\n\nIds are too."})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		files = append(files, e.Name())
	}
	for _, want := range []string{
		"client.go", "doc.go", "common_constants.go", "common_model_item.go", "items_model_queryresult.go",
		"common_model_itemblur.go", "sync_model_union.go", "items_method_getitems.go", "image_method_setitemimage.go",
		// every operation has a generated test, and the tests share client_test.go
		"items_method_getitems_test.go", "image_method_setitemimage_test.go", "client_test.go",
		// a file named for a GOOS would only build on it
		"sync_model_linuxtype.go",
	} {
		if !slices.Contains(files, want) {
			t.Errorf("no %s among %v", want, files)
		}
	}

	t.Run("signatures", func(t *testing.T) {
		all := strings.Join(func() []string {
			out := make([]string, 0, len(files))
			for _, f := range files {
				out = append(out, read(t, dir, f))
			}
			return out
		}(), "\n")
		for _, want := range []string{
			"func (c Client) GetItems(ctx context.Context, options ...GetItemsOptions) (result GetItemsResponse, err error) {",
			"func (c Client) GetItemsComplete(ctx context.Context, options ...GetItemsOptions) (result GetItemsCompleteResult, err error) {",
			"func (c Client) GetItem(ctx context.Context, itemId string) (result GetItemResponse, err error) {",
			"func (c Client) UpdateItem(ctx context.Context, itemId string, input Item) (result UpdateItemResponse, err error) {",
			"func (c Client) GetItemImage(ctx context.Context, itemId string, imageType Kind, index int) (result GetItemImageResponse, err error) {",
			"func (c Client) SetItemImage(ctx context.Context, itemId string, imageType Kind, index int, input io.Reader, contentType string) (result SetItemImageResponse, err error) {",
			"func (c Client) PutSync(ctx context.Context, jobId int64, ratio float64, input []Item) (result PutSyncResponse, err error) {",
			`"/Items/" + url.PathEscape(itemId) + "/Images/" + url.PathEscape(string(imageType)) + "/" + strconv.Itoa(index),`,
			`"/Sync/" + strconv.FormatInt(jobId, 10) + "/" + strconv.FormatFloat(ratio, 'f', -1, 64)`,
			"StreamResponse: true,",
			`Accept: "image/*",`,
			"http.StatusNoContent,",
			"// Deprecated: the document marks this operation deprecated.",
			"// Deprecated: the document marks this parameter deprecated.",
			// response models: pointer for a struct, primitive or constant; value for the rest
			"Answer       *QueryResult\n", "*QueryResult\n", "Answer       []string\n", "Answer       json.RawMessage\n",
			"Answer       *string\n", "Answer       *Kind\n", "Answer       Union\n",
			// options on the wire
			`out.Append("isFavorite", strconv.FormatBool(*o.IsFavorite))`,
			`out.Append("ticks", strconv.FormatInt(*o.Ticks, 10))`,
			`out.Append("kind", string(o.Kind))`,
			"for _, v := range o.Kinds {\n\t\tout.Append(\"kinds\", string(v))",
			"for _, v := range o.Years {\n\t\tout.Append(\"years\", strconv.Itoa(v))",
			`out.Append("ids", client.CSV(o.Ids))`,
			`out.Append("streamOptions", client.JSONObject(o.StreamOptions))`,
			`out.Append("X-Client", o.XClient)`,
			// model fields
			"IsFolder *bool", "Children []Item",
			"func PossibleValuesForKind() []string {",
			"type Union = json.RawMessage",
			"type Linux struct{}",
			// the default client.go: the base client's authorizer the service's Auth names
			"func New(baseURL, token string, opts ...client.Option) (*Client, error) {", "c, err := client.New(baseURL, client.Bearer(token), append(slices.Clip(opts), client.WithSecrets(token))...)", "// Client is a client for Mini; each of its operations is a method.",
			`const APIVersion = "1"`,
			// the package documentation: the base client, how the lists page, the service's notes, the workarounds
			"// under common_. The requests go through the hand-written base client,\n// " + testClient + ".\n",
			"// A list operation, one whose options page with StartIndex and Limit, also has a\n// <Name>Complete method",
			"//\n// Dates are strings.\n//\n// Ids are too.\n//\n// # Workarounds\n",
			"//   - mini-fix",
			// the paged server answers in the shape the service's lists have
			`fmt.Fprintf(w, "{%q:[%s],%q:%d}", items, item, total, count)`,
		} {
			if !strings.Contains(all, want) {
				t.Errorf("the package lacks %q", want)
			}
		}
		for _, re := range []string{"`json:\"IsFolder,omitempty\"`", "`json:\"Children,omitzero\"`", "`json:\"Parent,omitempty\"`", `Parent +\*Item`, `Blur +\*ItemBlur`, `Rating +float32`, `Ticks +\*int64`, `Season +\*int `, `Episode +int `} {
			if !regexp.MustCompile(re).MatchString(all) {
				t.Errorf("the package lacks %s", re)
			}
		}
		if !strings.HasPrefix(read(t, dir, "items_method_getitems.go"), generator.Header) {
			t.Error("a generated file lacks the header")
		}
	})

	// the generated package builds on a base client, and its own tests, one
	// for every operation, pass against one
	t.Run("builds and passes its tests", func(t *testing.T) {
		goTool(t, dir, "vet")
		goTool(t, dir, "test", "-count=1")
	})

	t.Run("check", func(t *testing.T) {
		svc := miniDefinitions(t)
		n, err := generator.Check(svc, dir)
		if err != nil || n != len(svc.Operations()) {
			t.Fatalf("Check = %d, %v", n, err)
		}
		path := filepath.Join(dir, "items_method_getitems.go")
		src := strings.Replace(read(t, dir, "items_method_getitems.go"), "func (c Client) GetItemsComplete(", "func (c Client) getItemsComplete(", 1)
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := generator.Check(svc, dir); err == nil || !strings.Contains(err.Error(), "GetItemsComplete (GET /Items)") {
			t.Errorf("Check with a method gone = %v", err)
		}
	})

	t.Run("stale files removed, hand-written kept", func(t *testing.T) {
		svc := miniDefinitions(t)
		stale := filepath.Join(dir, "sync_method_gone.go")
		kept := filepath.Join(dir, "extra_test.go")
		manual := filepath.Join(dir, "manual.go")
		for path, src := range map[string]string{stale: generator.Header + "\npackage mini\n", kept: "package mini\n", manual: "package mini\n"} {
			if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := generator.Generate(svc, generator.Options{Dir: dir, ClientImport: testClient}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Error("a stale generated file survived")
		}
		for _, path := range []string{kept, manual} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s was removed: %v", path, err)
			}
		}
	})

	t.Run("hand-written file where a generated one goes refused", func(t *testing.T) {
		svc := miniDefinitions(t)
		doc := filepath.Join(dir, "doc.go")
		manual := "// Package mini is written by hand.\npackage mini\n"
		if err := os.WriteFile(doc, []byte(manual), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := generator.Generate(svc, generator.Options{Dir: dir, ClientImport: testClient}); err == nil || !strings.Contains(err.Error(), "doc.go is written by hand, and generation would overwrite it") {
			t.Errorf("Generate over a hand-written doc.go = %v", err)
		}
		if got, err := os.ReadFile(doc); err != nil || string(got) != manual { //nolint:gosec // the test's own file
			t.Errorf("the hand-written doc.go was changed: %q, %v", got, err)
		}
		if err := os.Remove(doc); err != nil {
			t.Fatal(err)
		}
		// and with it gone, generation writes its own again
		if _, err := generator.Generate(svc, generator.Options{Dir: dir, ClientImport: testClient}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRenderRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		patch func(*definitions.Service, *generator.Options)
		want  string
	}{
		{"no base client", func(_ *definitions.Service, o *generator.Options) { o.ClientImport = "" }, "no base client to build the package on"},
		{"an auth that names no authorizer", func(s *definitions.Service, _ *generator.Options) { s.Auth = "an API key" }, `Auth "an API key" is not the name of an authorizer of ` + testClient},
		{"no auth", func(s *definitions.Service, _ *generator.Options) { s.Auth = "" }, `Auth "" is not the name of an authorizer`},
		{"bad package", func(s *definitions.Service, _ *generator.Options) { s.Package = "my-pkg" }, "is not a package name"},
		{"dangling reference", func(s *definitions.Service, _ *generator.Options) {
			s.Groups[0].Models = slices.DeleteFunc(s.Groups[0].Models, func(m definitions.Model) bool { return m.Name == "Item" })
		}, `reference to undefined type "Item"`},
		{"identifier clash", func(s *definitions.Service, _ *generator.Options) {
			for gi := range s.Groups {
				for oi := range s.Groups[gi].Operations {
					if s.Groups[gi].Operations[oi].Name == "GetKind" {
						s.Groups[gi].Operations[oi].Name = "GetItems" + "Complete"
					}
				}
			}
		}, "Client.GetItemsComplete is declared by both operation GetItems and operation GetItemsComplete"},
		{"a service value that is no name", func(_ *definitions.Service, o *generator.Options) { o.ClientService = "the service" }, `ClientService "the service" is not the name of a value in the package`},
		{"a service value named like a model", func(_ *definitions.Service, o *generator.Options) { o.ClientService = "Item" }, "Item is declared by both the service's own file and model Item"},
		{"a client that is not Go", func(_ *definitions.Service, o *generator.Options) { o.Client = "type Client struct {" }, "the config's Client is not Go source"},
		{"a client with no New", func(_ *definitions.Service, o *generator.Options) { o.Client = "type Client struct{}" }, "the config's Client does not declare New"},
		{"a client with no Client", func(_ *definitions.Service, o *generator.Options) { o.Client = "func New() {}" }, "the config's Client does not declare Client"},
		{"a client with imports", func(_ *definitions.Service, o *generator.Options) { o.Client = `import "time"` }, `it imports "time"; the generator writes the imports`},
		{"a client that declares a model's name", func(_ *definitions.Service, o *generator.Options) { o.Client = ownClient + "\nconst Item = 1\n" }, "Item is declared by both client.go and model Item"},
		{"a client that declares an operation's method", func(_ *definitions.Service, o *generator.Options) {
			o.Client = ownClient + "\nfunc (c *Client) GetItem() {}\n"
		}, "Client.GetItem is declared by both client.go and operation GetItem"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := miniDefinitions(t)
			opts := generator.Options{ClientImport: testClient}
			tt.patch(svc, &opts)
			if _, err := generator.Render(svc, opts); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Render = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// ownClient is a client.go a service's config gives in place of the default:
// a hosted API's, which knows where it lives and words its own errors.
const ownClient = `
// DefaultBaseURL is where the API lives.
const DefaultBaseURL = "https://api.example.org"

// Client is a client for the hosted API.
type Client struct {
	Client *client.Client
}

// New returns a client for the API at baseURL that authenticates with a read
// access token.
func New(baseURL, token string) (*Client, error) {
	if token == "" {
		return nil, errors.New("a read access token is required")
	}
	c, err := client.New(baseURL, client.Bearer(strings.TrimSpace(token)))
	if err != nil {
		return nil, err
	}

	return &Client{Client: c}, nil
}
`

// A service that gives its own client.go gets it, under the generated header
// and the document's version, with the imports it uses; and one with no
// lists gets no paged server in its tests and no word of paging in its docs.
//
//nolint:paralleltest // writes the testdata package the other test would remove
func TestGenerateOwnClient(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.1.0", "info": {"title": "hosted-api", "version": "3"}, "paths": {
    "/3/configuration": {"get": {"operationId": "configuration-details", "tags": ["Configuration"],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"change_keys": {"type": "array", "items": {"type": "string"}}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := importer.FromSpec(config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "a read access token"}, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{Client: ownClient})

	client := read(t, dir, "client.go")
	for _, want := range []string{
		generator.Header + "\npackage mini\n\nimport (\n\t\"errors\"\n\t\"strings\"\n\n\t\"" + testClient + "\"\n)\n",
		`const APIVersion = "3"`, `const DefaultBaseURL = "https://api.example.org"`, "client.New(baseURL, client.Bearer(strings.TrimSpace(token)))",
	} {
		if !strings.Contains(client, want) {
			t.Errorf("client.go has no %s:\n%s", want, client)
		}
	}
	if strings.Contains(client, "Client is a client for hosted-api") {
		t.Errorf("client.go has the default client as well as the service's own:\n%s", client)
	}
	if helpers := read(t, dir, "client_test.go"); strings.Contains(helpers, "pagedServer") || strings.Contains(helpers, `"fmt"`) {
		t.Errorf("a service with no lists has a paged server in its tests:\n%s", helpers)
	}
	if doc := read(t, dir, "doc.go"); strings.Contains(doc, "Complete") {
		t.Errorf("a service with no lists documents paging:\n%s", doc)
	}

	goTool(t, dir, "vet")
	goTool(t, dir, "test", "-count=1")
}

// A file is generated when it starts with the header, whichever copy of
// pandorest wrote it: the repositories that carried their own named it by its
// path, and their stale files must still be recognised once they regenerate
// with this one. A package's doc.go has no header, so that a linter sees the
// package comment in it, and is known by how it starts.
func TestGenerated(t *testing.T) {
	t.Parallel()

	for src, want := range map[string]bool{
		generator.Header + "\npackage x\n":                                     true,
		"// Package emby is a hand-written client.\npackage emby\n":            false,
		"// Code generated by sdk/pandorest; DO NOT EDIT.\n\npackage x\n":      true,
		"// Code generated by internal/pandorest; DO NOT EDIT.\n\npackage x\n": true,
		"// Code generated by mockgen; DO NOT EDIT.\n\npackage x\n":            false,
		"// Code generated by notpandorest; DO NOT EDIT.\n\npackage x\n":       false,
		"package x\n\n// Code generated by pandorest; DO NOT EDIT.\n":          false,
		// a doc.go names pandorest in its package comment and carries no header, so a linter sees the comment
		"// Package emby is a typed client for Emby Server REST API 4.10.0.40, generated by pandorest\n// from the definitions.\npackage emby\n": true,
		"package x\n": false,
		"":            false,
	} {
		if got := generator.Generated([]byte(src)); got != want {
			t.Errorf("Generated(%q) = %t, want %t", src, got, want)
		}
	}
}

// arrSpec is a document in the shape Sonarr, Radarr and Prowlarr publish: a
// list that pages by page number, a failure that answers the success shape, a
// required property, and a settings object saved whole.
const arrSpec = `{"openapi": "3.0.1", "info": {"title": "Arr", "version": "3.0.0"}, "paths": {
  "/api/v3/series/{seriesId}/history": {"get": {"tags": ["History"], "parameters": [
      {"name": "seriesId", "in": "path", "schema": {"type": "integer"}},
      {"name": "page", "in": "query", "schema": {"type": "integer", "format": "int32"}},
      {"name": "pageSize", "in": "query", "schema": {"type": "integer", "format": "int32"}}],
    "responses": {"200": {"content": {"text/plain": {"schema": {"$ref": "#/components/schemas/HistoryPage"}}, "application/json": {"schema": {"$ref": "#/components/schemas/HistoryPage"}}}}}}},
  "/api/v3/indexer/testall": {"post": {"tags": ["Indexer"], "responses": {
    "200": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Record"}}}}},
    "400": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Record"}}}}}}}},
  "/api/v3/config/host": {"put": {"tags": ["HostConfig"],
    "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/HostConfig"}}}},
    "responses": {"202": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/HostConfig"}}}}}}},
  "/login": {"post": {"tags": ["Authentication"], "responses": {"302": {"description": "on to where the caller was going"}}}},
  "/logout": {"get": {"tags": ["Authentication"], "responses": {"302": {"description": "to the front page"}}}}},
  "components": {"schemas": {
    "Record": {"type": "object", "required": ["title", "monitored"], "properties": {"id": {"type": "integer"}, "title": {"type": "string"}, "monitored": {"type": "boolean"}}},
    "HistoryPage": {"type": "object", "properties": {"page": {"type": "integer"}, "pageSize": {"type": "integer"}, "totalRecords": {"type": "integer"},
      "records": {"type": "array", "items": {"$ref": "#/components/schemas/Record"}}}},
    "HostConfig": {"type": "object", "properties": {"bindAddress": {"type": "string"}, "port": {"type": "integer"}, "sslPort": {"type": "integer", "nullable": true},
      "enableSsl": {"type": "boolean"}, "tags": {"type": "array", "items": {"type": "integer"}}}}}}}`

// A service words the New the generator writes without writing it: what its
// credential is called names the parameter and the refusal of an empty one,
// and its documentation is the service's own, as it is broken.
func TestNewInTheServicesWords(t *testing.T) {
	t.Parallel()

	opts := generator.Options{ClientImport: testClient, Credential: "API key", NewDoc: "New returns a client for the Arr server at baseURL (scheme and host, plus\nthe URL base when it is set up with one) that authenticates with the API\nkey from Settings > General."}
	files, err := generator.Render(miniDefinitions(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	src := string(files["client.go"])
	for _, want := range []string{
		"// New returns a client for the Arr server at baseURL (scheme and host, plus\n// the URL base when it is set up with one) that authenticates with the API\n// key from Settings > General.\nfunc New(baseURL, apiKey string, opts ...client.Option) (*Client, error) {\n",
		"\tif apiKey == \"\" {\n\t\treturn nil, errors.New(\"API key is required\")\n\t}\n",
		"c, err := client.New(baseURL, client.Bearer(apiKey), append(slices.Clip(opts), client.WithSecrets(apiKey))...)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("client.go lacks %q:\n%s", want, src)
		}
	}
	if strings.Contains(src, "token") {
		t.Errorf("client.go still speaks of a token:\n%s", src)
	}

	// what it is called alone words the default documentation too
	opts.NewDoc = ""
	files, err = generator.Render(miniDefinitions(t), opts)
	if err != nil {
		t.Fatal(err)
	}
	if src := string(files["client.go"]); !strings.Contains(src, "// path prefix) that authenticates with apiKey. The options are the base\n") {
		t.Errorf("the default documentation does not name the parameter:\n%s", src)
	}

	for name, tc := range map[string]struct {
		patch func(*generator.Options)
		want  string
	}{
		"a credential that makes no name":  {func(o *generator.Options) { o.Credential = "--" }, `Credential "--" makes no name for New's parameter`},
		"one that makes a name New uses":   {func(o *generator.Options) { o.Credential = "base URL" }, `Credential "base URL" makes no name for New's parameter ("baseURL")`},
		"one that makes a keyword":         {func(o *generator.Options) { o.Credential = "type" }, `Credential "type" makes no name for New's parameter`},
		"documentation of something else":  {func(o *generator.Options) { o.NewDoc = "Returns a client." }, "NewDoc must document New, and start with its name"},
		"words beside a client of its own": {func(o *generator.Options) { o.Client = ownClient }, "words the default client.go's New (Credential, NewDoc) and gives a Client of its own as well"},
	} {
		opts := generator.Options{ClientImport: testClient, Credential: "API key"}
		tc.patch(&opts)
		if _, err := generator.Render(miniDefinitions(t), opts); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Render = %v, want containing %q", name, err, tc.want)
		}
	}
}

// With InlineSingleOptions an operation with one option takes it as an
// argument: variadic when optional, plain when required, a list as its
// elements, the name kept clear of the body's own; a list that pages and an
// operation with two keep their struct. The generated tests prove the wire.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestInlineSingleOptions(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
    "/lookup": {"get": {"operationId": "GetLookup", "tags": ["Items"], "parameters": [{"name": "term", "in": "query", "schema": {"type": "string"}}], "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}},
    "/force": {"post": {"operationId": "PostForce", "tags": ["Items"], "parameters": [{"name": "forceSave", "in": "query", "schema": {"type": "boolean"}}], "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}},
    "/season": {"get": {"operationId": "GetSeason", "tags": ["Items"], "parameters": [{"name": "seasonNumber", "in": "query", "required": true, "schema": {"type": "integer"}}], "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}},
    "/byids": {"get": {"operationId": "GetByIds", "tags": ["Items"], "parameters": [{"name": "ids", "in": "query", "schema": {"type": "array", "items": {"type": "string"}}}], "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}},
    "/typed": {"get": {"operationId": "GetTyped", "tags": ["Items"], "parameters": [{"name": "type", "in": "query", "schema": {"type": "string"}}], "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}},
    "/reqids": {"get": {"operationId": "GetReqIds", "tags": ["Items"], "parameters": [{"name": "ids", "in": "query", "required": true, "schema": {"type": "array", "items": {"type": "string"}}}], "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}},
    "/url": {"get": {"operationId": "GetUrl", "tags": ["Items"], "parameters": [{"name": "url", "in": "query", "schema": {"type": "string"}}], "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}},
    "/pair": {"get": {"operationId": "GetPair", "tags": ["Items"], "parameters": [{"name": "a", "in": "query", "schema": {"type": "string"}}, {"name": "b", "in": "query", "schema": {"type": "string"}}], "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"name": {"type": "string"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := importer.FromSpec(config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey"}, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{InlineSingleOptions: true})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	for file, wants := range map[string][]string{
		"items_method_getlookup.go": {"func (c Client) GetLookup(ctx context.Context, term ...string) (result GetLookupResponse, err error) {", `return result, fmt.Errorf("GetLookup takes one term at most, not %d", len(term))`, "option.Term = term[0]", "OptionsObject: option,"},
		"items_method_postforce.go": {"func (c Client) PostForce(ctx context.Context, forceSave ...bool) (", "option.ForceSave = &forceSave[0]"},
		"items_method_getseason.go": {"func (c Client) GetSeason(ctx context.Context, seasonNumber int) (", "option.SeasonNumber = &seasonNumber\n"},
		"items_method_getbyids.go":  {"func (c Client) GetByIds(ctx context.Context, ids ...string) (", "if len(ids) > 0 {\n\t\toption.Ids = ids\n\t}"},
		"items_method_gettyped.go":  {"func (c Client) GetTyped(ctx context.Context, typeOption ...string) ("},
		"items_method_getreqids.go": {"func (c Client) GetReqIds(ctx context.Context, ids []string) (", "option.Ids = ids\n"},
		"items_method_geturl.go":    {"func (c Client) GetUrl(ctx context.Context, urlOption ...string) ("},
		"items_method_getpair.go":   {"func (c Client) GetPair(ctx context.Context, options ...GetPairOptions) ("},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}
}

// A list that pages by page number alone, the page size being the server's
// (TMDB), gets a pager that learns the size from the first page; its
// generated test proves the walk.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestPagerWithoutLimit(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
    "/popular": {"get": {"operationId": "GetPopular", "tags": ["Movies"], "parameters": [{"name": "page", "in": "query", "schema": {"type": "integer"}}, {"name": "language", "in": "query", "schema": {"type": "string"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"page": {"type": "integer"}, "results": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "integer"}}}}, "total_pages": {"type": "integer"}, "total_results": {"type": "integer"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey", Paging: &config.Paging{Start: "page", Items: "results", Total: "total_results", ByPage: true}}
	svc, err := importer.FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	src := read(t, dir, "movies_method_getpopular.go")
	for _, want := range []string{"func (c Client) GetPopularComplete(ctx context.Context, options ...GetPopularOptions) (result GetPopularCompleteResult, err error) {", "limit := 0\n", "if limit == 0 {\n\t\t\tlimit = len(page.Answer.Results)\n\t\t}", "len(result.Items) >= page.Answer.TotalResults"} {
		if !strings.Contains(src, want) {
			t.Errorf("the pager lacks %q:\n%s", want, src)
		}
	}
}

// A service that gives the layouts its dates are written in has them as Time
// and Day: fields left out when zero, options sent in the first layout, and
// A service whose pages count from 0 says so with FirstPage, and its pager
// asks for page 0 first and counts the pages it has loaded from there; the
// generated tests expect the same. A list of the service's second paging
// shape is tested against an answer in its own names, not the first shape's.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestFirstPage(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
    "/items": {"get": {"operationId": "GetItems", "tags": ["Items"], "parameters": [{"name": "page", "in": "query", "schema": {"type": "integer"}}, {"name": "limit", "in": "query", "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"results": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "integer"}}}}, "total": {"type": "integer"}}}}}}}}},
    "/sessions": {"get": {"operationId": "GetSessions", "tags": ["Items"], "parameters": [{"name": "page", "in": "query", "schema": {"type": "integer"}}, {"name": "itemsPerPage", "in": "query", "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"sessions": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "integer"}}}}, "total": {"type": "integer"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey", Paging: &config.Paging{Start: "page", Limit: "limit", Items: "results", Total: "total", ByPage: true, FirstPage: new(0)}, Pagings: []config.Paging{{Start: "page", Limit: "itemsPerPage", Items: "sessions", Total: "total", ByPage: true, FirstPage: new(0)}}}
	svc, err := importer.FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	for file, wants := range map[string][]string{
		"items_method_getitems.go":         {"\tnumber := 0\n\tif option.Page != nil && *option.Page > 0 {\n\t\tnumber = *option.Page\n\t}\n\toption.Page = &number\n", "page.Answer.Total > 0 && (number+1)*limit >= page.Answer.Total {"},
		"items_method_getitems_test.go":    {`{"total reached", 2, 9, GetItemsOptions{Limit: new(1)}, 2, []string{"0", "1"}},`, `{"short page", 0, 9, GetItemsOptions{Limit: new(2)}, 1, []string{"0"}},`, `pagedServer(t, "page", "results", "total", "{}", tc.total, tc.pages)`},
		"items_method_getsessions_test.go": {`pagedServer(t, "page", "sessions", "total", "{}", tc.total, tc.pages)`},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}
}

// A record with a Name and an Id gets GetName and GetId, so one piece of code
// can take records of several types; a record lacking either, or one with a
// field already so named, gets neither.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestAccessors(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
    "/items": {"get": {"operationId": "GetItems", "tags": ["Items"], "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}}}}},
    "/tags": {"get": {"operationId": "GetTags", "tags": ["Items"], "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Tag"}}}}}}},
    "/odd": {"get": {"operationId": "GetOdd", "tags": ["Items"], "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Odd"}}}}}}}},
  "components": {"schemas": {
    "Item": {"type": "object", "properties": {"name": {"type": "string", "nullable": true}, "id": {"type": "integer"}}},
    "Tag": {"type": "object", "properties": {"name": {"type": "string"}, "id": {"type": "string"}}},
    "Odd": {"type": "object", "properties": {"name": {"type": "string"}, "id": {"type": "integer"}, "getName": {"type": "string"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey", KeepNull: []string{"Item.name"}}
	svc, err := importer.FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{})
	if err := os.WriteFile(filepath.Join(dir, "accessors_test.go"), []byte(`package mini

import "testing"

type named interface{ GetName() string }

func TestAccessors(t *testing.T) {
	t.Parallel()

	for _, r := range []named{Item{}, Item{Name: new("a")}, Tag{Name: "b", Id: "t1"}} {
		_ = r.GetName()
	}
	if (Item{Id: 3}).GetId() != 3 || (Tag{Id: "t1"}).GetId() != "t1" || (Item{}).GetName() != "" || (Item{Name: new("a")}).GetName() != "a" {
		t.Error("an accessor does not answer its field")
	}
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	goTool(t, dir, "vet")
	goTool(t, dir, "test")
	if src := read(t, dir, "items_model_odd.go"); strings.Contains(src, "GetName()") {
		t.Errorf("a record with a GetName field got an accessor:\n%s", src)
	}
}

// the generated tests prove both on the wire.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestDates(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
    "/items": {"get": {"operationId": "GetItems", "tags": ["Items"], "parameters": [{"name": "since", "in": "query", "schema": {"type": "string", "format": "date-time"}}, {"name": "born", "in": "query", "schema": {"type": "string", "format": "date"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}}}}},
    "/items/{when}": {"get": {"operationId": "GetItemAt", "tags": ["Items"], "parameters": [{"name": "when", "in": "path", "required": true, "schema": {"type": "string", "format": "date-time"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}}}}}},
  "components": {"schemas": {"Item": {"type": "object", "properties": {"name": {"type": "string"}, "created": {"type": "string", "format": "date-time"}, "born": {"type": "string", "format": "date"}, "plain": {"type": "string"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey", Dates: config.Dates{Send: "2006-01-02T15:04:05.0000000Z07:00", Read: []string{time.RFC3339Nano}}, Days: config.Dates{Send: time.DateOnly}}
	svc, err := importer.FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{ClientService: "service"})
	writeServiceFile(t, dir)
	// the service's own file may pick a layout per parameter; proven here before the parallel tests run
	if err := os.WriteFile(filepath.Join(dir, "override_test.go"), []byte(`package mini

import (
	"testing"
	"time"
)

//nolint:paralleltest // sets the service's hook, which the parallel tests must not see
func TestSendDateAs(t *testing.T) {
	service.SendDateAs = func(operation, parameter string) string {
		if operation == "GetItems" && parameter == "since" {
			return "2006-01-02"
		}
		return ""
	}
	defer func() { service.SendDateAs = nil }()

	c, s := newOperationServer(t, 200, "application/json", "{}")
	if _, err := c.GetItems(t.Context(), GetItemsOptions{Since: Time{Time: time.Date(2024, time.March, 4, 5, 6, 7, 0, time.UTC)}, Born: Day{Time: time.Date(2024, time.March, 4, 0, 0, 0, 0, time.UTC)}}); err != nil {
		t.Fatal(err)
	}
	r, _ := s.only(t)
	if got := r.URL.Query(); got.Get("since") != "2024-03-04" || got.Get("born") != "2024-03-04" {
		t.Errorf("query = %q", r.URL.RawQuery)
	}
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	for file, wants := range map[string][]string{
		"time.go":                   {"type Time struct{ time.Time }", "func sendDateAs(operation, parameter string) string {", "if service.SendDateAs == nil {", "func (t Time) As(layout string) string {", "type Day struct{ time.Time }", `var timeLayouts = []string{"2006-01-02T15:04:05.0000000Z07:00", "2006-01-02T15:04:05.999999999Z07:00"}`, `var dayLayouts = []string{"2006-01-02"}`, "client.ParseTime(data, timeLayouts)"},
		"items_model_item.go":       {"Created Time   `json:\"created,omitzero\"`", "Born    Day    `json:\"born,omitzero\"`", "Plain   string `json:\"plain,omitempty\"`"},
		"items_method_getitems.go":  {"Since Time", "Born  Day", "if !o.Since.IsZero() {\n\t\tout.Append(\"since\", o.Since.As(sendDateAs(\"GetItems\", \"since\")))", "if !o.Born.IsZero() {\n\t\tout.Append(\"born\", o.Born.As(sendDateAs(\"GetItems\", \"born\")))"},
		"items_method_getitemat.go": {"func (c Client) GetItemAt(ctx context.Context, when Time) (", "url.PathEscape(when.As(sendDateAs(\"GetItemAt\", \"when\")))"},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}

	// set to keep a date no layout reads, the type holds its text and the decode goes on
	cfg.Dates.KeepUnread = true
	svc, err = importer.FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir = generateInModule(t, svc, generator.Options{})
	// without a service file of its own, the usual layout and no hook
	if src := read(t, dir, "items_method_getitems.go"); !strings.Contains(src, `out.Append("since", o.Since.String())`) {
		t.Errorf("without ClientService the query does not use the plain layout:\n%s", src)
	}
	goTool(t, dir, "vet")
	goTool(t, dir, "test")
	if src := read(t, dir, "time.go"); !strings.Contains(src, "\tUnread string\n") || !strings.Contains(src, "func (t Time) UnreadDate() string { return t.Unread }") || !strings.Contains(src, "if unread, ok := errors.AsType[*client.UnreadDateError](err); ok {") || strings.Contains(src, "func (t Day) UnreadDate()") {
		t.Errorf("time.go with KeepUnread on dates alone:\n%s", src)
	}

	// without the layouts, dates are the strings the document declares
	svc, err = importer.FromSpec(config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey"}, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	files, err := generator.Render(svc, generator.Options{ClientImport: testClient})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["time.go"]; ok || !strings.Contains(string(files["items_model_item.go"]), "Created string `json:\"created,omitempty\"`") {
		t.Errorf("without layouts: time.go %v, model:\n%s", ok, files["items_model_item.go"])
	}
}

// An integer a workaround declares an epoch is Unix or UnixMilli: a field
// left out when zero, an option and a path parameter sent as the number, and
// the generated tests prove both on the wire.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestUnixDates(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
    "/events": {"get": {"operationId": "GetEvents", "tags": ["Events"], "parameters": [{"name": "since", "in": "query", "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Event"}}}}}}},
    "/events/{at}": {"get": {"operationId": "GetEventAt", "tags": ["Events"], "parameters": [{"name": "at", "in": "path", "required": true, "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Event"}}}}}}},
    "/events/stream": {"get": {"operationId": "GetEventStream", "tags": ["Events"],
      "responses": {"200": {"content": {"text/event-stream": {"schema": {"type": "string", "format": "binary"}}}}}}}},
  "components": {"schemas": {"Event": {"type": "object", "properties": {"name": {"type": "string"}, "timestamp": {"type": "integer"}, "at": {"type": "integer"}, "since": {"type": "integer"}, "count": {"type": "integer"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := workarounds.UndeclaredUnixDates(workarounds.About{Name: "mini-epochs", Service: "mini", Bug: "epochs are declared integers"}, "seconds", "timestamp", "since").Apply(spec); err != nil {
		t.Fatal(err)
	}
	if err := workarounds.UndeclaredUnixDates(workarounds.About{Name: "mini-millis", Service: "mini", Bug: "at is milliseconds"}, "milliseconds", "at").Apply(spec); err != nil {
		t.Fatal(err)
	}
	svc, err := importer.FromSpec(config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey"}, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	for file, wants := range map[string][]string{
		"time.go":                         {"type Unix struct{ time.Time }", "type UnixMilli struct{ time.Time }", "return json.Marshal(t.Unix())", "t.Time = time.UnixMilli(n).UTC()", "func (t Unix) String() string { return strconv.FormatInt(t.Unix(), 10) }"},
		"events_model_event.go":           {"Timestamp Unix      `json:\"timestamp,omitzero\"`", "At        UnixMilli `json:\"at,omitzero\"`", "Count     int       `json:\"count,omitempty\"`"},
		"events_method_getevents.go":      {"Since Unix", "if !o.Since.IsZero() {\n\t\tout.Append(\"since\", o.Since.String())"},
		"events_method_geteventat.go":     {"func (c Client) GetEventAt(ctx context.Context, at UnixMilli) (", "+ at.String(),"},
		"events_method_geteventstream.go": {"func (r GetEventStreamResponse) Reader() (body io.ReadCloser, contentType string) {", "func (r GetEventStreamResponse) ContentLength() int64 {"},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}
}

// A kind of id is a named type in ids.go, carried by path parameters, fields
// and options, formatted by the method on a path the document types as text,
// sent joined where the service takes several in one parameter; the
// generated tests prove the wire.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestIDs(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
    "/series": {"get": {"operationId": "GetSeries", "tags": ["Series"], "parameters": [{"name": "ids", "in": "query", "schema": {"type": "string"}}, {"name": "tagIds", "in": "query", "schema": {"type": "array", "items": {"type": "integer"}}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Series"}}}}}}}},
    "/series/{id}": {"put": {"operationId": "PutSeriesById", "tags": ["Series"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
      "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Series"}}}},
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Series"}}}}}}},
    "/items/{itemId}": {"get": {"operationId": "GetItem", "tags": ["Items"], "parameters": [{"name": "itemId", "in": "path", "required": true, "schema": {"type": "string"}}, {"name": "userId", "in": "query", "schema": {"type": "string"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}}}}}},
  "components": {"schemas": {
    "Series": {"type": "object", "properties": {"id": {"type": "integer"}, "title": {"type": "string"}, "tags": {"type": "array", "items": {"type": "integer"}}}},
    "Item": {"type": "object", "properties": {"id": {"type": "string"}, "userId": {"type": "string"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey", IDs: []config.ID{
		{Name: "SeriesID", Of: []string{"Series"}, Joined: []string{"ids"}},
		{Name: "ItemID", Of: []string{"Item"}, Names: []string{"itemId"}, Shape: "^[0-9a-f]{4}$", Example: "0a1b"},
		{Name: "UserID", Base: "string", Names: []string{"userId"}},
		// a shape the sampler's 1 and 2 do not match, so a generated test of a list must use the example or refuse its own call
		{Name: "TagID", Base: "int", Names: []string{"tags", "tagIds"}, Shape: `^[5-9]\d*$`, Example: "7"},
	}}
	svc, err := importer.FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{Notes: "Dates are strings."})
	// the service's own notes come before the Ids heading, or they read as part of it
	if doc := read(t, dir, "doc.go"); !strings.Contains(doc, "// Dates are strings.") || strings.Index(doc, "// Dates are strings.") > strings.Index(doc, "// # Ids") {
		t.Errorf("doc.go has the notes under Ids:\n%s", doc)
	}
	// the conversion example names this package's first kind over its own base, not a kind from another service
	if doc := read(t, dir, "doc.go"); !strings.Contains(doc, "int(id) and SeriesID(n) convert one value") {
		t.Errorf("doc.go's Ids example does not name the first kind:\n%s", doc)
	}
	// a kind with a shape refuses a value without it before any request, naming the value
	if err := os.WriteFile(filepath.Join(dir, "shape_test.go"), []byte(`package mini

import (
	"strings"
	"testing"
)

func TestShape(t *testing.T) {
	t.Parallel()

	c, s := newOperationServer(t, 200, "application/json", "{}")
	if _, err := c.GetItem(t.Context(), ItemID("x32")); err == nil || err.Error() != "x32 is not an ItemID (the shape is ^[0-9a-f]{4}$)" {
		t.Errorf("a malformed id = %v", err)
	}
	if _, err := c.GetSeries(t.Context(), GetSeriesOptions{TagIds: []TagID{7, 0}}); err == nil || !strings.Contains(err.Error(), "0 is not a TagID") {
		t.Errorf("a malformed id among a list = %v", err)
	}
	if n := len(s.requests); n != 0 {
		t.Errorf("%d requests were sent", n)
	}
	if _, err := c.GetItem(t.Context(), ItemID("0a1b")); err != nil {
		t.Errorf("a well-formed id = %v", err)
	}
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	for file, wants := range map[string][]string{
		"ids.go":                         {"type SeriesID int\n", "type TagID int\n", "type ItemID string\n", "type UserID string\n", "var itemIDShape = regexp.MustCompile(\"^[0-9a-f]{4}$\")", "func (id ItemID) Check() error {", "func (id TagID) Check() error {"},
		"series_model_series.go":         {"Id    SeriesID `json:\"id,omitempty\"`", "Tags  []TagID  `json:\"tags,omitzero\"`"},
		"series_method_putseriesbyid.go": {"func (c Client) PutSeriesById(ctx context.Context, id SeriesID, input Series) (", `url.PathEscape(strconv.Itoa(int(id)))`},
		"series_method_getseries.go":     {"Ids    []SeriesID", "TagIds []TagID", `out.Append("ids", client.Joined(o.Ids))`, `out.Append("tagIds", strconv.Itoa(int(v)))`},
		// a list of a shaped kind is filled with its example, not 1 and 2, or the method refuses the call before the wire is seen
		"series_method_getseries_test.go": {"TagIds: []TagID{TagID(7), TagID(7)}"},
		"items_method_getitem.go":         {"func (c Client) GetItem(ctx context.Context, itemId ItemID, options ...GetItemOptions) (", "if err := itemId.Check(); err != nil {", `url.PathEscape(string(itemId))`, `if o.UserId != "" {`, `out.Append("userId", string(o.UserId))`},
		"items_model_item.go":             {"Id     ItemID `json:\"id,omitempty\"`", "UserId UserID `json:\"userId,omitempty\"`"},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}
}

// A joined list sits beside its string under one name on the wire, and the generated test sets one of the two, or it would send both
// and expect each alone (embyfin's move, 70 generated tests failing).
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestJoinedLists(t *testing.T) {
	spec, err := openapi.Parse([]byte(miniSpec))
	if err != nil {
		t.Fatal(err)
	}
	if err := workarounds.JoinedLists(workarounds.About{Name: "mini-lists", Service: "mini", Bug: "userId carries several joined by commas"}, map[string]workarounds.Joined{"userId": {Of: "string"}}).Apply(spec); err != nil {
		t.Fatal(err)
	}
	svc, err := importer.FromSpec(miniConfig, spec, []string{"mini-fix", "mini-lists"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	method := read(t, dir, "items_method_getitems.go")
	for _, want := range []string{"UserIdList []string", `out.Append("userId", client.CSV(o.UserIdList))`} {
		if !strings.Contains(method, want) {
			t.Errorf("the method lacks %q:\n%s", want, method)
		}
	}
	test := read(t, dir, "items_method_getitems_test.go")
	if !strings.Contains(test, `"v-UserId"`) || strings.Contains(test, "UserIdList:") {
		t.Errorf("the test does not set the string alone:\n%s", test)
	}
}

// callMethods is a description of a few JSON-RPC methods as TrueNAS writes its own: a query, a job taking a record, a method
// answering nothing, one answering an inline record with a date and an enum, and one with an argument named as a method's
// local is.
const callMethods = `{
  "config.save": {"description": "Save the configuration.", "roles": ["READONLY_ADMIN"], "filterable": false, "job": true, "downloadable": true,
    "accepts": [{"title": "options", "type": "object", "properties": {"secretseed": {"title": "secretseed", "type": "boolean", "_name_": "secretseed", "_required_": false}}, "_name_": "options", "_required_": false}],
    "returns": [{"title": "result", "type": "null", "_name_": "result", "_required_": true}]},
  "config.upload": {"description": "Upload a configuration.", "roles": ["FULL_ADMIN"], "filterable": false, "job": true, "uploadable": true, "accepts": [],
    "returns": [{"title": "result", "type": "null", "_name_": "result", "_required_": true}]},
  "auth.login_ex": {"description": "Authenticate.", "roles": [], "filterable": false, "job": false, "item_method": false, "no_auth_required": true,
    "accepts": [{"title": "login_data", "discriminator": {"propertyName": "mechanism"}, "oneOf": [
      {"title": "AuthApiKeyPlain", "type": "object", "properties": {"mechanism": {"title": "mechanism", "type": "string", "const": "API_KEY_PLAIN", "enum": ["API_KEY_PLAIN"], "_name_": "mechanism", "_required_": true}, "api_key": {"title": "api_key", "type": "string", "_name_": "api_key", "_required_": true}}},
      {"title": "AuthPasswordPlain", "type": "object", "properties": {"mechanism": {"title": "mechanism", "type": "string", "const": "PASSWORD_PLAIN", "enum": ["PASSWORD_PLAIN"], "_name_": "mechanism", "_required_": true}, "password": {"title": "password", "type": "string", "_name_": "password", "_required_": true}, "remember": {"title": "remember", "type": "boolean", "_name_": "remember", "_required_": false}}}],
      "_name_": "login_data", "_required_": true}],
    "returns": [{"title": "result", "type": "object", "properties": {"response_type": {"title": "response_type", "type": "string", "enum": ["SUCCESS", "AUTH_ERR"], "_name_": "response_type", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "pool.query": {"description": "Query pools.", "roles": ["POOL_READ", "READONLY_ADMIN"], "filterable": true, "job": false,
    "accepts": [
      {"title": "filters", "type": "array", "items": [{}], "_name_": "filters", "_required_": false},
      {"title": "options", "type": "object", "properties": {"get": {"title": "get", "type": "boolean", "_name_": "get", "_required_": false}, "count": {"title": "count", "type": "boolean", "_name_": "count", "_required_": false}, "limit": {"title": "limit", "type": "integer", "_name_": "limit", "_required_": false}, "force_sql_filters": {"title": "force_sql_filters", "type": "boolean", "_name_": "force_sql_filters", "_required_": false}}, "_name_": "options", "_required_": false}],
    "returns": [{"title": "result", "anyOf": [{"type": "array", "items": [{"title": "PoolEntry", "type": "object", "properties": {"id": {"title": "id", "type": "integer", "_name_": "id", "_required_": true}, "status": {"title": "status", "type": "string", "enum": ["ONLINE", ""], "_name_": "status", "_required_": true}}}]}, {"type": "integer"}], "_name_": "result", "_required_": true}]},
  "pool.create": {"description": "Create a pool.", "roles": ["POOL_WRITE"], "filterable": false, "job": true,
    "accepts": [{"title": "pool_create", "type": "object", "properties": {"name": {"title": "name", "type": "string", "_name_": "name", "_required_": true}}, "_name_": "pool_create", "_required_": true}, {"title": "wait", "type": "boolean", "_name_": "wait", "_required_": false}],
    "returns": [{"title": "PoolEntry", "type": "object", "properties": {"id": {"title": "id", "type": "integer", "_name_": "id", "_required_": true}, "status": {"title": "status", "type": "string", "enum": ["ONLINE", ""], "_name_": "status", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "pool.scrub.run": {"description": "Scrub a pool.", "roles": ["POOL_WRITE"], "filterable": false, "job": false,
    "accepts": [{"title": "name", "type": "string", "_name_": "name", "_required_": true}, {"title": "threshold", "type": "integer", "_name_": "threshold", "_required_": false}],
    "returns": [{"title": "result", "type": "null", "_name_": "result", "_required_": true}]},
  "system.info": {"description": "System information.", "roles": ["READONLY_ADMIN"], "filterable": false, "job": false, "accepts": [],
    "returns": [{"title": "result", "type": "object", "properties": {"hostname": {"title": "hostname", "type": "string", "_name_": "hostname", "_required_": true}, "boottime": {"title": "boottime", "type": "string", "format": "date-time", "_name_": "boottime", "_required_": true}, "state": {"title": "state", "type": "string", "enum": ["READY", "BOOTING"], "_name_": "state", "_required_": true}}, "_name_": "result", "_required_": true}]},
  "core.bulk": {"description": "Run a method many times.", "roles": ["READONLY_ADMIN"], "filterable": false, "job": true,
    "accepts": [{"title": "method", "type": "string", "_name_": "method", "_required_": true}, {"title": "params", "type": "array", "items": [{"type": "array", "items": [{}]}], "_name_": "params", "_required_": true}],
    "returns": [{"title": "result", "type": "array", "items": [{}], "_name_": "result", "_required_": true}]}
}`

func callDefinitions(t *testing.T, patches ...workarounds.Workaround) *definitions.Service {
	t.Helper()

	spec, _, err := jsonrpc.Parse([]byte(callMethods), "TrueNAS", "25.10.7")
	if err != nil {
		t.Fatal(err)
	}
	applied := make([]string, 0, len(patches))
	for _, w := range patches {
		if err := w.Apply(spec); err != nil {
			t.Fatal(err)
		}
		applied = append(applied, w.Name())
	}
	svc, err := importer.FromSpec(config.Service{Name: "truenas", Package: "truenas", Naming: config.OperationIDNaming, ReadRole: "READONLY_ADMIN", TypedUnions: true, Dates: config.Dates{Send: "$date", Read: []string{"2006-01-02T15:04:05Z07:00"}}}, spec, applied, nil)
	if err != nil {
		t.Fatal(err)
	}

	return svc
}

// A service of JSON-RPC methods generates a method per call that hands the method's name and its arguments, in order and
// trimmed of those left off the end, to the Caller New takes, with a test per method recording the call; flat and grouped.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestCalls(t *testing.T) {
	svc := callDefinitions(t)
	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	for file, wants := range map[string][]string{
		"client.go":                          {"type Client struct {\n\tCaller client.Caller\n}", "func New(caller client.Caller) (*Client, error) {"},
		"pool_method_poolquery.go":           {"func (c Client) PoolQuery(ctx context.Context, filters []any, options PoolQueryOptions) (result PoolQueryResponse, err error) {", "params := client.CallParams(0, filters, options)", `err = c.Caller.Call(ctx, "pool.query", params, &result.Answer)`, "// It changes nothing on the server."},
		"pool_method_poolcreate.go":          {"func (c Client) PoolCreate(ctx context.Context, poolCreate PoolCreate, wait bool) (result PoolCreateResponse, err error) {", "params := client.CallParams(1, poolCreate, wait)", "var answer PoolEntry", `if err := c.Caller.Call(ctx, "pool.create", params, &answer); err != nil {`, "result.PoolEntry = result.Answer", "// The server runs it as a job"},
		"poolscrub_method_poolscrubrun.go":   {"func (c Client) PoolScrubRun(ctx context.Context, name string, threshold int) (result PoolScrubRunResponse, err error) {", `err = c.Caller.Call(ctx, "pool.scrub.run", params, nil)`},
		"core_method_corebulk.go":            {"func (c Client) CoreBulk(ctx context.Context, method string, paramsParam [][]any) (result CoreBulkResponse, err error) {"},
		"system_model_systeminfoanswer.go":   {"Boottime Time ", "SystemInfoAnswerState `json:\"state,omitempty\"`"},
		"pool_constants.go":                  {"PoolEntryStatusEmpty "},
		"pool_model_poolqueryoptions.go":     {"ForceSqlFilters *bool `json:\"force_sql_filters,omitempty\"`"}, // a flag in a record a call sends is left out unless set, as in a request body
		"pool_method_poolqueryone.go":        {"// PoolQueryOne calls pool.query for one record", "func (c Client) PoolQueryOne(ctx context.Context, filters []any, options PoolQueryOptions) (result PoolQueryOneResponse, err error) {", `params := client.CallParams(2, filters, client.Flagged{Record: options, Flag: "get"})`, "var answer PoolEntry"},
		"pool_method_poolquerycount.go":      {"func (c Client) PoolQueryCount(ctx context.Context, filters []any, options PoolQueryOptions) (result PoolQueryCountResponse, err error) {", `client.Flagged{Record: options, Flag: "count"}`, `c.Caller.Call(ctx, "pool.query", params, &answer)`},
		"pool_method_poolqueryone_test.go":   {`client.Flagged{Record: PoolQueryOptions{}, Flag: "get"}}; !reflect.DeepEqual(r.params, want) {`},
		"auth_model_authloginexlogindata.go": {"type AuthLoginExLoginData struct {\n\tclient.Union\n}", `func (u AuthLoginExLoginData) Kind() string { return u.Union.Kind("mechanism") }`, `AsAuthApiKeyPlain() (AuthApiKeyPlain, error)`, `client.UnionAs[AuthPasswordPlain](u.Union, "mechanism", "PASSWORD_PLAIN")`, `AuthApiKeyPlain (mechanism "API_KEY_PLAIN")`},
		"auth_method_authloginex.go":         {"func (c Client) AuthLoginEx(ctx context.Context, loginData AuthLoginExLoginData) (result AuthLoginExResponse, err error) {"},
		"config_method_configsave.go":        {"// Its answer is a file: ConfigSaveTo writes it out.", "func (c Client) ConfigSaveTo(ctx context.Context, w io.Writer, options ConfigSaveOptions) (int64, error) {", `return client.DownloadWith(ctx, c.Caller, "config.save", params, w)`},
		"config_method_configupload.go":      {"func (c Client) ConfigUpload(ctx context.Context, files ...io.Reader) (result ConfigUploadResponse, err error) {", "return client.UploadWith(ctx, c.Caller, method, params, into, files...)", `err = send(ctx, "config.upload", params, nil)`},
		"config_method_configupload_test.go": {`r.uploaded != "file bytes"`},
		"config_method_configsave_test.go":   {`c.ConfigSaveTo(t.Context(), &file, ConfigSaveOptions{})`},
		"pool_method_poolcreate_test.go":     {`if want := []any{PoolCreate{}, true}; !reflect.DeepEqual(r.params, want) {`, "// an argument the server does not insist on is left off the end when it is not set", `if want := []any{PoolCreate{}}; !reflect.DeepEqual(r.params, want) {`},
		"doc.go":                             {"Every call goes through the client.Caller"},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}
	if src := read(t, dir, "pool_method_poolquery.go"); strings.Contains(src, "HttpResponse") || strings.Contains(src, "ExpectedStatusCodes") {
		t.Errorf("a call's method speaks of HTTP:\n%s", src)
	}
	// a member of a union that is sent is a sent record, its flags left out unless set
	members, _ := filepath.Glob(filepath.Join(dir, "*_model_authpasswordplain.go"))
	if len(members) != 1 {
		t.Fatalf("the union member's model = %v", members)
	}
	if src := read(t, dir, filepath.Base(members[0])); !regexp.MustCompile("Remember +\\*bool +`json:\"remember,omitempty\"`").MatchString(src) {
		t.Errorf("the union member's flag is not a pointer:\n%s", src)
	}

	// a call service takes a connection, so a credential or an authorizer says nothing
	bad := *svc
	bad.Auth = "Bearer"
	if _, err := generator.Render(&bad, generator.Options{ClientImport: testClient}); err == nil || !strings.Contains(err.Error(), "a service of calls takes a connection") {
		t.Errorf("Auth on a call service = %v", err)
	}

	// the check finds every call under its full name in one package
	if n, err := generator.Check(svc, dir); err != nil || n != 10 {
		t.Errorf("Check of the flat layout = %d, %v", n, err)
	}

	dir = generateInModule(t, svc, generator.Options{Grouped: true, ImportPath: "github.com/katbyte/pandorest/generator/testdata/mini"})
	goTool(t, dir+"/...", "vet")
	goTool(t, dir+"/...", "test")
	// and under its short name on its own group's client in a package per namespace, where Query alone is every group's
	if n, err := generator.Check(svc, dir); err != nil || n != 10 {
		t.Errorf("Check of the grouped layout = %d, %v", n, err)
	}
	if err := os.Remove(filepath.Join(dir, "pool", "method_poolquery.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := generator.Check(svc, dir); err == nil || !strings.Contains(err.Error(), "1 methods missing") || !strings.Contains(err.Error(), "Query (CALL pool.query)") {
		t.Errorf("Check without one group's method = %v", err)
	}
	dir = generateInModule(t, svc, generator.Options{Grouped: true, ImportPath: "github.com/katbyte/pandorest/generator/testdata/mini"})
	// in a package per namespace the method drops the namespace, and its types keep it
	for file, wants := range map[string][]string{
		"client/client.go":                  {"func New(caller base.Caller) (*Client, error) {", "pool.NewClient(caller),"},
		"pool/client.go":                    {"func NewClient(caller client.Caller) Client { return Client{Caller: caller} }"},
		"pool/method_poolquery.go":          {"// Query calls pool.query.", "func (c Client) Query(ctx context.Context, filters []any, options truenas.PoolQueryOptions) (result truenas.PoolQueryResponse, err error) {"},
		"pool/method_poolquery_test.go":     {"func TestOperationPoolQuery(t *testing.T) {", "c.Query(t.Context(), "},
		"poolscrub/method_poolscrubrun.go":  {"func (c Client) Run(ctx context.Context, name string, threshold int) (result truenas.PoolScrubRunResponse, err error) {"},
		"core/method_corebulk.go":           {"func (c Client) Bulk(ctx context.Context, method string, paramsParam [][]any) (result truenas.CoreBulkResponse, err error) {"},
		"truenas/operation_poolscrubrun.go": {"type PoolScrubRunResponse struct{}\n"},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}

	// a tool calling the short name through the group's field names the method; the short name alone does not, being every namespace's
	tools := filepath.Join(dir, "tools")
	if err := os.MkdirAll(tools, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, "tools.go"), []byte("package tools\n\nfunc use(c *client.Client, s pool.Client) {\n\tc.Pool.Query(nil, nil, truenas.PoolQueryOptions{})\n\tc.Core.Bulk(nil, \"\", nil)\n\ts.Create(nil, nil, false)\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uncalled, total, err := generator.Uncalled(svc, tools)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"AuthLoginEx", "ConfigSave", "ConfigUpload", "PoolCreate", "PoolQueryCount", "PoolQueryOne", "PoolScrubRun", "SystemInfo"}; total != 10 || !reflect.DeepEqual(uncalled, want) {
		t.Errorf("Uncalled = %v of %d, want %v of 10", uncalled, total, want)
	}

	// counted through one field, only what that field's client reaches names a method: with two services in one repository, the
	// other's call of the same name does not count, and nor does a call through no field
	if err := os.WriteFile(filepath.Join(tools, "two.go"), []byte("package tools\n\ntype servers struct {\n\tnas, other *client.Client\n}\n\nfunc (s servers) use() {\n\ts.nas.Pool.Query(nil, nil, truenas.PoolQueryOptions{})\n\ts.other.Core.Bulk(nil, \"\", nil)\n\ts.nas.System.Info(nil)\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uncalled, total, err = generator.UncalledOn(svc, tools, "nas")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"AuthLoginEx", "ConfigSave", "ConfigUpload", "CoreBulk", "PoolCreate", "PoolQueryCount", "PoolQueryOne", "PoolScrubRun"}; total != 10 || !reflect.DeepEqual(uncalled, want) {
		t.Errorf("UncalledOn nas = %v of %d, want %v of 10", uncalled, total, want)
	}
}

// A field a workaround names a Go type for is held as that type, imported from where the workaround says, and the package
// built on it still builds and passes its tests; the definitions carry the type so a diff shows it.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestNamedTypes(t *testing.T) {
	secrets := workarounds.NamedTypes(workarounds.About{Name: "truenas-secrets", Service: "truenas", Bug: "a read-only caller is sent ******** in place of a secret, whatever its type"}, testClient, map[string]string{
		"SystemInfoAnswer.hostname": "client.Secret[string]",
		"SystemInfoAnswer.state":    "client.Secret[$item]", // the field's own enum, by whatever name the package gives it
		"PoolEntry.status":          "client.Secret[string]",
	})
	svc := callDefinitions(t, secrets)
	m := svc.Models()["SystemInfoAnswer"]
	if m == nil {
		t.Fatal("no SystemInfoAnswer model")
	}
	hostname := slices.IndexFunc(m.Fields, func(f definitions.Field) bool { return f.JSONName == "hostname" })
	if hostname < 0 || m.Fields[hostname].Type.Type != definitions.Named || m.Fields[hostname].Type.GoType != "client.Secret[string]" || m.Fields[hostname].Type.Import != testClient || m.Fields[hostname].Type.String() != "Named(client.Secret[string])" {
		t.Errorf("hostname = %+v", m.Fields[hostname].Type)
	}
	if !slices.Contains(svc.Workarounds, "truenas-secrets") {
		t.Errorf("the workaround is not recorded: %v", svc.Workarounds)
	}

	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")
	for file, wants := range map[string][]string{
		"system_model_systeminfoanswer.go": {"Hostname client.Secret[string]", "`json:\"hostname,omitzero\"`", "client.Secret[SystemInfoAnswerState] `json:\"state,omitzero\"`", strconv.Quote(testClient)},
		"pool_model_poolentry.go":          {"Status client.Secret[string] `json:\"status"},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}

	// a type from another package is imported under the qualifier it is written with
	svc = callDefinitions(t, workarounds.NamedTypes(workarounds.About{Name: "truenas-sets", Service: "truenas", Bug: "a set is written two ways"}, "example.com/nas/rpc", map[string]string{"SystemInfoAnswer.hostname": "nasrpc.Set[string]"}))
	files, err := generator.Render(svc, generator.Options{ClientImport: testClient})
	if err != nil {
		t.Fatal(err)
	}
	if src := string(files["system_model_systeminfoanswer.go"]); !strings.Contains(src, "Hostname nasrpc.Set[string]") || !strings.Contains(src, `nasrpc "example.com/nas/rpc"`) {
		t.Errorf("the type is not imported under its qualifier:\n%s", src)
	}
}

// Every package has a table of its operations a program can go by: a request with its method and path, read for a GET, and
// whether it answers or takes a file.
func TestOperationsTable(t *testing.T) {
	t.Parallel()

	files, err := generator.Render(miniDefinitions(t), generator.Options{ClientImport: testClient})
	if err != nil {
		t.Fatal(err)
	}
	src := string(files["operations.go"])
	for _, want := range []string{"type OperationInfo struct {", "var Operations = []OperationInfo{", `{Name: "GetItems", Method: "GET", Path: "/Items", Group: "Items", Read: true},`} {
		if !strings.Contains(src, want) {
			t.Errorf("operations.go lacks %q:\n%s", want, src)
		}
	}
	if !regexp.MustCompile(`Method: "(POST|PUT|DELETE)", Path: "[^"]+", Group: "[A-Za-z]+"},`).MatchString(src) {
		t.Errorf("no write in the table:\n%s", src)
	}
}

// A credential named by a phrase is an argument called credential, and the
// phrase words the refusal.
func TestCredentialPhrase(t *testing.T) {
	t.Parallel()

	files, err := generator.Render(miniDefinitions(t), generator.Options{ClientImport: testClient, Credential: "TMDB read access token or API key"}) //nolint:gosec // the name of a credential, not one
	if err != nil {
		t.Fatal(err)
	}
	client := string(files["client.go"])
	for _, want := range []string{"func New(baseURL, credential string, opts ...client.Option) (*Client, error) {", `"TMDB read access token or API key is required"`} {
		if !strings.Contains(client, want) {
			t.Errorf("client.go has no %q:\n%s", want, client)
		}
	}
}

// serviceSource is the file a repository writes by hand beside its generated
// package: what is the service's own about being talked to.
const serviceSource = `package mini

import "github.com/katbyte/pandorest/client"

// service is what is Arr's own about being talked to.
var service = client.Service{Name: "Arr", UserAgent: "mini-test", Example: "nas:8989"}
`

// A service that pages by page number, expects a failure that answers in
// full, holds the document to its required properties and writes its
// settings whole gets a package that does each, builds, and passes its own
// tests: the walk by page among them.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestGenerateServiceOptions(t *testing.T) {
	spec, err := openapi.Parse([]byte(arrSpec))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := importer.FromSpec(config.Service{
		Name: "mini", Package: "mini", Naming: config.PathNaming, Auth: "APIKey", PathPrefix: "/api/v3", Words: map[string]string{"testall": "TestAll"},
		Paging:     &config.Paging{Start: "page", Limit: "pageSize", Items: "records", Total: "totalRecords", ByPage: true},
		PreferJSON: true, ExpectSameShape: true, SendRequired: true, WrittenWhole: []string{"HostConfig"}, KeepNull: []string{"HostConfig.sslPort", "HostConfig.bindAddress"},
	}, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// the service says what is its own in a file it writes by hand beside the
	// generated ones, which the generated New makes its client from
	dir := generateInModule(t, svc, generator.Options{Version: "5.0.1.9", ClientService: "service"})
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(serviceSource), 0o600); err != nil {
		t.Fatal(err)
	}

	for file, wants := range map[string][]string{
		"history_method_getseriesbyseriesidhistory.go": {
			"func (c Client) GetSeriesBySeriesIdHistoryComplete(ctx context.Context, seriesId int, options ...GetSeriesBySeriesIdHistoryOptions) (result GetSeriesBySeriesIdHistoryCompleteResult, err error) {",
			"// GetSeriesBySeriesIdHistoryComplete calls GetSeriesBySeriesIdHistory page by page, from the\n// page options.Page names (the first when unset), until every result is loaded.\n// options.PageSize is the page size, client.DefaultPageSize when unset.",
			"\tnumber := 1\n\tif option.Page != nil && *option.Page > 0 {\n\t\tnumber = *option.Page\n\t}\n\toption.Page = &number\n",
			"if len(page.Answer.Records) < limit || page.Answer.TotalRecords > 0 && number*limit >= page.Answer.TotalRecords {",
			"\t\tnumber++\n",
		},
		"history_method_getseriesbyseriesidhistory_test.go": {
			`c, starts := pagedServer(t, "page", "records", "totalRecords", "{}", tc.total, tc.pages)`,
			`{"total reached", 2, 9, GetSeriesBySeriesIdHistoryOptions{PageSize: new(1)}, 2, []string{"1", "2"}},`,
			`{"empty page", 0, 2, GetSeriesBySeriesIdHistoryOptions{PageSize: new(1)}, 2, []string{"1", "2", "3"}},`,
			`{"short page", 0, 9, GetSeriesBySeriesIdHistoryOptions{PageSize: new(2)}, 1, []string{"1"}},`,
		},
		"client_test.go": {`fmt.Fprintf(w, "{%q:[%s],%q:%d}", items, item, total, count)`, "reporting total items under the names the list answers\n// with, and records the page of each request.", "\t\tif status >= http.StatusMultipleChoices && status < http.StatusBadRequest {\n\t\t\tw.Header().Set(\"Location\", \"/redirected\")\n\t\t}\n"},
		// an operation whose documented answer is a redirect expects it, and its test proves the client hands it back unfollowed
		"authentication_method_postlogin.go":      {"\t\t\thttp.StatusFound,\n"},
		"authentication_method_getlogout_test.go": {"c, s := newOperationServer(t, 302, \"\", \"\")", "if result.HttpResponse == nil || result.HttpResponse.StatusCode != http.StatusFound {"},
		"indexer_method_postindexertestall.go":    {"\t\t\thttp.StatusOK,\n\t\t\thttp.StatusBadRequest,\n"},
		"client.go":                               {`const APIVersion = "3.0.0"`, `const DocumentVersion = "5.0.1.9"`, "c, err := service.New(baseURL, client.APIKey(token), append(slices.Clip(opts), client.WithSecrets(token))...)"},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}
	// a required field is always sent, and so are a settings object's strings
	// and numbers; its flags, its lists and a number whose null is kept are not
	for file, tags := range map[string][]string{
		"common_model_record.go": {"`json:\"id,omitempty\"`", "`json:\"title\"`", `Monitored +bool +` + "`json:\"monitored\"`"},
		// a string whose null is kept is a pointer left out when nil, even in a model written whole
		"hostconfig_model_hostconfig.go": {`BindAddress +\*string +` + "`json:\"bindAddress,omitempty\"`", "`json:\"port\"`", `SslPort +\*int +` + "`json:\"sslPort,omitempty\"`", "`json:\"enableSsl\"`", "`json:\"tags,omitzero\"`"},
	} {
		src := read(t, dir, file)
		for _, tag := range tags {
			if !regexp.MustCompile(tag).MatchString(src) {
				t.Errorf("%s lacks %s:\n%s", file, tag, src)
			}
		}
	}

	goTool(t, dir, "vet")
	goTool(t, dir, "test", "-count=1")
}

// A command set gives a method per command that fixes the command's name, an AndWait that queues and polls, and WaitForCommand for
// one queued the plain way; the generated tests prove each on the wire.
//
//nolint:paralleltest // writes the testdata package the other tests would remove
func TestCommands(t *testing.T) {
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
    "/api/v3/command": {"post": {"operationId": "PostCommand", "tags": ["Command"], "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/CommandResource"}}}},
      "responses": {"201": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/CommandResource"}}}}}}},
    "/api/v3/command/{id}": {"get": {"operationId": "GetCommandById", "tags": ["Command"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/CommandResource"}}}}}}}},
  "components": {"schemas": {"CommandResource": {"type": "object", "properties": {"id": {"type": "integer"}, "name": {"type": "string"}, "status": {"$ref": "#/components/schemas/CommandStatus"}, "message": {"type": "string"}}},
    "CommandStatus": {"type": "string", "enum": ["queued", "started", "completed", "failed", "aborted"]}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	w := workarounds.Commands(workarounds.About{Name: "mini-commands", Service: "mini", Bug: "the command body is an open record"}, workarounds.CommandSet{
		Post: "POST /api/v3/command", Get: "GET /api/v3/command/{id}", Status: "status", Finished: []string{"completed", "failed", "aborted"}, Failed: []string{"failed", "aborted"},
		Commands: map[string]map[string]*openapi.Schema{"RefreshSeries": {"seriesId": {Type: "integer"}}, "RssSync": {}},
	})
	if err := w.Apply(spec); err != nil {
		t.Fatal(err)
	}
	cfg := config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, Auth: "APIKey"}
	svc, err := importer.FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := generateInModule(t, svc, generator.Options{})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")

	for file, wants := range map[string][]string{
		"command_method_refreshseries.go": {"func (c Client) RefreshSeries(ctx context.Context, input RefreshSeriesCommand) (result RefreshSeriesResponse, err error) {", "\tinput.Name = \"RefreshSeries\"\n", "func (c Client) RefreshSeriesAndWait(ctx context.Context, input RefreshSeriesCommand, wait time.Duration) (result GetCommandByIdResponse, err error) {", "return c.WaitForCommand(ctx, queued.Answer.Id, wait)"},
		"command_method_postcommand.go":   {"func (c Client) WaitForCommand(ctx context.Context, id int, wait time.Duration) (result GetCommandByIdResponse, err error) {", `if slices.Contains([]string{"failed", "aborted"}, status) {`, `if slices.Contains([]string{"completed", "failed", "aborted"}, status) {`, "id, result.Answer.Name, status, result.Answer.Message, client.ErrCommandFailed)", "client.ErrCommandRunning)"},
		"command_model_rsssynccommand.go": {"type RssSyncCommand struct {", "Name string `json:\"name,omitempty\"`"},
	} {
		src := read(t, dir, file)
		for _, want := range wants {
			if !strings.Contains(src, want) {
				t.Errorf("%s lacks %q:\n%s", file, want, src)
			}
		}
	}
	if _, err := generator.Check(svc, dir); err != nil {
		t.Errorf("Check = %v", err)
	}

	// the grouped layout has the same methods on the tag's client
	dir = generateInModule(t, svc, generator.Options{Grouped: true, ImportPath: "github.com/katbyte/pandorest/generator/testdata/mini"})
	goTool(t, dir, "vet")
	goTool(t, dir, "test")
	if src := read(t, dir, "command/method_refreshseries.go"); !strings.Contains(src, "func (c Client) RefreshSeriesAndWait(ctx context.Context, input mini.RefreshSeriesCommand, wait time.Duration) (result mini.GetCommandByIdResponse, err error) {") {
		t.Errorf("the grouped command method lacks AndWait:\n%s", src)
	}
}
