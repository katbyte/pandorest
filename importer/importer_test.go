package importer

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/config"
	"github.com/katbyte/pandorest/definitions"
	"github.com/katbyte/pandorest/importer/workarounds"
	"github.com/katbyte/pandorest/openapi"
)

// miniSpec exercises one of every shape the importer normalises.
const miniSpec = `{
  "openapi": "3.0.1",
  "info": {"title": "Mini", "version": "1"},
  "paths": {
    "/Items": {"get": {"operationId": "GetItems", "tags": ["ItemsService"], "summary": "Gets items.", "parameters": [
        {"name": "userId", "in": "query", "schema": {"type": "string", "format": "uuid"}},
        {"name": "startIndex", "in": "query", "schema": {"type": "integer", "format": "int32"}},
        {"name": "limit", "in": "query", "schema": {"type": "integer", "format": "int32"}},
        {"name": "ticks", "in": "query", "schema": {"type": "integer", "format": "int64"}},
        {"name": "rating", "in": "query", "schema": {"type": "number", "format": "double"}},
        {"name": "isFavorite", "in": "query", "schema": {"type": "boolean"}},
        {"name": "kind", "in": "query", "schema": {"allOf": [{"$ref": "#/components/schemas/Kind"}]}},
        {"name": "kinds", "in": "query", "schema": {"type": "array", "items": {"$ref": "#/components/schemas/Kind"}}},
        {"name": "ids", "in": "query", "style": "form", "explode": false, "schema": {"type": "array", "items": {"type": "string"}}},
        {"name": "old", "in": "query", "deprecated": true, "schema": {"type": "string"}},
        {"name": "X-Client", "in": "header", "schema": {"type": "string"}},
        {"name": "streamOptions", "in": "query", "schema": {"type": "object", "additionalProperties": {"type": "string"}}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Query_Result.Item"}}}}}}},
    "/Items/{itemId}": {
      "get": {"operationId": "GetItem", "tags": ["ItemsService"], "parameters": [{"name": "itemId", "in": "path", "schema": {"type": "string"}}],
        "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}, "application/xml": {}}}}},
      "post": {"operationId": "UpdateItem", "tags": ["ItemsService"], "parameters": [{"name": "itemId", "in": "path", "schema": {"type": "string"}}],
        "requestBody": {"content": {"application/*+json": {"schema": {"$ref": "#/components/schemas/Item"}}, "application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}},
        "responses": {"204": {"description": "ok"}}}},
    "/Items/{itemId}/Images/{imageType}/{index}": {
      "get": {"operationId": "GetItemImage", "tags": ["ImageService"], "parameters": [
          {"name": "itemId", "in": "path", "schema": {"type": "string"}},
          {"name": "imageType", "in": "path", "schema": {"allOf": [{"$ref": "#/components/schemas/Kind"}]}},
          {"name": "index", "in": "path", "schema": {"type": "integer"}}],
        "responses": {"200": {"content": {"image/*": {"schema": {"type": "string", "format": "binary"}}}}}},
      "post": {"operationId": "SetItemImage", "tags": ["ImageService"], "parameters": [
          {"name": "itemId", "in": "path", "schema": {"type": "string"}},
          {"name": "imageType", "in": "path", "schema": {"allOf": [{"$ref": "#/components/schemas/Kind"}]}},
          {"name": "index", "in": "path", "schema": {"type": "integer"}}],
        "requestBody": {"content": {"image/*": {"schema": {"type": "string", "format": "binary"}}}},
        "responses": {"200": {"description": "ok"}, "204": {"description": "ok"}}}},
    "/Config/{key}": {"get": {"operationId": "GetConfig", "tags": ["ConfigService"], "parameters": [{"name": "key", "in": "path", "schema": {"type": "string"}}],
        "responses": {"200": {"content": {"application/json": {"schema": {"type": "string", "format": "binary"}}}}}}},
    "/Union": {"post": {"operationId": "PostUnion", "tags": ["ConfigService"],
        "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Union"}}}},
        "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"type": "string"}}}}}}}}
  },
  "components": {"schemas": {
    "Kind": {"type": "string", "enum": ["Movie", "Series"]},
    "Union": {"type": "object", "oneOf": [{"$ref": "#/components/schemas/Item"}]},
    "Unused": {"type": "object", "properties": {"Name": {"type": "string"}}},
    "Item": {"type": "object", "properties": {
      "Id": {"type": "string"}, "IsFolder": {"type": "boolean", "nullable": true}, "Parent": {"$ref": "#/components/schemas/Item"},
      "Kind": {"allOf": [{"$ref": "#/components/schemas/Kind"}]}, "Children": {"type": "array", "items": {"$ref": "#/components/schemas/Item"}},
      "Blur": {"type": "object", "properties": {"Primary": {"type": "object", "additionalProperties": {"type": "string"}}}},
      "Rating": {"type": "number", "format": "float"}}},
    "Query_Result.Item": {"type": "object", "properties": {"Items": {"type": "array", "items": {"$ref": "#/components/schemas/Item"}}, "TotalRecordCount": {"type": "integer"}}}
  }}
}`

var miniConfig = config.Service{Name: "mini", Package: "mini", Naming: config.OperationIDNaming, TagSuffix: "Service", Auth: "Token", Paging: &config.Paging{Start: "StartIndex", Limit: "Limit", Items: "Items", Total: "TotalRecordCount"}}

func importMini(t *testing.T, patch func(*openapi.Spec)) (*definitions.Service, error) {
	t.Helper()

	spec, err := openapi.Parse([]byte(miniSpec))
	if err != nil {
		t.Fatal(err)
	}
	if patch != nil {
		patch(spec)
	}

	return FromSpec(miniConfig, spec, nil, func(msg string) { t.Log(msg) })
}

func find[T any](items []T, match func(T) bool) *T {
	i := slices.IndexFunc(items, match)
	if i < 0 {
		return nil
	}

	return &items[i]
}

