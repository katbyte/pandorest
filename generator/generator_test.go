package generator_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/config"
	"github.com/katbyte/pandorest/definitions"
	"github.com/katbyte/pandorest/generator"
	"github.com/katbyte/pandorest/importer"
	"github.com/katbyte/pandorest/importer/workarounds"
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
      "Id": {"type": "string"}, "IsFolder": {"type": "boolean"}, "Parent": {"$ref": "#/components/schemas/Item"},
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

	opts.Dir = filepath.Join("testdata", "mini")
	opts.Definitions = "api-defs/mini"
	opts.ClientImport = testClient
	if err := os.RemoveAll(opts.Dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll("testdata") })
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
	for _, want := range []string{"\t// What to ask for.\n\tAccept string\n", `out.Append("Accept", o.Accept)`, "func (c Client) GetItem(ctx context.Context, itemId string, options GetItemOperationOptions) ("} {
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
	for _, want := range []string{"// The operation answers text (text/plain), which Model holds as it was sent.", "\tModel        *string\n", `Accept: "text/plain",`, "if err = resp.Text(result.Model); err != nil {"} {
		if !strings.Contains(method, want) {
			t.Errorf("GetName has no %q:\n%s", want, method)
		}
	}
	if strings.Contains(method, "Unmarshal") || strings.Contains(method, "StreamResponse") {
		t.Errorf("GetName decodes or streams its text:\n%s", method)
	}

	test := read(t, dir, "sync_method_getname_test.go")
	if !strings.Contains(test, `newOperationServer(t, 200, "application/json", "some text, as sent\n")`) || !strings.Contains(test, `*result.Model != "some text, as sent\n"`) || strings.Contains(test, "does not decode") {
		t.Errorf("GetName's test:\n%s", test)
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
			"func (c Client) GetItems(ctx context.Context, options GetItemsOperationOptions) (result GetItemsOperationResponse, err error) {",
			"func (c Client) GetItemsComplete(ctx context.Context, options GetItemsOperationOptions) (result GetItemsCompleteResult, err error) {",
			"func (c Client) GetItem(ctx context.Context, itemId string) (result GetItemOperationResponse, err error) {",
			"func (c Client) UpdateItem(ctx context.Context, itemId string, input Item) (result UpdateItemOperationResponse, err error) {",
			"func (c Client) GetItemImage(ctx context.Context, itemId string, imageType Kind, index int) (result GetItemImageOperationResponse, err error) {",
			"func (c Client) SetItemImage(ctx context.Context, itemId string, imageType Kind, index int, input io.Reader, contentType string) (result SetItemImageOperationResponse, err error) {",
			"func (c Client) PutSync(ctx context.Context, jobId int64, ratio float64, input []Item) (result PutSyncOperationResponse, err error) {",
			`fmt.Sprintf("/Items/%s/Images/%s/%d", url.PathEscape(itemId), url.PathEscape(string(imageType)), index),`,
			`fmt.Sprintf("/Sync/%d/%s", jobId, strconv.FormatFloat(ratio, 'f', -1, 64))`,
			"StreamResponse: true,",
			`Accept: "image/*",`,
			"http.StatusNoContent,",
			"// Deprecated: the document marks this operation deprecated.",
			"// Deprecated: the document marks this parameter deprecated.",
			// response models: pointer for a struct, primitive or constant; value for the rest
			"Model        *QueryResult\n", "Model        []string\n", "Model        json.RawMessage\n",
			"Model        *string\n", "Model        *Kind\n", "Model        Union\n",
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
			"func New(baseURL, token string, opts ...client.Option) (*Client, error) {", "c, err := client.New(baseURL, client.Bearer(token), opts...)", "// Client is a client for Mini; each of its operations is a method.",
			`const APIVersion = "1"`,
			// the package documentation: the base client, how the lists page, the service's notes, the workarounds
			"// under common_. The requests go through the hand-written base client,\n// " + testClient + ".\n",
			"// A list operation, one whose options page with StartIndex and Limit, also has a\n// <Name>Complete method",
			"//\n// Dates are strings.\n//\n// Ids are too.\n//\n// # Workarounds\n",
			"//   - mini-fix",
			// the paged server answers in the shape the service's lists have
			`fmt.Fprintf(w, "{\"Items\":[%s],\"TotalRecordCount\":%d}", item, total)`,
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
		generator.Header + "\npackage x\n": true,
		"// Package emby is a typed client for Emby Server REST API 4.10.0.40, generated by pandorest\n// from the definitions.\npackage emby\n": true,
		"// Package emby is a hand-written client.\npackage emby\n":                                                                              false,
		"// Code generated by sdk/pandorest; DO NOT EDIT.\n\npackage x\n":                                                                        true,
		"// Code generated by internal/pandorest; DO NOT EDIT.\n\npackage x\n":                                                                   true,
		"// Code generated by mockgen; DO NOT EDIT.\n\npackage x\n":                                                                              false,
		"// Code generated by notpandorest; DO NOT EDIT.\n\npackage x\n":                                                                         false,
		"package x\n\n// Code generated by pandorest; DO NOT EDIT.\n":                                                                            false,
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
		"c, err := client.New(baseURL, client.Bearer(apiKey), opts...)",
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
		PreferJSON: true, ExpectSameShape: true, SendRequired: true, WrittenWhole: []string{"HostConfig"}, KeepNull: []string{"HostConfig.sslPort"},
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
			"func (c Client) GetSeriesBySeriesIdHistoryComplete(ctx context.Context, seriesId int, options GetSeriesBySeriesIdHistoryOperationOptions) (result GetSeriesBySeriesIdHistoryCompleteResult, err error) {",
			"// GetSeriesBySeriesIdHistoryComplete calls GetSeriesBySeriesIdHistory page by page, from the\n// page options.Page names (the first when unset), until every result is loaded.\n// options.PageSize is the page size, client.DefaultPageSize when unset.",
			"\tnumber := 1\n\tif options.Page != nil && *options.Page > 0 {\n\t\tnumber = *options.Page\n\t}\n\toptions.Page = &number\n",
			"if len(page.Model.Records) < limit || page.Model.TotalRecords > 0 && number*limit >= page.Model.TotalRecords {",
			"\t\tnumber++\n",
		},
		"history_method_getseriesbyseriesidhistory_test.go": {
			`c, starts := pagedServer(t, "page", "{}", tc.total, tc.pages)`,
			`{"total reached", 2, 9, GetSeriesBySeriesIdHistoryOperationOptions{PageSize: new(1)}, 2, []string{"1", "2"}},`,
			`{"empty page", 0, 2, GetSeriesBySeriesIdHistoryOperationOptions{PageSize: new(1)}, 2, []string{"1", "2", "3"}},`,
			`{"short page", 0, 9, GetSeriesBySeriesIdHistoryOperationOptions{PageSize: new(2)}, 1, []string{"1"}},`,
		},
		"client_test.go": {`fmt.Fprintf(w, "{\"records\":[%s],\"totalRecords\":%d}", item, total)`, "reporting total items, and records the page of\n// each request.", "\t\tif status >= http.StatusMultipleChoices && status < http.StatusBadRequest {\n\t\t\tw.Header().Set(\"Location\", \"/redirected\")\n\t\t}\n"},
		// an operation whose documented answer is a redirect expects it, and its test proves the client hands it back unfollowed
		"authentication_method_postlogin.go":      {"\t\t\thttp.StatusFound,\n"},
		"authentication_method_getlogout_test.go": {"c, s := newOperationServer(t, 302, \"\", \"\")", "if result.HttpResponse == nil || result.HttpResponse.StatusCode != 302 {"},
		"indexer_method_postindexertestall.go":    {"\t\t\thttp.StatusOK,\n\t\t\thttp.StatusBadRequest,\n"},
		"client.go":                               {`const APIVersion = "3.0.0"`, `const DocumentVersion = "5.0.1.9"`, "c, err := service.New(baseURL, client.APIKey(token), opts...)"},
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
		"common_model_record.go":         {"`json:\"id,omitempty\"`", "`json:\"title\"`", `Monitored +\*bool +` + "`json:\"monitored\"`"},
		"hostconfig_model_hostconfig.go": {"`json:\"bindAddress\"`", "`json:\"port\"`", `SslPort +\*int +` + "`json:\"sslPort,omitempty\"`", "`json:\"enableSsl,omitempty\"`", "`json:\"tags,omitzero\"`"},
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