func TestImport(t *testing.T) {
	t.Parallel()

	svc, err := importMini(t, nil)
	if err != nil {
		t.Fatal(err)
	}

	groups := make([]string, 0, len(svc.Groups))
	for _, g := range svc.Groups {
		groups = append(groups, g.Name)
	}
	if !slices.Equal(groups, []string{"Common", "Config", "Image", "Items"}) {
		t.Fatalf("groups = %v", groups)
	}

	op := func(name string) *definitions.Operation {
		t.Helper()
		o := find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == name })
		if o == nil {
			t.Fatalf("no operation %s", name)
		}
		return *o
	}

	t.Run("options", func(t *testing.T) {
		t.Parallel()

		o := op("GetItems")
		want := map[string]string{
			"userId": "String", "limit": "Integer", "ticks": "Integer64", "rating": "Double", "isFavorite": "Boolean",
			"kind": "Reference(Kind)", "kinds": "List[Reference(Kind)]", "ids": "List[String]", "streamOptions": "Dictionary[String]", "X-Client": "String",
		}
		for name, typ := range want {
			opt := find(o.Options, func(opt definitions.Option) bool { return opt.Name == name })
			if opt == nil || opt.Type.String() != typ {
				t.Errorf("option %s = %+v, want %s", name, opt, typ)
			}
		}
		if ids := find(o.Options, func(opt definitions.Option) bool { return opt.Name == "ids" }); !ids.CommaSeparated {
			t.Error("ids declares explode false but is not comma-separated")
		}
		if kinds := find(o.Options, func(opt definitions.Option) bool { return opt.Name == "kinds" }); kinds.CommaSeparated {
			t.Error("kinds takes OpenAPI's default (one key per value) but is comma-separated")
		}
		if old := find(o.Options, func(opt definitions.Option) bool { return opt.Name == "old" }); !old.Deprecated {
			t.Error("the deprecated option is not marked")
		}
		if h := find(o.Options, func(opt definitions.Option) bool { return opt.Name == "X-Client" }); h.In != definitions.InHeader || h.Field != "XClient" {
			t.Errorf("header option = %+v", h)
		}
		if o.Pageable == nil || o.Pageable.StartIndexOption != "StartIndex" || o.Pageable.LimitOption != "Limit" ||
			o.Pageable.ItemsField != "Items" || o.Pageable.TotalField != "TotalRecordCount" || o.Pageable.ItemType.String() != "Reference(Item)" {
			t.Errorf("Pageable = %+v", o.Pageable)
		}
		if o.Description != "Gets items." || !slices.Equal(o.ExpectedStatusCodes, []int{200}) {
			t.Errorf("GetItems = %+v", o)
		}
	})

	t.Run("bodies", func(t *testing.T) {
		t.Parallel()

		if o := op("GetItem"); o.Response.ContentType != "application/json" || o.Response.Type.String() != "Reference(Item)" || o.Pageable != nil {
			t.Errorf("GetItem response = %+v (xml twin ignored)", o.Response)
		}
		if o := op("UpdateItem"); o.Request.ContentType != "application/json" || o.Request.Type.String() != "Reference(Item)" || o.Response != nil ||
			!slices.Equal(o.ExpectedStatusCodes, []int{204}) {
			t.Errorf("UpdateItem = %+v", o)
		}
		img := op("GetItemImage")
		if img.Response.Type.Type != definitions.RawFile || img.Response.ContentType != "image/*" {
			t.Errorf("GetItemImage response = %+v", img.Response)
		}
		args := make([]string, 0, len(img.PathParameters))
		for _, p := range img.PathParameters {
			args = append(args, p.Argument+" "+p.Type.String())
		}
		if !slices.Equal(args, []string{"itemId String", "imageType Reference(Kind)", "index Integer"}) {
			t.Errorf("path parameters = %v", args)
		}
		if o := op("SetItemImage"); o.Request.Type.Type != definitions.RawFile || o.Request.ContentType != "image/*" || !slices.Equal(o.ExpectedStatusCodes, []int{200, 204}) {
			t.Errorf("SetItemImage = %+v", o)
		}
		// a binary string answered as JSON is a JSON document of no schema
		if o := op("GetConfig"); o.Response.Type.Type != definitions.RawObject {
			t.Errorf("GetConfig response = %+v", o.Response)
		}
		if o := op("PostUnion"); o.Request.Type.String() != "Reference(Union)" || o.Response.Type.String() != "List[String]" {
			t.Errorf("PostUnion = %+v", o)
		}
	})

	t.Run("models", func(t *testing.T) {
		t.Parallel()

		models := svc.Models()
		item := models["Item"]
		if item == nil {
			t.Fatal("no Item model")
		}
		fields := map[string]string{}
		for _, f := range item.Fields {
			fields[f.JSONName] = f.Type.String()
		}
		want := map[string]string{
			"Id": "String", "IsFolder": "Boolean", "Parent": "Reference(Item)", "Kind": "Reference(Kind)",
			"Children": "List[Reference(Item)]", "Blur": "Reference(ItemBlur)", "Rating": "Float",
		}
		for k, v := range want {
			if fields[k] != v {
				t.Errorf("Item.%s = %q, want %q", k, fields[k], v)
			}
		}
		if f := find(item.Fields, func(f definitions.Field) bool { return f.Name == "IsFolder" }); !f.Nullable {
			t.Error("IsFolder lost its nullable flag")
		}
		if blur := models["ItemBlur"]; blur == nil || blur.SchemaName != "" || blur.Fields[0].Type.String() != "Dictionary[String]" {
			t.Errorf("inline model = %+v", blur)
		}
		if u := models["Union"]; u == nil || !slices.Equal(u.Union, []string{"Item"}) {
			t.Errorf("union = %+v", u)
		}
		if qr := models["QueryResultItem"]; qr == nil || qr.SchemaName != "Query_Result.Item" {
			t.Errorf("QueryResultItem = %+v", qr)
		}
		if k := svc.Constants()["Kind"]; k == nil || len(k.Values) != 2 || k.Values[1] != (definitions.ConstantValue{Name: "KindSeries", Value: "Series"}) {
			t.Errorf("Kind = %+v", k)
		}
	})

	t.Run("ownership", func(t *testing.T) {
		t.Parallel()

		owner := map[string]string{}
		for _, g := range svc.Groups {
			for _, m := range g.Models {
				owner[m.Name] = g.Name
			}
			for _, c := range g.Constants {
				owner[c.Name] = g.Name
			}
		}
		// Kind is used by Items and Image; Item only by Items (a union names its
		// variants but does not use them)
		want := map[string]string{"QueryResultItem": "Items", "Item": "Items", "ItemBlur": "Items", "Kind": "Common", "Union": "Config", "Unused": "Common"}
		for name, g := range want {
			if owner[name] != g {
				t.Errorf("%s is in %s, want %s", name, owner[name], g)
			}
		}
	})
}

func TestPathNaming(t *testing.T) {
	t.Parallel()

	spec, err := openapi.Parse([]byte(miniSpec))
	if err != nil {
		t.Fatal(err)
	}
	cfg := miniConfig
	cfg.Naming = config.PathNaming
	svc, err := FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(svc.Operations()))
	for _, o := range svc.Operations() {
		names = append(names, o.Name)
	}
	slices.Sort(names)
	want := []string{"GetConfigByKey", "GetItems", "GetItemsByItemId", "GetItemsByItemIdImagesByImageTypeByIndex", "PostItemsByItemId", "PostItemsByItemIdImagesByImageTypeByIndex", "PostUnion"}
	if !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
}

// The importer refuses what it cannot normalise faithfully, listing every
// problem, so each gets a workaround rather than a silent guess.
func TestImportFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		patch func(*openapi.Spec)
		want  string
	}{
		{
			name:  "undeclared path parameter",
			patch: func(s *openapi.Spec) { s.Operation("GET", "/Items/{itemId}").Parameters = nil },
			want:  "GET /Items/{itemId}: path parameter {itemId} is not declared",
		},
		{
			name: "declared parameter the template lacks",
			patch: func(s *openapi.Spec) {
				op := s.Operation("GET", "/Items")
				op.Parameters = append(op.Parameters, &openapi.Parameter{Name: "id", In: openapi.InPath})
			},
			want: `GET /Items: declares path parameter "id" the template does not have`,
		},
		{
			name:  "GET with no response content",
			patch: func(s *openapi.Spec) { s.Operation("GET", "/Items/{itemId}").Responses["200"].Content = nil },
			want:  "GET /Items/{itemId}: declares no response content",
		},
		{
			// two ids that are not the same and make the same Go name
			name:  "two operations that make one method name",
			patch: func(s *openapi.Spec) { s.Operation("POST", "/Items/{itemId}").OperationID = "get-item" },
			want:  "POST /Items/{itemId}: makes the method GetItem, which GET /Items/{itemId} makes too: give one of them a name of its own with a workaround",
		},
		{
			name:  "a workaround's name that is not a Go name",
			patch: func(s *openapi.Spec) { s.Operation("POST", "/Items/{itemId}").Name = "update item" },
			want:  `POST /Items/{itemId}: a workaround names it "update item", which is not an exported Go name`,
		},
		{
			name:  "a workaround's name that is not exported",
			patch: func(s *openapi.Spec) { s.Operation("POST", "/Items/{itemId}").Name = "updateItem" },
			want:  `POST /Items/{itemId}: a workaround names it "updateItem", which is not an exported Go name`,
		},
		{
			name:  "duplicate operationId",
			patch: func(s *openapi.Spec) { s.Operation("POST", "/Items/{itemId}").OperationID = "GetItem" },
			want:  `operationId "GetItem" is also`,
		},
		{
			name: "no success response",
			patch: func(s *openapi.Spec) {
				s.Operation("POST", "/Items/{itemId}").Responses = map[string]*openapi.Response{"404": {}}
			},
			want: "POST /Items/{itemId}: declares no success response",
		},
		{
			name:  "no tag",
			patch: func(s *openapi.Spec) { s.Operation("POST", "/Union").Tags = nil },
			want:  "POST /Union: has no tag",
		},
		{
			name:  "type name clash",
			patch: func(s *openapi.Spec) { s.Components.Schemas["ItemBlur"] = &openapi.Schema{Type: "object"} },
			want:  "type ItemBlur is declared by both",
		},
		{
			// the component is declared first (schemas come before
			// operations), so the operation's own inline response must not
			// quietly become the component's shape
			name: "component named like an operation's inline answer",
			patch: func(s *openapi.Spec) {
				s.Components.Schemas["GetItemAnswer"] = &openapi.Schema{Type: "object", Properties: map[string]*openapi.Schema{"Other": {Type: "string"}}}
				s.Operation("GET", "/Items/{itemId}").Responses["200"].Content["application/json"].Schema = &openapi.Schema{Type: "object", Properties: map[string]*openapi.Schema{"Id": {Type: "string"}}}
			},
			want: "type GetItemAnswer is declared by both",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := importMini(t, tt.patch)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "fix each with a workaround") {
				t.Errorf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// The shapes TMDB's document has that the servers' do not: every type
// inline, named after its operation; a request body declared inline; a
// Content-Type header parameter, which OpenAPI says to ignore; and "_id"
// beside "id".
func TestInlineShapes(t *testing.T) {
	t.Parallel()

	spec, err := openapi.Parse([]byte(`{
  "openapi": "3.1.0", "info": {"title": "tmdb-api", "version": "3"},
  "paths": {
    "/3/movie/{movie_id}": {"get": {"operationId": "movie-details", "tags": ["Movie"],
      "parameters": [{"name": "movie_id", "in": "path", "required": true, "schema": {"type": "integer", "format": "int32"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {
        "id": {"type": "integer"}, "_id": {"type": "string"}, "title": {"type": "string"}}}}}}}}},
    "/3/movie/{movie_id}/rating": {"post": {"operationId": "movie-add-rating", "tags": ["Movie"],
      "parameters": [{"name": "movie_id", "in": "path", "required": true, "schema": {"type": "integer"}},
        {"name": "Content-Type", "in": "header", "required": true, "schema": {"type": "string"}}],
      "requestBody": {"content": {"application/json": {"schema": {"type": "object", "properties": {"value": {"type": "number"}}}}}},
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"success": {"type": "boolean"}}}}}}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := FromSpec(config.Service{Name: "tmdb", Package: "tmdb", Naming: config.OperationIDNaming, Auth: "Token"}, spec, nil, func(msg string) { t.Log(msg) })
	if err != nil {
		t.Fatal(err)
	}

	ops := map[string]*definitions.Operation{}
	for _, op := range svc.Operations() {
		ops[op.Name] = op
	}
	details, rating := ops["MovieDetails"], ops["MovieAddRating"]
	if details == nil || rating == nil {
		t.Fatalf("operations = %v", ops)
	}
	// named after the method, not the path (GetN3MovieByMovieId...)
	if details.Response.Type.ReferenceName != "MovieDetailsAnswer" || rating.Request.Type.ReferenceName != "MovieAddRatingRequest" {
		t.Errorf("response %+v, request %+v", details.Response.Type, rating.Request.Type)
	}
	if len(rating.Options) != 0 {
		t.Errorf("the Content-Type header became an option: %+v", rating.Options)
	}

	models := svc.Models()
	fields := make([]string, 0, len(models["MovieDetailsAnswer"].Fields))
	for _, f := range models["MovieDetailsAnswer"].Fields {
		fields = append(fields, f.Name+"="+f.JSONName+":"+string(f.Type.Type))
	}
	slices.Sort(fields)
	if want := []string{"Id=id:Integer", "Title=title:String", "UnderscoreId=_id:String"}; !slices.Equal(fields, want) {
		t.Errorf("fields = %v, want %v", fields, want)
	}
	if value := models["MovieAddRatingRequest"]; value == nil || len(value.Fields) != 1 || value.Fields[0].Type.Type != definitions.Double {
		t.Errorf("the rating body = %+v", value)
	}
}

// importMiniWarnings imports the patched mini document and returns the
// warnings the importer logged.
func importMiniWarnings(t *testing.T, patch func(*openapi.Spec)) (svc *definitions.Service, warnings []string) {
	t.Helper()

	spec, err := openapi.Parse([]byte(miniSpec))
	if err != nil {
		t.Fatal(err)
	}
	patch(spec)
	svc, err = FromSpec(miniConfig, spec, nil, func(msg string) {
		if strings.Contains(msg, "warning:") {
			warnings = append(warnings, msg)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	return svc, warnings
}

// How an option travels: a list comma-separated or one key per value, an
// object as one JSON string or, in deepObject style, one key per field.
func TestOptionWire(t *testing.T) {
	t.Parallel()

	no := false
	svc, err := importMini(t, func(s *openapi.Spec) {
		op := s.Operation("GET", "/Items")
		op.Parameters = append(op.Parameters,
			&openapi.Parameter{Name: "deep", In: openapi.InQuery, Style: "deepObject", Schema: &openapi.Schema{Type: openapi.TypeObject, AdditionalProperties: []byte(`{"type": "string"}`)}},
			&openapi.Parameter{Name: "years", In: openapi.InQuery, Schema: &openapi.Schema{Type: openapi.TypeArray, Items: &openapi.Schema{Type: openapi.TypeInteger, Format: "int64"}}},
			&openapi.Parameter{Name: "counts", In: openapi.InQuery, Schema: &openapi.Schema{Type: openapi.TypeArray, Items: &openapi.Schema{Type: openapi.TypeInteger}}},
			&openapi.Parameter{Name: "X-Tags", In: openapi.InHeader, Schema: &openapi.Schema{Type: openapi.TypeArray, Items: &openapi.Schema{Type: openapi.TypeString}}},
			&openapi.Parameter{Name: "X-Each", In: openapi.InHeader, Explode: new(true), Schema: &openapi.Schema{Type: openapi.TypeArray, Items: &openapi.Schema{Type: openapi.TypeString}}},
			&openapi.Parameter{Name: "X-Comma", In: openapi.InHeader, Explode: &no, Schema: &openapi.Schema{Type: openapi.TypeArray, Items: &openapi.Schema{Type: openapi.TypeString}}},
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	get := *find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "GetItems" })
	opt := func(name string) definitions.Option {
		t.Helper()
		o := find(get.Options, func(o definitions.Option) bool { return o.Name == name })
		if o == nil {
			t.Fatalf("no option %s", name)
		}
		return *o
	}

	if o := opt("deep"); o.Type.String() != "Dictionary[String]" || !o.DeepObject {
		t.Errorf("deepObject option = %+v", o)
	}
	if o := opt("streamOptions"); o.Type.String() != "Dictionary[String]" || o.DeepObject {
		t.Errorf("JSON-string object option = %+v", o)
	}
	if o := opt("years"); o.Type.String() != "List[Integer64]" || o.CommaSeparated {
		t.Errorf("int64 list option = %+v", o)
	}
	if o := opt("counts"); o.Type.String() != "List[Integer]" {
		t.Errorf("int list option = %+v", o)
	}
	if o := opt("X-Tags"); o.In != definitions.InHeader || o.Type.String() != "List[String]" || !o.CommaSeparated {
		t.Errorf("header list without explode = %+v, want comma-separated", o)
	}
	if o := opt("X-Each"); o.In != definitions.InHeader || o.CommaSeparated {
		t.Errorf("header list with explode true = %+v, want one key per value", o)
	}
	if o := opt("X-Comma"); !o.CommaSeparated {
		t.Errorf("header list with explode false = %+v, want comma-separated", o)
	}
	for _, name := range []string{"deep", "years", "X-Tags"} {
		if o := opt(name); o.DeepObject && o.Type.Type != definitions.Dictionary || o.CommaSeparated && o.Type.Type != definitions.List {
			t.Errorf("%s: wire flags on the wrong type: %+v", name, o)
		}
	}
}

// A document's own Accept header is ignored, as OpenAPI says, and one a
// workaround adds is an option: the server chooses its answer by it, so the
// caller is given the say.
func TestAcceptOption(t *testing.T) {
	t.Parallel()

	accept := func(svc *definitions.Service) *definitions.Option {
		get := *find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "GetItems" })

		return find(get.Options, func(o definitions.Option) bool { return o.Name == "Accept" })
	}

	svc, err := importMini(t, func(s *openapi.Spec) {
		op := s.Operation("GET", "/Items")
		op.Parameters = append(op.Parameters, &openapi.Parameter{Name: "Accept", In: openapi.InHeader, Schema: &openapi.Schema{Type: openapi.TypeString}})
	})
	if err != nil || accept(svc) != nil {
		t.Errorf("a document's own Accept header = %+v, %v, want it ignored", accept(svc), err)
	}

	svc, err = importMini(t, func(s *openapi.Spec) {
		if err := workarounds.AddParameters(s, []string{"GET /Items"}, workarounds.Param{Name: "Accept", In: openapi.InHeader, Type: openapi.TypeString}, workarounds.Param{Name: "Authorization", In: openapi.InHeader, Type: openapi.TypeString}); err != nil {
			t.Error(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if o := accept(svc); o == nil || o.In != definitions.InHeader || o.Field != "Accept" {
		t.Errorf("an Accept header a workaround added = %+v, want an option", o)
	}

	// only Accept: the credentials and the body still set the other two
	get := *find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "GetItems" })
	if o := find(get.Options, func(o definitions.Option) bool { return o.Name == "Authorization" }); o != nil {
		t.Errorf("an Authorization header a workaround added = %+v, want it ignored", o)
	}
}

// An answer a workaround says is short text is read whole into a string,
// where any other answer that is not JSON is a file, and the call asks for
// its type.
func TestTextAnswer(t *testing.T) {
	t.Parallel()

	svc, err := importMini(t, func(s *openapi.Spec) {
		w := workarounds.WrongAnswers(workarounds.About{Name: "mini-item-text", Service: "mini", Bug: "the item is sent as text"}, map[string]workarounds.Correction{
			"GET /Items/{itemId}": {Declared: workarounds.Model("Item"), Answers: workarounds.Text("text/plain")},
		})
		if err := w.Apply(s); err != nil {
			t.Error(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	answer := func(name string) *definitions.Body {
		return (*find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == name })).Response
	}

	text := answer("GetItem")
	if text.ContentType != "text/plain" || text.Type.Type != definitions.String || !text.Text() || text.AsksFor() != "text/plain" {
		t.Errorf("an answer of text = %+v, want a string of text/plain", text)
	}

	// a file is still a file, and JSON asks for nothing of its own
	file := answer("GetItemImage")
	if file.Type.Type != definitions.RawFile || file.Text() || file.AsksFor() != "image/*" {
		t.Errorf("a file = %+v", file)
	}
	list := answer("GetItems")
	if list.Text() || list.AsksFor() != "" {
		t.Errorf("a JSON answer = %+v", list)
	}
}

// What the importer warns about and skips rather than failing on.
func TestImportWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		patch func(*openapi.Spec)
		want  string
		check func(t *testing.T, svc *definitions.Service)
	}{
		{
			name: "two properties folding to one field",
			patch: func(s *openapi.Spec) {
				s.Components.Schemas["Item"].Properties["id"] = &openapi.Schema{Type: openapi.TypeInteger}
			},
			want: `mini: warning: Item: properties "Id" and "id" are both field Id; "id" skipped`,
			check: func(t *testing.T, svc *definitions.Service) {
				t.Helper()
				item := svc.Models()["Item"]
				n := 0
				for _, f := range item.Fields {
					if f.Name == "Id" {
						n++
						if f.JSONName != "Id" || f.Type.Type != definitions.String {
							t.Errorf("Id = %+v, want the first property kept", f)
						}
					}
				}
				if n != 1 {
					t.Errorf("Item has %d Id fields", n)
				}
			},
		},
		{
			name: "two parameters folding to one field",
			patch: func(s *openapi.Spec) {
				op := s.Operation("GET", "/Items")
				op.Parameters = append(op.Parameters, &openapi.Parameter{Name: "user_id", In: openapi.InQuery, Schema: &openapi.Schema{Type: openapi.TypeInteger}})
			},
			want: `mini: warning: GET /Items: parameters "userId" and "user_id" are both field UserId; "user_id" skipped`,
			check: func(t *testing.T, svc *definitions.Service) {
				t.Helper()
				get := *find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "GetItems" })
				if o := find(get.Options, func(o definitions.Option) bool { return o.Name == "user_id" }); o != nil {
					t.Errorf("user_id became an option: %+v", o)
				}
				if o := find(get.Options, func(o definitions.Option) bool { return o.Field == "UserId" }); o == nil || o.Name != "userId" {
					t.Errorf("UserId = %+v, want the first parameter kept", o)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, warnings := importMiniWarnings(t, tt.patch)
			if !slices.Contains(warnings, tt.want) {
				t.Errorf("warnings = %q, want %q", warnings, tt.want)
			}
			tt.check(t, svc)
		})
	}
}

// An XML-only request body cannot be sent by the generated client, so the
// import fails rather than guessing at a content type.
func TestXMLOnlyRequestBody(t *testing.T) {
	t.Parallel()

	_, err := importMini(t, func(s *openapi.Spec) {
		s.Operation("POST", "/Items/{itemId}").RequestBody.Content = map[string]*openapi.MediaType{
			"application/xml": {Schema: &openapi.Schema{Ref: openapi.SchemaRefPrefix + "Item"}},
			"text/xml":        {Schema: &openapi.Schema{Ref: openapi.SchemaRefPrefix + "Item"}},
		}
	})
	if err == nil || !strings.Contains(err.Error(), "POST /Items/{itemId}: request body is XML only") || !strings.Contains(err.Error(), "fix each with a workaround") {
		t.Errorf("err = %v", err)
	}
}

// An option whose $ref names no schema is sent as a string: the servers bind
// query parameters from text whatever the document says.
func TestOptionUndefinedRef(t *testing.T) {
	t.Parallel()

	svc, err := importMini(t, func(s *openapi.Spec) {
		op := s.Operation("GET", "/Items")
		op.Parameters = append(op.Parameters,
			&openapi.Parameter{Name: "ghost", In: openapi.InQuery, Schema: &openapi.Schema{Ref: openapi.SchemaRefPrefix + "Ghost"}},
			&openapi.Parameter{Name: "ghosts", In: openapi.InQuery, Schema: &openapi.Schema{Type: openapi.TypeArray, Items: &openapi.Schema{Ref: openapi.SchemaRefPrefix + "Ghost"}}},
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	get := *find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "GetItems" })
	if o := find(get.Options, func(o definitions.Option) bool { return o.Name == "ghost" }); o == nil || o.Type.String() != "String" {
		t.Errorf("undefined $ref option = %+v, want String", o)
	}
	if o := find(get.Options, func(o definitions.Option) bool { return o.Name == "ghosts" }); o == nil || o.Type.String() != "List[String]" {
		t.Errorf("undefined $ref list option = %+v, want List[String]", o)
	}
}

// The fields the service config names in KeepNull are kept apart from their
// zero, and nothing else is; a name that is neither a string nor a nullable
// number fails the import, so a stale entry is noticed rather than keeping
// nothing.
func TestKeepNull(t *testing.T) {
	t.Parallel()

	importKeeping := func(t *testing.T, keep ...string) (*definitions.Service, error) {
		t.Helper()

		spec, err := openapi.Parse([]byte(miniSpec))
		if err != nil {
			t.Fatal(err)
		}
		spec.Components.Schemas["Item"].Properties["Season"] = &openapi.Schema{Type: openapi.TypeInteger, Nullable: true}
		// an inline record the importer names, which KeepNull reaches by that name
		spec.Components.Schemas["Item"].Properties["Blur"].Properties["Ok"] = &openapi.Schema{Type: openapi.TypeBoolean}
		cfg := miniConfig
		cfg.KeepNull = keep

		return FromSpec(cfg, spec, nil, func(msg string) { t.Log(msg) })
	}

	svc, err := importKeeping(t, "Item.Season", "Item.Id", "ItemBlur.Ok")
	if err != nil {
		t.Fatal(err)
	}
	kept := 0
	for _, m := range svc.Models() {
		for _, f := range m.Fields {
			want := m.SchemaName == "Item" && (f.JSONName == "Season" || f.JSONName == "Id") || m.Name == "ItemBlur" && f.JSONName == "Ok"
			if f.KeepsNull != want {
				t.Errorf("%s.%s keeps null %t, want %t", m.Name, f.JSONName, f.KeepsNull, want)
			}
			if f.KeepsNull {
				kept++
			}
		}
	}
	if kept != 3 {
		t.Errorf("%d fields keep null, want Item.Season, Item.Id and ItemBlur.Ok alone", kept)
	}

	// an answer declared inline, in a document with no components, is a model only once its operation is read
	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Dock", "version": "1"}, "paths": {
    "/registries/test": {"post": {"operationId": "PostRegistriesTest", "tags": ["Registries"],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"authenticated": {"type": "boolean"}, "message": {"type": "string"}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	svc, err = FromSpec(config.Service{Name: "dock", Package: "dock", Naming: config.OperationIDNaming, Auth: "APIKey", KeepNull: []string{"PostRegistriesTestAnswer.authenticated"}}, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f := fieldNamed(svc.Models()["PostRegistriesTestAnswer"].Fields, "authenticated"); f == nil || !f.KeepsNull {
		t.Errorf("PostRegistriesTestAnswer.authenticated = %+v, want kept", f)
	}

	for keep, want := range map[string]string{
		"Item.Ghost":  "KeepNull names Item.Ghost, a field the document does not have",
		"Ghost.Id":    "KeepNull names Ghost.Id, a field the document does not have",
		"ItemSeason":  "KeepNull names ItemSeason, a field the document does not have",
		"Item.Parent": "KeepNull names Item.Parent, which is Reference(Item), not a boolean, a string or a number",
		"Item.Rating": "KeepNull names Item.Rating, which the document no longer declares nullable",
	} {
		if _, err := importKeeping(t, keep); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("KeepNull %s: err = %v, want containing %q", keep, err, want)
		}
	}
}

// Which operations are lists is the service config's to say: the two
// parameters and the two properties its Paging names, whatever their case,
// and none at all for a service that names none.
func TestPaging(t *testing.T) {
	t.Parallel()

	importPaging := func(t *testing.T, paging *config.Paging) *definitions.Operation {
		t.Helper()

		spec, err := openapi.Parse([]byte(miniSpec))
		if err != nil {
			t.Fatal(err)
		}
		cfg := miniConfig
		cfg.Paging = paging
		svc, err := FromSpec(cfg, spec, nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		return *find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "GetItems" })
	}

	if o := importPaging(t, nil); o.Pageable != nil {
		t.Errorf("a service with no paging has a list: %+v", o.Pageable)
	}
	if o := importPaging(t, &config.Paging{Start: "startindex", Limit: "LIMIT", Items: "items", Total: "totalRecordCount"}); o.Pageable == nil || o.Pageable.StartIndexOption != "StartIndex" || o.Pageable.ItemsField != "Items" {
		t.Errorf("paging names in another case = %+v", o.Pageable)
	}
	// a first page is only for a list paged by number, and never below 0
	for _, paging := range []config.Paging{{Start: "startIndex", Limit: "limit", Items: "Items", Total: "TotalRecordCount", FirstPage: new(0)}, {Start: "startIndex", Limit: "limit", Items: "Items", Total: "TotalRecordCount", ByPage: true, FirstPage: new(-1)}} {
		spec, err := openapi.Parse([]byte(miniSpec))
		if err != nil {
			t.Fatal(err)
		}
		cfg := miniConfig
		cfg.Paging = &paging
		if _, err := FromSpec(cfg, spec, nil, nil); err == nil || !strings.Contains(err.Error(), "FirstPage") {
			t.Errorf("FirstPage %d with ByPage %v imported: %v", *paging.FirstPage, paging.ByPage, err)
		}
	}
	for name, paging := range map[string]*config.Paging{
		"another start":       {Start: "page", Limit: "limit", Items: "Items", Total: "TotalRecordCount"},
		"another limit":       {Start: "startIndex", Limit: "pageSize", Items: "Items", Total: "TotalRecordCount"},
		"another list":        {Start: "startIndex", Limit: "limit", Items: "records", Total: "TotalRecordCount"},
		"another total":       {Start: "startIndex", Limit: "limit", Items: "Items", Total: "totalRecords"},
		"nothing named":       {},
		"a total, not a list": {Start: "startIndex", Limit: "limit", Items: "TotalRecordCount", Total: "TotalRecordCount"},
	} {
		if o := importPaging(t, paging); o.Pageable != nil {
			t.Errorf("%s: GetItems is a list: %+v", name, o.Pageable)
		}
	}
}

// Import reads the document the service resolved to and applies the
// workarounds that are for it, recording them in the definitions.
func TestImportFromFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := miniConfig.In(root)
	if err := os.MkdirAll(filepath.Join(root, config.DefaultDir), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, cfg.SpecPath("1")), []byte(miniSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := cfg.Resolve()
	if err != nil {
		t.Fatal(err)
	}

	all := []workarounds.Workaround{
		retitle{"other-title", "other", "Other"},
		retitle{"mini-title", "mini", "Mini, fixed"},
	}
	var logged []string
	svc, err := Import(cfg, all, func(msg string) { logged = append(logged, msg) })
	if err != nil {
		t.Fatal(err)
	}
	if svc.Title != "Mini, fixed" || !slices.Equal(svc.Workarounds, []string{"mini-title"}) || svc.Source != cfg.Spec || !slices.Contains(logged, "mini: applied workaround mini-title") {
		t.Errorf("Import = %q, workarounds %v, source %q, logged %v", svc.Title, svc.Workarounds, svc.Source, logged)
	}

	cfg.Spec = cfg.SpecPath("2")
	if _, err := Import(cfg, nil, nil); err == nil {
		t.Error("Import of a document that is not there succeeded")
	}
}

// A JSON-RPC service vendors its method list as <name>-methods-<version>.json, which Resolve finds and Import reads through
// the jsonrpc loader: every method is a call, grouped by its namespace.
func TestImportJSONRPCFromFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.Service{Name: "truenas", Package: "truenas", Naming: config.OperationIDNaming, Document: config.JSONRPCDocument, ReadRole: "READONLY_ADMIN"}.In(root)
	if err := os.MkdirAll(filepath.Join(root, config.DefaultDir), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.DefaultDir, "truenas-methods-25.10.7.json"), []byte(methods), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := cfg.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "25.10.7" || cfg.Spec != filepath.Join(config.DefaultDir, "truenas-methods-25.10.7.json") {
		t.Fatalf("Resolve = version %q, document %q", cfg.Version, cfg.Spec)
	}

	svc, err := Import(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	o := find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "PoolQuery" })
	if o == nil || (*o).Call == nil || !(*o).Call.Read || !(*o).Call.Filterable || svc.Source != cfg.Spec {
		t.Errorf("Import of a methods file: PoolQuery %+v, source %q", o, svc.Source)
	}
	if g := find(svc.Groups, func(g definitions.Group) bool { return g.Name == "Pool" }); g == nil {
		t.Errorf("Import of a methods file: no Pool group among %d groups", len(svc.Groups))
	}
}

// retitle is a workaround that renames the document, once.
type retitle struct{ name, service, title string }

func (r retitle) Name() string    { return r.name }
func (r retitle) Service() string { return r.service }
func (retitle) Bug() string       { return "the document has the wrong title" }
func (r retitle) Apply(spec *openapi.Spec) error {
	if spec.Info.Title == r.title {
		return errors.New("the title is right")
	}
	spec.Info.Title = r.title

	return nil
}

// arrSpec is a document in the shape Swashbuckle writes for Sonarr, Radarr and
// Prowlarr: no operationIds, every path under a prefix, words run together,
// text/plain listed beside JSON, lists that page by page number, a failure
// that answers the success shape, and a settings object saved whole.
const arrSpec = `{"openapi": "3.0.1", "info": {"title": "Arr", "version": "3.0.0"}, "paths": {
  "/api/v3/history": {"get": {"tags": ["History"], "parameters": [
      {"name": "page", "in": "query", "schema": {"type": "integer", "format": "int32"}},
      {"name": "pageSize", "in": "query", "schema": {"type": "integer", "format": "int32"}}],
    "responses": {"200": {"content": {
      "text/plain": {"schema": {"$ref": "#/components/schemas/HistoryPage"}},
      "application/json": {"schema": {"$ref": "#/components/schemas/HistoryPage"}},
      "text/json": {"schema": {"$ref": "#/components/schemas/HistoryPage"}}}}}}},
  "/api/v3/episodefile/{id}": {"get": {"tags": ["EpisodeFile"], "parameters": [{"name": "id", "in": "path", "schema": {"type": "integer"}}],
    "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Record"}}}}}}},
  "/api/v3/indexer/testall": {"post": {"tags": ["Indexer"], "responses": {
    "200": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Record"}}}}},
    "400": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Record"}}}}},
    "500": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Record"}}}},
    "default": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Record"}}}}}}}},
  "/api/v3/config/host": {"put": {"tags": ["HostConfig"],
    "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/HostConfig"}}}},
    "responses": {"202": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/HostConfig"}}}}}}},
  "/ping": {"get": {"tags": ["Ping"], "responses": {"200": {"content": {"text/plain": {"schema": {"type": "string"}}}}}}}},
  "components": {"schemas": {
    "Kind": {"type": "string", "enum": ["a", "b"]},
    "Record": {"type": "object", "required": ["title"], "properties": {"id": {"type": "integer"}, "title": {"type": "string"}, "monitored": {"type": "boolean"}}},
    "HistoryPage": {"type": "object", "properties": {"page": {"type": "integer"}, "pageSize": {"type": "integer"}, "totalRecords": {"type": "integer"},
      "records": {"type": "array", "items": {"$ref": "#/components/schemas/Record"}}}},
    "HostConfig": {"type": "object", "properties": {"bindAddress": {"type": "string"}, "port": {"type": "integer"}, "enableSsl": {"type": "boolean"}}}}}}`

var arrConfig = config.Service{
	Name: "arr", Package: "arr", Naming: config.PathNaming, Auth: "Token",
	PathPrefix: "/api/v3", Words: map[string]string{"episodefile": "EpisodeFile", "testall": "TestAll"},
	Paging:     &config.Paging{Start: "page", Limit: "pageSize", Items: "records", Total: "totalRecords", ByPage: true},
	PreferJSON: true, ExpectSameShape: true, SendRequired: true, WrittenWhole: []string{"HostConfig"},
}

func importArr(t *testing.T, patch func(*config.Service)) (*definitions.Service, error) {
	t.Helper()

	spec, err := openapi.Parse([]byte(arrSpec))
	if err != nil {
		t.Fatal(err)
	}
	cfg := arrConfig
	if patch != nil {
		patch(&cfg)
	}

	return FromSpec(cfg, spec, nil, nil)
}

// What a service's config asks for beyond the defaults, each of which a
// service that does not ask keeps as it was.
func TestServiceOptions(t *testing.T) {
	t.Parallel()

	operation := func(t *testing.T, svc *definitions.Service, key string) *definitions.Operation {
		t.Helper()

		o := find(svc.Operations(), func(o *definitions.Operation) bool { return o.Key() == key })
		if o == nil {
			t.Fatalf("no operation %s", key)
		}

		return *o
	}
	field := func(t *testing.T, svc *definitions.Service, model, jsonName string) *definitions.Field {
		t.Helper()

		return fieldNamed(svc.Models()[model].Fields, jsonName)
	}

	svc, err := importArr(t, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("names leave the prefix out and spell the words", func(t *testing.T) {
		t.Parallel()

		for key, want := range map[string]string{
			"GET /api/v3/history": "GetHistory", "GET /api/v3/episodefile/{id}": "GetEpisodeFileById", "POST /api/v3/indexer/testall": "PostIndexerTestAll",
			"PUT /api/v3/config/host": "PutConfigHost", "GET /ping": "GetPing",
		} {
			if o := operation(t, svc, key); o.Name != want || o.Path != strings.TrimPrefix(key, o.Method+" ") {
				t.Errorf("%s is %s at %s, want %s at its own path", key, o.Name, o.Path, want)
			}
		}

		plain, err := importArr(t, func(c *config.Service) { c.PathPrefix, c.Words = "", nil })
		if err != nil {
			t.Fatal(err)
		}
		if o := operation(t, plain, "GET /api/v3/episodefile/{id}"); o.Name != "GetApiV3EpisodefileById" {
			t.Errorf("with no prefix and no words the method is %s", o.Name)
		}
	})

	t.Run("words are spelling only", func(t *testing.T) {
		t.Parallel()

		for name, tc := range map[string]struct {
			words map[string]string
			want  string
		}{
			"a key in capitals": {map[string]string{"EpisodeFile": "EpisodeFile"}, `the config's Words has the key "EpisodeFile", which is not lower case`},
			"another word":      {map[string]string{"episodefile": "EpisodeFiles"}, `the config's Words spells "episodefile" as "EpisodeFiles", which is not the same letters`},
		} {
			if _, err := importArr(t, func(c *config.Service) { c.Words = tc.words }); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%s: err = %v, want containing %q", name, err, tc.want)
			}
		}
	})

	t.Run("lists page by page number", func(t *testing.T) {
		t.Parallel()

		want := definitions.Pageable{StartIndexOption: "Page", LimitOption: "PageSize", ByPage: true, ItemsField: "Records", TotalField: "TotalRecords", ItemType: reference("Record")}
		if o := operation(t, svc, "GET /api/v3/history"); o.Pageable == nil || !reflect.DeepEqual(*o.Pageable, want) {
			t.Errorf("Pageable = %+v, want %+v", o.Pageable, want)
		}
	})

	t.Run("JSON is preferred beside another type", func(t *testing.T) {
		t.Parallel()

		if o := operation(t, svc, "GET /api/v3/history"); o.Response.ContentType != "application/json" || o.Response.Type.String() != "Reference(HistoryPage)" {
			t.Errorf("history answers %+v", o.Response)
		}
		// an answer with no JSON at all is still a file
		if o := operation(t, svc, "GET /ping"); o.Response.ContentType != "text/plain" || o.Response.Type.Type != definitions.RawFile {
			t.Errorf("ping answers %+v", o.Response)
		}

		file, err := importArr(t, func(c *config.Service) { c.PreferJSON = false })
		if err != nil {
			t.Fatal(err)
		}
		if o := operation(t, file, "GET /api/v3/history"); o.Response.ContentType != "text/plain" || o.Response.Type.Type != definitions.RawFile || o.Pageable != nil {
			t.Errorf("without the preference history answers %+v (pageable %+v)", o.Response, o.Pageable)
		}
	})

	t.Run("a failure that answers the success shape is expected", func(t *testing.T) {
		t.Parallel()

		// the 400 answers the list of results; the 500 answers something else, and "default" is no status
		if o := operation(t, svc, "POST /api/v3/indexer/testall"); !slices.Equal(o.ExpectedStatusCodes, []int{200, 400}) {
			t.Errorf("test-all expects %v, want 200 and 400", o.ExpectedStatusCodes)
		}

		strict, err := importArr(t, func(c *config.Service) { c.ExpectSameShape = false })
		if err != nil {
			t.Fatal(err)
		}
		if o := operation(t, strict, "POST /api/v3/indexer/testall"); !slices.Equal(o.ExpectedStatusCodes, []int{200}) {
			t.Errorf("without it test-all expects %v, want 200 alone", o.ExpectedStatusCodes)
		}

		// only a failure still needs a success beside it
		spec, err := openapi.Parse([]byte(arrSpec))
		if err != nil {
			t.Fatal(err)
		}
		testAll := spec.Operation("POST", "/api/v3/indexer/testall")
		testAll.Responses = map[string]*openapi.Response{"400": testAll.Responses["400"]}
		if _, err := FromSpec(arrConfig, spec, nil, nil); err == nil || !strings.Contains(err.Error(), "POST /api/v3/indexer/testall: declares no success response") {
			t.Errorf("an operation with only a 400 = %v", err)
		}
	})

	t.Run("required properties are held to", func(t *testing.T) {
		t.Parallel()

		if f := field(t, svc, "Record", "title"); !f.Required {
			t.Error("Record.title is required and is not marked")
		}
		if f := field(t, svc, "Record", "id"); f.Required {
			t.Error("Record.id is not required and is marked")
		}

		unread, err := importArr(t, func(c *config.Service) { c.SendRequired = false })
		if err != nil {
			t.Fatal(err)
		}
		if f := field(t, unread, "Record", "title"); f.Required {
			t.Error("required is read for a service that did not ask")
		}
	})

	t.Run("settings are written whole", func(t *testing.T) {
		t.Parallel()

		for name, m := range svc.Models() {
			if want := name == "HostConfig"; m.WrittenWhole != want {
				t.Errorf("%s written whole = %t, want %t", name, m.WrittenWhole, want)
			}
		}
		for schema, want := range map[string]string{
			"Ghost": "the config's WrittenWhole names Ghost, and the document has no object schema by that name",
			"Kind":  "the config's WrittenWhole names Kind, and the document has no object schema by that name",
		} {
			if _, err := importArr(t, func(c *config.Service) { c.WrittenWhole = []string{schema} }); err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("WrittenWhole %s: err = %v, want containing %q", schema, err, want)
			}
		}
	})
}

// Two paths that make one method name fail the import rather than have the
// second numbered, and a workaround settles it by naming one: the name is the
// method's, and its inline answer's.
func TestNameClash(t *testing.T) {
	t.Parallel()

	// a page outside the prefix, beside the API's own list under it
	withPage := func(t *testing.T, name string) (*definitions.Service, error) {
		t.Helper()

		spec, err := openapi.Parse([]byte(arrSpec))
		if err != nil {
			t.Fatal(err)
		}
		spec.Paths["/history"] = &openapi.PathItem{Get: &openapi.Operation{Tags: []string{"Pages"}, Name: name, Responses: map[string]*openapi.Response{
			"200": {Content: map[string]*openapi.MediaType{"application/json": {Schema: &openapi.Schema{Type: openapi.TypeObject, Properties: map[string]*openapi.Schema{"title": {Type: openapi.TypeString}}}}}},
		}}}

		return FromSpec(arrConfig, spec, nil, nil)
	}

	if _, err := withPage(t, ""); err == nil || !strings.Contains(err.Error(), "GET /history: makes the method GetHistory, which GET /api/v3/history makes too") {
		t.Errorf("two paths that make GetHistory = %v", err)
	}

	svc, err := withPage(t, "GetHistoryPage")
	if err != nil {
		t.Fatal(err)
	}
	page := find(svc.Operations(), func(o *definitions.Operation) bool { return o.Key() == "GET /history" })
	if page == nil || (*page).Name != "GetHistoryPage" || (*page).Response.Type.String() != "Reference(GetHistoryPageAnswer)" {
		t.Errorf("the named operation = %+v", page)
	}
	if list := find(svc.Operations(), func(o *definitions.Operation) bool { return o.Key() == "GET /api/v3/history" }); list == nil || (*list).Name != "GetHistory" {
		t.Errorf("the other operation = %+v", list)
	}
}

// An operation that documents a redirect and no 2xx answers the redirect: it
// is what the method expects, and a GET of that kind need not say what
// content it answers. One that documents a 2xx as well expects both, which
// tells the base client the redirect is on the way to the answer.
func TestRedirectAnswers(t *testing.T) {
	t.Parallel()

	spec, err := openapi.Parse([]byte(arrSpec))
	if err != nil {
		t.Fatal(err)
	}
	moved := func(codes ...string) *openapi.Operation {
		op := &openapi.Operation{Tags: []string{"Authentication"}, Responses: map[string]*openapi.Response{}}
		for _, code := range codes {
			op.Responses[code] = &openapi.Response{}
		}

		return op
	}
	spec.Paths["/login"] = &openapi.PathItem{Post: moved("302", "401")}
	spec.Paths["/logout"] = &openapi.PathItem{Get: moved("302", "303")}
	spec.Paths["/ping"].Get.Responses["307"] = &openapi.Response{}

	svc, err := FromSpec(arrConfig, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string][]int{"POST /login": {302}, "GET /logout": {302, 303}, "GET /ping": {200, 307}, "GET /api/v3/history": {200}} {
		o := find(svc.Operations(), func(o *definitions.Operation) bool { return o.Key() == key })
		if o == nil || !slices.Equal((*o).ExpectedStatusCodes, want) {
			t.Errorf("%s expects %v, want %v", key, (*o).ExpectedStatusCodes, want)
		}
	}
}

// A service whose lists page in more than one shape names each in Paging and
// Pagings: a shape without a limit (TMDB's page of twenty) and one that
// leaves the list's name to the answer (Dockhand's logs and events) both
// mark their lists, and an operation that takes a shape's Start and matches
// none is named in the log with why.
func TestPagingShapes(t *testing.T) {
	t.Parallel()

	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Mixed", "version": "1"}, "paths": {
    "/popular": {"get": {"operationId": "GetPopular", "tags": ["Movies"], "parameters": [{"name": "page", "in": "query", "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"page": {"type": "integer"}, "results": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "integer"}}}}, "total_pages": {"type": "integer"}, "total_results": {"type": "integer"}}}}}}}}},
    "/audit": {"get": {"operationId": "GetAudit", "tags": ["Audit"], "parameters": [{"name": "limit", "in": "query", "schema": {"type": "integer"}}, {"name": "offset", "in": "query", "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"logs": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "integer"}}}}, "total": {"type": "integer"}}}}}}}}},
    "/scan": {"get": {"operationId": "GetScan", "tags": ["Audit"], "parameters": [{"name": "limit", "in": "query", "schema": {"type": "integer"}}, {"name": "offset", "in": "query", "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"findings": {"type": "array", "items": {"type": "string"}}, "fixes": {"type": "array", "items": {"type": "string"}}, "total": {"type": "integer"}}}}}}}}},
    "/changes": {"get": {"operationId": "GetChanges", "tags": ["Movies"], "parameters": [{"name": "page", "in": "query", "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"changes": {"type": "array", "items": {"type": "string"}}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var logged []string
	cfg := config.Service{Name: "mixed", Package: "mixed", Naming: config.OperationIDNaming, Auth: "Token", Paging: &config.Paging{Start: "page", Items: "results", Total: "total_results", ByPage: true}, Pagings: []config.Paging{{Start: "offset", Limit: "limit", Total: "total"}}}
	svc, err := FromSpec(cfg, spec, nil, func(msg string) { logged = append(logged, msg) })
	if err != nil {
		t.Fatal(err)
	}
	ops := map[string]*definitions.Operation{}
	for _, o := range svc.Operations() {
		ops[o.Name] = o
	}
	if p := ops["GetPopular"].Pageable; p == nil || p.LimitOption != "" || !p.ByPage || p.ItemsField != "Results" || p.TotalField != "TotalResults" {
		t.Errorf("a page-only shape = %+v", p)
	}
	if p := ops["GetAudit"].Pageable; p == nil || p.LimitOption != "Limit" || p.StartIndexOption != "Offset" || p.ByPage || p.ItemsField != "Logs" || p.TotalField != "Total" {
		t.Errorf("an offset shape with the list found by itself = %+v", p)
	}
	if ops["GetScan"].Pageable != nil || ops["GetChanges"].Pageable != nil {
		t.Error("an answer with two lists, or without a total, was marked a list")
	}
	for _, want := range []string{
		"mixed: warning: GET /scan takes a paging parameter and gets no Complete method: it takes offset but its answer GetScanAnswer has 2 lists, so Items must name one",
		"mixed: warning: GET /changes takes a paging parameter and gets no Complete method: it takes page but its answer GetChangesAnswer has no list results",
	} {
		if !slices.Contains(logged, want) {
			t.Errorf("the log lacks %q:\n%s", want, strings.Join(logged, "\n"))
		}
	}
}

// A kind of id is applied by the models it is of, by name and by In, with
// Except, Joined and Plain; its base is read from the model's id; a typed id
// on a path the document types as text is kept; and the import refuses what
// it cannot settle and logs what each kind reached and what no kind covers.
func TestIDs(t *testing.T) {
	t.Parallel()

	const doc = `{"openapi": "3.0.1", "info": {"title": "Arr", "version": "1"}, "paths": {
    "/series": {"get": {"operationId": "GetSeries", "tags": ["Series"], "parameters": [{"name": "tvdbId", "in": "query", "schema": {"type": "integer"}}, {"name": "ids", "in": "query", "schema": {"type": "string"}}, {"name": "tags", "in": "query", "schema": {"type": "string"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/SeriesResource"}}}}}}}},
    "/series/{id}": {"get": {"operationId": "GetSeriesById", "tags": ["Series"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/SeriesResource"}}}}}},
      "put": {"operationId": "PutSeriesById", "tags": ["Series"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
      "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/SeriesResource"}}}},
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/SeriesResource"}}}}}}},
    "/episode": {"get": {"operationId": "GetEpisode", "tags": ["Episode"], "parameters": [{"name": "seriesId", "in": "query", "schema": {"type": "integer"}}, {"name": "episodeIds", "in": "query", "schema": {"type": "array", "items": {"type": "integer"}}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/EpisodeResource"}}}}}}}},
    "/series/{id}/folder": {"get": {"operationId": "GetSeriesByIdFolder", "tags": ["Series"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"path": {"type": "string"}}}}}}}}},
    "/series/{id}/drop": {"delete": {"operationId": "DeleteSeriesById", "tags": ["Series"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "integer"}}],
      "responses": {"204": {"description": "gone"}}}},
    "/command/{id}": {"get": {"operationId": "GetCommandById", "tags": ["Command"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "properties": {"id": {"type": "integer"}, "downloadId": {"type": "string"}}}}}}}}}},
  "components": {"schemas": {
    "SeriesResource": {"type": "object", "properties": {"id": {"type": "integer"}, "title": {"type": "string"}, "tvdbId": {"type": "integer"}, "tags": {"type": "array", "items": {"type": "integer"}}, "parentId": {"type": "integer"}}},
    "EpisodeResource": {"type": "object", "properties": {"id": {"type": "integer"}, "seriesId": {"type": "integer"}, "episodeFileId": {"type": "integer"}}},
    "TagResource": {"type": "object", "properties": {"id": {"type": "integer"}, "label": {"type": "string"}}}}}}`
	spec, err := openapi.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var logged []string
	cfg := config.Service{Name: "arr", Package: "arr", Naming: config.OperationIDNaming, Auth: "APIKey", IDs: []config.ID{
		{Name: "SeriesID", Of: []string{"SeriesResource"}, Names: []string{"seriesId"}, Joined: []string{"ids"}, In: []string{"SeriesResource .parentId"}},
		{Name: "EpisodeID", Of: []string{"EpisodeResource"}, Names: []string{"episodeIds"}},
		{Name: "TagID", Of: []string{"TagResource"}, Names: []string{"tags"}, Joined: []string{"tags"}},
		{Name: "TvdbID", Base: "int", Names: []string{"tvdbId"}, Except: []string{"GET /series tvdbId"}},
	}, Plain: []string{"downloadId", "GetCommandByIdAnswer .id"}}
	svc, err := FromSpec(cfg, spec, nil, func(msg string) { logged = append(logged, msg) })
	if err != nil {
		t.Fatal(err)
	}
	ops := map[string]*definitions.Operation{}
	for _, o := range svc.Operations() {
		ops[o.Name] = o
	}
	id := func(kind string) definitions.TypeRef {
		return definitions.TypeRef{Type: definitions.ID, ReferenceName: kind}
	}
	series := svc.Models()["SeriesResource"]
	if got := fieldNamed(series.Fields, "id").Type; got != id("SeriesID") {
		t.Errorf("SeriesResource.id = %+v", got)
	}
	if got := fieldNamed(series.Fields, "parentId").Type; got != id("SeriesID") {
		t.Errorf("SeriesResource.parentId, by In, = %+v", got)
	}
	if got := fieldNamed(series.Fields, "tags").Type; got.Type != definitions.List || *got.NestedItem != id("TagID") {
		t.Errorf("SeriesResource.tags = %+v", got)
	}
	if got := fieldNamed(series.Fields, "tvdbId").Type; got != id("TvdbID") {
		t.Errorf("SeriesResource.tvdbId = %+v", got)
	}
	if got := ops["GetSeriesById"].PathParameters[0].Type; got != id("SeriesID") {
		t.Errorf("GET /series/{id}, by Of, = %+v", got)
	}
	if got := ops["PutSeriesById"].PathParameters[0].Type; got != id("SeriesID") {
		t.Errorf("PUT /series/{id}, text in the document, = %+v", got)
	}
	if got := ops["GetCommandById"].PathParameters[0].Type; got.Type != definitions.Integer {
		t.Errorf("GET /command/{id}, of no kind, = %+v", got)
	}
	if got := ops["GetSeriesByIdFolder"].PathParameters[0].Type; got != id("SeriesID") {
		t.Errorf("GET /series/{id}/folder, under a typed path, = %+v", got)
	}
	if got := ops["DeleteSeriesById"].PathParameters[0].Type; got != id("SeriesID") {
		t.Errorf("DELETE /series/{id}/drop, answering nothing, = %+v", got)
	}
	opts := ops["GetSeries"].Options
	if got := opts[1].Type; got.Type != definitions.List || *got.NestedItem != id("SeriesID") || !opts[1].CommaSeparated {
		t.Errorf("GET /series ids, Joined, = %+v comma %t", got, opts[1].CommaSeparated)
	}
	if got := opts[0].Type; got.Type != definitions.Integer {
		t.Errorf("GET /series tvdbId, Except, = %+v", got)
	}
	if got := opts[2].Type; got.Type != definitions.List || *got.NestedItem != id("TagID") || !opts[2].CommaSeparated {
		t.Errorf("GET /series tags, a name in both Names and Joined, = %+v comma %t", got, opts[2].CommaSeparated)
	}
	if got := ops["GetEpisode"].Options[1].Type; got.Type != definitions.List || *got.NestedItem != id("EpisodeID") {
		t.Errorf("GET /episode episodeIds = %+v", got)
	}
	if len(svc.IDs) != 4 || svc.IDs[0] != (definitions.IDKind{Name: "SeriesID", Base: definitions.Integer}) || svc.IDs[3].Base != definitions.Integer {
		t.Errorf("kinds = %+v", svc.IDs)
	}
	for _, want := range []string{"arr: ids: SeriesID in 9 places (5 from Of, 3 by name, 1 by In)", "arr: ids: TvdbID in 1 place (0 from Of, 1 by name, 0 by In)", "arr: the bare id of 1 place is left plain (those named like a kind's model first): GetCommandByIdAnswer", "arr: 2 names look like ids and have no kind (give each a kind in IDs, or leave it in Plain): episodeFileId in EpisodeResource; id in GET /command/{id}"} {
		if !slices.ContainsFunc(logged, func(l string) bool { return strings.HasPrefix(l, want) }) {
			t.Errorf("the log lacks %q:\n%s", want, strings.Join(logged, "\n"))
		}
	}

	// what the import refuses
	for name, tc := range map[string]struct {
		ids   []config.ID
		plain []string
		want  string
	}{
		"a kind reaching nothing":          {ids: []config.ID{{Name: "GhostID", Base: "int", Names: []string{"ghostId"}}}, want: "the id kind GhostID reaches no field, option or path parameter"},
		"a model that is not there":        {ids: []config.ID{{Name: "GhostID", Of: []string{"Ghost"}}}, want: "GhostID: Of names Ghost, which is not a model of the document"},
		"a model with no id":               {ids: []config.ID{{Name: "GhostID", Of: []string{"GetCommandByIdAnswer"}, Names: []string{"seriesId"}}}, want: ""},
		"two kinds on one place":           {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}}, {Name: "OtherID", Base: "int", Names: []string{"id"}}}, want: "SeriesResource id is claimed by SeriesID and OtherID"},
		"a name both typed and plain":      {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, Names: []string{"seriesId"}}}, plain: []string{"seriesId"}, want: "seriesId is both a name of SeriesID and in Plain"},
		"an In the inference makes stale":  {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, In: []string{"GET /series/{id} id"}}}, want: `SeriesID: In "GET /series/{id} id" is not needed, Of or Names type it, so remove it`},
		"an In matching nothing":           {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, In: []string{"SeriesResource .nothing"}}}, want: `SeriesID: In "SeriesResource .nothing" matches no plain id`},
		"a string kind on a number":        {ids: []config.ID{{Name: "SeriesID", Base: "string", Names: []string{"seriesId"}}}, want: "GET /episode seriesId is a Integer, which cannot be a SeriesID (String)"},
		"a joined string not named Joined": {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, Names: []string{"ids"}}}, want: "GET /series ids is a String, which cannot be a SeriesID (Integer); a string that carries several joined by commas is named in Joined"},
		"a Joined written as a place":      {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, Joined: []string{"GET /series ids"}}}, want: `SeriesID: Joined "GET /series ids" matches no string parameter: it takes a parameter's name, not a place`},
		"a base that is not a type":        {ids: []config.ID{{Name: "SeriesID", Base: "float", Names: []string{"seriesId"}}}, want: `SeriesID: Base "float" is not int, int64 or string`},
		"a shape with an example":          {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, Shape: "^[1-9][0-9]*$", Example: "12"}}, want: ""},
		"a shape that is no expression":    {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, Shape: "^[", Example: "1"}}, want: `SeriesID: Shape "^[" is not a regular expression`},
		"a shape with no example":          {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, Shape: "^[1-9][0-9]*$"}}, want: "SeriesID: a Shape needs an Example"},
		"an example not of the shape":      {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, Shape: "^[1-9][0-9]*$", Example: "0"}}, want: `SeriesID: the Example "0" is not of the Shape "^[1-9][0-9]*$"`},
		"an example that is no number":     {ids: []config.ID{{Name: "SeriesID", Of: []string{"SeriesResource"}, Shape: "^[a-z]+$", Example: "abc"}}, want: `SeriesID: the Example "abc" is not a number, which the kind is`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			spec, _ := openapi.Parse([]byte(doc))
			cfg := config.Service{Name: "arr", Package: "arr", Naming: config.OperationIDNaming, Auth: "APIKey", IDs: tc.ids, Plain: tc.plain}
			err := VerifyIDs(cfg, spec)
			if tc.want == "" {
				if err != nil {
					t.Errorf("VerifyIDs = %v", err)
				}

				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("VerifyIDs = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// Of reads the base off a model that spells its id Id, as Emby and Jellyfin do, so the kind needs no Base beside it.
func TestIDsSpeltId(t *testing.T) {
	t.Parallel()

	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Emby", "version": "1"}, "paths": {
    "/Items/{id}": {"get": {"operationId": "GetItemsById", "tags": ["Items"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}, {"name": "ExcludeItemIds", "in": "query", "schema": {"type": "string"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/BaseItemDto"}}}}}}}},
  "components": {"schemas": {"BaseItemDto": {"type": "object", "properties": {"Id": {"type": "string"}, "Name": {"type": "string"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var logged []string
	cfg := config.Service{Name: "emby", Package: "emby", Naming: config.OperationIDNaming, Auth: "APIKey", IDs: []config.ID{{Name: "ItemID", Of: []string{"BaseItemDto"}, Names: []string{"ExcludeItemIds"}}}}
	svc, err := FromSpec(cfg, spec, nil, func(msg string) { logged = append(logged, msg) })
	if err != nil {
		t.Fatal(err)
	}
	// a text parameter named like a list, typed as one id by name, is said, since only Joined would make it the list it may be
	if want := "emby: 1 text parameter named like a list is typed as one id (name it in Joined if it carries several joined by commas): ExcludeItemIds in GET /Items/{id}"; !slices.Contains(logged, want) {
		t.Errorf("the log lacks %q:\n%s", want, strings.Join(logged, "\n"))
	}
	if got := fieldNamed(svc.Models()["BaseItemDto"].Fields, "Id").Type; got != (definitions.TypeRef{Type: definitions.ID, ReferenceName: "ItemID"}) {
		t.Errorf("BaseItemDto.Id = %+v", got)
	}
	if len(svc.IDs) != 1 || svc.IDs[0].Base != definitions.String {
		t.Errorf("kinds = %+v", svc.IDs)
	}
}

// A command set makes an operation per command beside the one that queues them, each sending its own schema and fixed to its name,
// told apart by its key; the set's Get resolves to that operation's Go name.
func TestCommands(t *testing.T) {
	t.Parallel()

	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Arr", "version": "1"}, "paths": {
    "/api/v3/command": {"post": {"tags": ["Command"], "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/CommandResource"}}}},
      "responses": {"201": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/CommandResource"}}}}}}},
    "/api/v3/command/{id}": {"get": {"tags": ["Command"], "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "integer"}}],
      "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/CommandResource"}}}}}}}},
  "components": {"schemas": {"CommandResource": {"type": "object", "properties": {"id": {"type": "integer"}, "name": {"type": "string"}, "status": {"type": "string"}, "message": {"type": "string"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	w := workarounds.Commands(workarounds.About{Name: "arr-commands", Service: "arr", Bug: "the command body is an open record"}, workarounds.CommandSet{
		Post: "POST /api/v3/command", Get: "GET /api/v3/command/{id}", Status: "status", Finished: []string{"completed", "failed"}, Failed: []string{"failed"},
		Commands: map[string]map[string]*openapi.Schema{"RefreshSeries": {"seriesId": {Type: "integer"}}, "RssSync": {}},
	})
	if err := w.Apply(spec); err != nil {
		t.Fatal(err)
	}
	cfg := config.Service{Name: "arr", Package: "arr", Naming: config.PathNaming, PathPrefix: "/api/v3", Auth: "APIKey"}
	svc, err := FromSpec(cfg, spec, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(svc.Operations()))
	for _, o := range svc.Operations() {
		keys = append(keys, o.Key())
	}
	slices.Sort(keys)
	if want := []string{"GET /api/v3/command/{id}", "POST /api/v3/command", "POST /api/v3/command RefreshSeries", "POST /api/v3/command RssSync"}; !slices.Equal(keys, want) {
		t.Errorf("keys = %q", keys)
	}
	post := find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "PostCommand" })
	if post == nil || (*post).Commands == nil || (*post).Commands.Get != "GetCommandById" || (*post).Commands.Status != "status" {
		t.Fatalf("the queueing operation = %+v", post)
	}
	refresh := find(svc.Operations(), func(o *definitions.Operation) bool { return o.Name == "RefreshSeries" })
	if refresh == nil || (*refresh).Command != "RefreshSeries" || (*refresh).Request.Type.ReferenceName != "RefreshSeriesCommand" || (*refresh).Response.Type.ReferenceName != "CommandResource" {
		t.Errorf("the command's operation = %+v", refresh)
	}
	m := svc.Models()["RefreshSeriesCommand"]
	if m == nil || len(m.Fields) != 2 || m.Fields[0].JSONName != "name" && m.Fields[1].JSONName != "name" {
		t.Errorf("the command's model = %+v", m)
	}

	// the refusals
	for name, set := range map[string]workarounds.CommandSet{
		"again":               {Post: "POST /api/v3/command", Get: "GET /api/v3/command/{id}", Status: "status", Finished: []string{"completed"}, Commands: map[string]map[string]*openapi.Schema{"Other": {}}},
		"no such post":        {Post: "POST /api/v3/nothing", Get: "GET /api/v3/command/{id}", Status: "status", Finished: []string{"completed"}, Commands: map[string]map[string]*openapi.Schema{"Other": {}}},
		"no finished":         {Post: "POST /api/v3/command", Get: "GET /api/v3/command/{id}", Status: "status", Commands: map[string]map[string]*openapi.Schema{"Other": {}}},
		"no commands":         {Post: "POST /api/v3/command", Get: "GET /api/v3/command/{id}", Status: "status", Finished: []string{"completed"}},
		"a field called name": {Post: "POST /api/v3/command", Get: "GET /api/v3/command/{id}", Status: "status", Finished: []string{"completed"}, Commands: map[string]map[string]*openapi.Schema{"Other": {"name": {Type: "string"}}}},
		"not a Go name":       {Post: "POST /api/v3/command", Get: "GET /api/v3/command/{id}", Status: "status", Finished: []string{"completed"}, Commands: map[string]map[string]*openapi.Schema{"refresh-series": {}}},
	} {
		if err := workarounds.Commands(workarounds.About{Name: "x", Service: "arr"}, set).Apply(spec); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
