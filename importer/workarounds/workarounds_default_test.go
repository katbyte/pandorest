package workarounds_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/importer/workarounds"
	"github.com/katbyte/pandorest/openapi"
)

// buggy has one of each bug a default workaround fixes: GETs that declare
// no answer, answers and statuses that are wrong, a parameter and a field
// left out, and the web interface's page.
const buggy = `{"openapi": "3.0.1", "info": {"title": "Buggy", "version": "1"}, "paths": {
  "/": {"get": {"operationId": "Page", "tags": ["Web"], "responses": {"200": {}}}},
  "/Things": {
    "get": {"operationId": "GetThings", "tags": ["Things"], "parameters": [{"name": "kinds", "in": "query", "schema": {"type": "string"}}, {"name": "names", "in": "query", "schema": {"type": "string"}}], "responses": {"200": {}}},
    "post": {"operationId": "CreateThing", "tags": ["Things"], "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Thing"}}}}}}},
  "/Things/bulk": {"put": {"operationId": "UpdateThings", "tags": ["Things"], "responses": {"200": {"content": {
    "text/plain": {"schema": {"$ref": "#/components/schemas/Thing"}},
    "application/json": {"schema": {"$ref": "#/components/schemas/Thing"}},
    "text/json": {"schema": {"$ref": "#/components/schemas/Thing"}}}}}}},
  "/Things/{id}/cover": {"get": {"operationId": "GetCover", "tags": ["Things"], "parameters": [{"name": "id", "in": "path", "schema": {"type": "integer"}}], "responses": {"200": {}}}},
  "/Settings": {"get": {"operationId": "GetSettings", "tags": ["System"], "responses": {"200": {}}}},
  "/Stats": {"get": {"operationId": "GetStats", "tags": ["System"], "responses": {"200": {}}}},
  "/Path": {"get": {"operationId": "GetPath", "tags": ["System"], "responses": {"200": {"content": {"application/json": {"schema": {"type": "string"}}}}}}},
  "/Ping": {"get": {"operationId": "Ping", "tags": ["System"], "responses": {"200": {}}}},
  "/Version": {"get": {"operationId": "GetVersion", "tags": ["System"], "responses": {"200": {"content": {"text/plain": {"schema": {"type": "string"}}}}}}}},
  "components": {"schemas": {"Thing": {"type": "object", "properties": {"id": {"type": "integer"}, "createdAt": {"type": "string"}, "seenAt": {"type": "string", "format": "date-time"}, "born": {"type": "string"}, "stamp": {"type": "integer"}, "status": {"type": "string"}, "details": {"type": "object", "properties": {"status": {"type": "string"}}}}},
    "Note": {"type": "object", "properties": {"createdAt": {"type": "string"}}},
    "Kind": {"type": "string", "enum": ["movie", "show"]}}}}`

func loadBuggy(string) (*openapi.Spec, error) { return openapi.Parse([]byte(buggy)) }

func about(name, bug string) workarounds.About {
	return workarounds.About{Name: "buggy-" + name, Service: "buggy", Bug: bug}
}

// defaults is one of each, as a repository would list them.
func defaults() []workarounds.Workaround {
	return []workarounds.Workaround{
		workarounds.NotAPI(about("ui-routes", "the document includes the web interface's page"), "/"),
		workarounds.UndeclaredAnswers(about("undeclared-answers", "five GETs declare a 200 with no content"), map[string]workarounds.Answer{
			"GET /Things":            workarounds.ListOf("Thing"),
			"GET /Things/{id}/cover": workarounds.File("image/*"),
			"GET /Settings":          workarounds.JSON(),
			"GET /Stats":             workarounds.Shape(&openapi.Schema{Type: openapi.TypeObject, Properties: map[string]*openapi.Schema{"count": {Type: openapi.TypeInteger}}}),
			"GET /Ping":              workarounds.Text("text/plain"),
		}),
		workarounds.WrongAnswers(about("wrong-answers", "three operations declare one answer and give another"), map[string]workarounds.Correction{
			"GET /Path":        {Declared: workarounds.JSONString(), Answers: workarounds.Text("text/plain")},
			"PUT /Things/bulk": workarounds.ListDeclaredAsOne("Thing"),
			"GET /Version":     {Declared: workarounds.File("text/plain"), Answers: workarounds.Text("text/plain")},
		}),
		workarounds.WrongStatus(about("created-accepted", "a create answers 201 and an update 202 where the document says 200"), http.StatusOK, map[int][]string{
			http.StatusCreated:  {"POST /Things"},
			http.StatusAccepted: {"PUT /Things/bulk"},
		}),
		workarounds.UndeclaredParameters(about("things-filter", "GET /Things does not declare the name it filters by"), []string{"GET /Things"}, workarounds.Param{Name: "name", In: openapi.InQuery, Type: openapi.TypeString}),
		workarounds.UndeclaredProperties(about("thing-fields", "a thing is sent with a name the document leaves out"), map[string]map[string]*openapi.Schema{
			"Thing": {"name": {Type: openapi.TypeString, Nullable: true}},
		}),
		workarounds.UndeclaredDatesExcept(about("thing-dates", "a thing's createdAt is declared a plain string; the server writes a date-time, but a note's createdAt is who wrote it"), map[string][]string{"Note": {"createdAt"}}, "createdAt"),
		workarounds.UndeclaredUnixDates(about("thing-stamp", "a thing's stamp is declared an integer; the server writes seconds since 1970"), "seconds", "stamp"),
		workarounds.JoinedLists(about("thing-lists", "kinds and names are declared one string; the server reads each as several joined by commas"), map[string]workarounds.Joined{"kinds": {Of: "Kind", In: []string{"GET /Things"}}, "names": {Of: "string"}}),
		workarounds.UndeclaredEnums(about("thing-status", "a thing's status is declared a plain string; the server writes one of three, where its details' status is free text"), map[string]workarounds.Enum{"status": {Name: "ThingStatus", Values: []string{"new", "done", "gone"}, In: []string{"Thing .status"}}}),
		workarounds.UndeclaredDays(about("thing-days", "a thing's born is declared a plain string; the server writes a day"), "born"),
	}
}

// Each default workaround passes the test one written by hand must, and
// leaves the document saying what the server does.
func TestDefaultWorkarounds(t *testing.T) {
	t.Parallel()

	if err := workarounds.Verify(defaults(), loadBuggy); err != nil {
		t.Fatalf("Verify of the default workarounds = %v", err)
	}

	spec, err := loadBuggy("buggy")
	if err != nil {
		t.Fatal(err)
	}
	var logged []string
	applied, err := workarounds.Apply(defaults(), "buggy", spec, func(msg string) { logged = append(logged, msg) })
	if err != nil || len(applied) != 11 || applied[0] != "buggy-ui-routes" {
		t.Fatalf("Apply = %v, %v", applied, err)
	}
	for _, want := range []string{"buggy: applied workaround buggy-thing-dates: createdAt in 1 place", "buggy: applied workaround buggy-thing-days: born in 1 place", "buggy: applied workaround buggy-thing-status: status as ThingStatus in 1 place", "buggy: applied workaround buggy-ui-routes"} {
		if !slices.Contains(logged, want) {
			t.Errorf("the log lacks %q:\n%s", want, strings.Join(logged, "\n"))
		}
	}
	if thing := spec.Components.Schemas["Thing"].Properties; thing["createdAt"].Format != "date-time" || thing["born"].Format != "date" || thing["seenAt"].Format != "date-time" || thing["stamp"].Format != openapi.FormatUnixSeconds {
		t.Errorf("the dates are not declared: createdAt %q, born %q, seenAt %q, stamp %q", thing["createdAt"].Format, thing["born"].Format, thing["seenAt"].Format, thing["stamp"].Format)
	}

	if spec.Paths["/"] != nil {
		t.Error("the page is still in the document")
	}

	answer := func(method, path, status string) map[string]*openapi.MediaType {
		t.Helper()
		resp := spec.Operation(method, path).Responses[status]
		if resp == nil {
			t.Fatalf("%s %s has no %s", method, path, status)
		}

		return resp.Content
	}
	if s := answer("GET", "/Things", "200")["application/json"].Schema; s.Type != openapi.TypeArray || s.Items.RefName() != "Thing" {
		t.Errorf("GET /Things answers %+v, want a list of Thing", s)
	}
	if status := spec.Components.Schemas["ThingStatus"]; status == nil || !slices.Equal(status.EnumValues(), []string{"new", "done", "gone"}) || spec.Components.Schemas["Thing"].Properties["status"].RefName() != "ThingStatus" {
		t.Errorf("the enum is not declared: %+v, status %+v", status, spec.Components.Schemas["Thing"].Properties["status"])
	}
	if nested := spec.Components.Schemas["Thing"].Properties["details"].Properties["status"]; nested.RefName() != "" || nested.Type != openapi.TypeString {
		t.Errorf("the details' status, outside In, was declared the enum: %+v", nested)
	}
	var kinds, names []string
	for _, prm := range spec.Operation("GET", "/Things").Parameters {
		switch {
		case prm.Name == "kinds" && prm.Added:
			kinds = append(kinds, "list")
			if prm.Schema.Type != openapi.TypeArray || prm.Schema.Items.RefName() != "Kind" || prm.Exploded() || prm.Field != "KindsList" {
				t.Errorf("kinds is not a joined list of Kind beside the string: %+v field %q", prm.Schema, prm.Field)
			}
		case prm.Name == "kinds":
			kinds = append(kinds, prm.Schema.Type)
		case prm.Name == "names" && prm.Added:
			names = append(names, "list")
			if prm.Schema.Type != openapi.TypeArray || prm.Schema.Items.Type != openapi.TypeString || prm.Exploded() || prm.Field != "NamesList" {
				t.Errorf("names is not a joined list of strings beside the string: %+v field %q", prm.Schema, prm.Field)
			}
		case prm.Name == "names":
			names = append(names, prm.Schema.Type)
		}
	}
	if !slices.Equal(kinds, []string{"string", "list"}) || !slices.Equal(names, []string{"string", "list"}) {
		t.Errorf("kinds %v, names %v: the string must stay with the list added", kinds, names)
	}
	if note := spec.Components.Schemas["Note"].Properties; note["createdAt"].Format != "" {
		t.Errorf("the excepted createdAt was declared a date: %q", note["createdAt"].Format)
	}
	if m := answer("GET", "/Things/{id}/cover", "200")["image/*"]; m == nil || m.Text || m.Schema.Format != "binary" {
		t.Errorf("GET /Things/{id}/cover answers %+v, want a file", m)
	}
	if m := answer("GET", "/Settings", "200")["application/json"]; m == nil || m.Schema != nil {
		t.Errorf("GET /Settings answers %+v, want JSON of no shape", m)
	}
	if s := answer("GET", "/Stats", "200")["application/json"].Schema; s == nil || s.Properties["count"] == nil {
		t.Errorf("GET /Stats answers %+v, want the shape given", s)
	}
	for _, path := range []string{"/Ping", "/Path", "/Version"} {
		if content := answer("GET", path, "200"); len(content) != 1 || content["text/plain"] == nil || !content["text/plain"].Text {
			t.Errorf("GET %s answers %+v, want text read whole", path, content)
		}
	}

	// the statuses moved, and the bulk update's one answer is a list under its new one
	if content := answer("POST", "/Things", "201"); spec.Operation("POST", "/Things").Responses["200"] != nil || content["application/json"].Schema.RefName() != "Thing" {
		t.Errorf("POST /Things was not moved to 201: %+v", spec.Operation("POST", "/Things").Responses)
	}
	if content := answer("PUT", "/Things/bulk", "202"); len(content) != 1 || content["application/json"].Schema.Items.RefName() != "Thing" {
		t.Errorf("PUT /Things/bulk answers %+v under 202, want a list of Thing alone", content)
	}

	if p := spec.Operation("GET", "/Things").Parameter(openapi.InQuery, "name"); p == nil || !p.Added {
		t.Errorf("GET /Things was not given name: %+v", p)
	}
	if p := spec.Components.Schemas["Thing"].Properties["name"]; p == nil || !p.Nullable {
		t.Errorf("Thing was not given name: %+v", p)
	}
}

// What a default workaround says when the bug it is for is not in the
// document, or it was written wrong.
func TestDefaultWorkaroundFailures(t *testing.T) {
	t.Parallel()

	a := about("x", "a bug")
	for _, tc := range []struct {
		name string
		w    workarounds.Workaround
		want string
	}{
		{"an answer the document declares now", workarounds.UndeclaredAnswers(a, map[string]workarounds.Answer{"GET /Path": workarounds.JSON(), "POST /Things": workarounds.JSON(), "GET /Ping": workarounds.JSON()}), "these now declare their answer, so take them out: GET /Path, POST /Things"},
		{"an answer of a schema that is not there", workarounds.UndeclaredAnswers(a, map[string]workarounds.Answer{"GET /Things": workarounds.Model("Gone")}), "GET /Things: schema Gone is not in the document"},
		{"a file of no media type", workarounds.UndeclaredAnswers(a, map[string]workarounds.Answer{"GET /Things": workarounds.File("plain")}), `GET /Things: "plain" is not a media type other than JSON's`},
		{"text that is JSON", workarounds.UndeclaredAnswers(a, map[string]workarounds.Answer{"GET /Things": workarounds.Text("application/json")}), `"application/json" is not a media type other than JSON's`},
		{"a shape with no schema", workarounds.UndeclaredAnswers(a, map[string]workarounds.Answer{"GET /Things": workarounds.Shape(nil)}), "GET /Things: a shape with no schema"},
		{"no answer given", workarounds.UndeclaredAnswers(a, map[string]workarounds.Answer{"GET /Things": {}}), "GET /Things: no answer is given"},
		{"an operation that is gone", workarounds.UndeclaredAnswers(a, map[string]workarounds.Answer{"GET /Gone": workarounds.JSON()}), "GET /Gone is not in the document"},
		{"a target that is no method and path", workarounds.UndeclaredAnswers(a, map[string]workarounds.Answer{"/Things": workarounds.JSON()}), `"/Things" is not a method and a path`},
		{"no answers", workarounds.UndeclaredAnswers(a, nil), "it names no operations"},

		{"a correction of what is not declared", workarounds.WrongAnswers(a, map[string]workarounds.Correction{"POST /Things": {Declared: workarounds.JSONString(), Answers: workarounds.Model("Thing")}}), "POST /Things no longer declares a JSON string"},
		{"a correction of a list that is one", workarounds.WrongAnswers(a, map[string]workarounds.Correction{"POST /Things": {Declared: workarounds.ListOf("Thing"), Answers: workarounds.JSON()}}), "POST /Things no longer declares a list of Thing"},
		{"a correction of no answer", workarounds.WrongAnswers(a, map[string]workarounds.Correction{"GET /Things": {Declared: workarounds.JSON(), Answers: workarounds.ListOf("Thing")}}), "GET /Things no longer declares JSON of no declared shape"},
		{"a correction of a file beside JSON", workarounds.WrongAnswers(a, map[string]workarounds.Correction{"PUT /Things/bulk": {Declared: workarounds.File("text/plain"), Answers: workarounds.JSON()}}), "PUT /Things/bulk no longer declares a file (text/plain)"},
		{"a correction that changes nothing", workarounds.WrongAnswers(a, map[string]workarounds.Correction{"POST /Things": {Declared: workarounds.Model("Thing"), Answers: workarounds.Model("Thing")}}), "POST /Things: the correction changes nothing: both are one Thing"},
		{"a correction to a schema that is not there", workarounds.WrongAnswers(a, map[string]workarounds.Correction{"POST /Things": {Declared: workarounds.Model("Thing"), Answers: workarounds.ListOf("Gone")}}), "POST /Things: schema Gone is not in the document"},
		{"no corrections", workarounds.WrongAnswers(a, nil), "it names no operations"},

		{"a status that is not documented", workarounds.WrongStatus(a, http.StatusAccepted, map[int][]string{http.StatusCreated: {"POST /Things"}}), "POST /Things no longer documents a 202 and no 201"},
		{"no statuses", workarounds.WrongStatus(a, http.StatusOK, nil), "it names no operations"},

		{"a parameter that is declared", workarounds.UndeclaredParameters(a, []string{"GET /Things/{id}/cover"}, workarounds.Param{Name: "id", In: openapi.InPath, Type: openapi.TypeInteger}), "GET /Things/{id}/cover already declares path parameter id"},
		{"no parameters", workarounds.UndeclaredParameters(a, []string{"GET /Things"}), "it names no operations or no parameters"},

		{"a date no string property is called", workarounds.UndeclaredDates(a, "createdAt", "updatedAt"), "no string property or parameter is called updatedAt"},
		{"an exception that excludes nothing", workarounds.UndeclaredDatesExcept(a, map[string][]string{"GET /Things": {"createdAt"}}, "createdAt"), "GET /Things has no string called createdAt to leave alone"},
		{"an epoch no integer property is called", workarounds.UndeclaredUnixDates(a, "seconds", "createdAt"), "no integer property or parameter is called createdAt"},
		{"an epoch in no known unit", workarounds.UndeclaredUnixDates(a, "minutes", "stamp"), `unit "minutes" is not seconds or milliseconds`},
		{"a joined list no parameter has", workarounds.JoinedLists(a, map[string]workarounds.Joined{"ghosts": {Of: "string"}}), "no plain string parameter is called ghosts"},
		{"a joined list of what is not an enum", workarounds.JoinedLists(a, map[string]workarounds.Joined{"kinds": {Of: "Thing"}}), `kinds: Of "Thing" is not an enum schema of the document, or "string"`},
		{"a joined list in an operation that has none", workarounds.JoinedLists(a, map[string]workarounds.Joined{"kinds": {Of: "Kind", In: []string{"GET /Stats"}}}), `kinds: In "GET /Stats" matches no operation`},
		{"no joined lists", workarounds.JoinedLists(a, nil), "it names no parameters"},
		{"an enum no string property is called", workarounds.UndeclaredEnums(a, map[string]workarounds.Enum{"mode": {Values: []string{"a"}}}), "no plain string property or parameter is called mode"},
		{"an enum with no values", workarounds.UndeclaredEnums(a, map[string]workarounds.Enum{"status": {}}), "status has no values"},
		{"an enum named for another schema", workarounds.UndeclaredEnums(a, map[string]workarounds.Enum{"status": {Name: "Thing", Values: []string{"a"}}}), "schema Thing exists and is not this enum"},
		{"an enum in a place that has none", workarounds.UndeclaredEnums(a, map[string]workarounds.Enum{"status": {Values: []string{"a"}, In: []string{"Thing nothing.status"}}}), "Thing nothing.status has no plain string called status"},
		{"no enums", workarounds.UndeclaredEnums(a, nil), "it names no properties"},
		{"a date that is declared", workarounds.UndeclaredDates(a, "seenAt"), "every seenAt is declared a date-time now"},
		{"no dates", workarounds.UndeclaredDates(a), "it names no properties"},

		{"a property that is declared", workarounds.UndeclaredProperties(a, map[string]map[string]*openapi.Schema{"Thing": {"id": {Type: openapi.TypeInteger}}}), "Thing declares id now"},
		{"a property of a schema that is not there", workarounds.UndeclaredProperties(a, map[string]map[string]*openapi.Schema{"Gone": {"id": {Type: openapi.TypeInteger}}}), "no schema Gone"},
		{"no properties", workarounds.UndeclaredProperties(a, nil), "it names no properties"},

		{"a path that is not there", workarounds.NotAPI(a, "/", "/login"), "/login is not in the document"},
		{"no paths", workarounds.NotAPI(a), "it names no paths"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec, err := loadBuggy("buggy")
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.w.Apply(spec); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Apply = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// A workaround made here says its name, service and bug as one written by
// hand does.
func TestAbout(t *testing.T) {
	t.Parallel()

	w := workarounds.NotAPI(workarounds.About{Name: "buggy-ui-routes", Service: "buggy", Bug: "the page"}, "/")
	if w.Name() != "buggy-ui-routes" || w.Service() != "buggy" || w.Bug() != "the page" {
		t.Errorf("a default workaround = %s, %s, %s", w.Name(), w.Service(), w.Bug())
	}
}

// A document that declares no component schemas of its own (Dockhand's) can
// still have enums declared into it, at a path inside an inline answer.
func TestUndeclaredEnumsWithoutSchemas(t *testing.T) {
	t.Parallel()

	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Bare", "version": "1"}, "paths": {
    "/stacks": {"get": {"operationId": "GetStacks", "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"type": "object", "properties": {
      "status": {"type": "string"}, "containerDetails": {"type": "array", "items": {"type": "object", "properties": {"status": {"type": "string"}}}}}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	w := workarounds.UndeclaredEnums(workarounds.About{Name: "bare-stack-status", Service: "bare", Bug: "a stack's status is one of three; a container's is free text"}, map[string]workarounds.Enum{"status": {Name: "StackStatus", Values: []string{"running", "stopped", "partial"}, In: []string{"GET /stacks .status"}}})
	if err := w.Apply(spec); err != nil {
		t.Fatal(err)
	}
	rows := spec.Paths["/stacks"].Get.Responses["200"].Content["application/json"].Schema.Items.Properties
	if rows["status"].RefName() != "StackStatus" || rows["containerDetails"].Items.Properties["status"].RefName() != "" {
		t.Errorf("the top status %+v, the nested %+v", rows["status"], rows["containerDetails"].Items.Properties["status"])
	}

	// a property under a map's values is reached too (Dockhand's stack sources, keyed by name)
	spec, err = openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Bare", "version": "1"}, "paths": {
    "/sources": {"get": {"operationId": "GetSources", "responses": {"200": {"content": {"application/json": {"schema": {"type": "object", "additionalProperties": {"type": "object", "properties": {"sourceType": {"type": "string"}}}}}}}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	byName := workarounds.UndeclaredEnums(workarounds.About{Name: "bare-source-type", Service: "bare", Bug: "a source's type"}, map[string]workarounds.Enum{"sourceType": {Values: []string{"internal", "external", "git"}}})
	if err := byName.Apply(spec); err != nil {
		t.Fatal(err)
	}
	values, _ := spec.Paths["/sources"].Get.Responses["200"].Content["application/json"].Schema.Additional()
	if values.Properties["sourceType"].RefName() != "SourceType" {
		t.Errorf("under a map's values: %+v", values.Properties["sourceType"])
	}

	spec, _ = openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Bare", "version": "1"}, "paths": {
    "/stacks": {"get": {"operationId": "GetStacks", "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"type": "object", "properties": {
      "status": {"type": "string"}, "containerDetails": {"type": "array", "items": {"type": "object", "properties": {"status": {"type": "string"}}}}}}}}}}}}}}}`))
	deep := workarounds.UndeclaredEnums(workarounds.About{Name: "bare-container-status", Service: "bare", Bug: "the containers' status"}, map[string]workarounds.Enum{"status": {Name: "ContainerStatus", Values: []string{"up"}, In: []string{"GET /stacks containerDetails.status"}}})
	if err := deep.Apply(spec); err != nil {
		t.Fatal(err)
	}
	rows = spec.Paths["/stacks"].Get.Responses["200"].Content["application/json"].Schema.Items.Properties
	if rows["status"].RefName() != "" || rows["containerDetails"].Items.Properties["status"].RefName() != "ContainerStatus" {
		t.Errorf("by path: the top status %+v, the nested %+v", rows["status"], rows["containerDetails"].Items.Properties["status"])
	}
}

// A path parameter typed as text on one operation and as a number on another of the same path becomes the number; one only ever text stays.
func TestNumberedPathIDs(t *testing.T) {
	t.Parallel()

	spec, err := openapi.Parse([]byte(`{"openapi": "3.0.1", "info": {"title": "Bare", "version": "1"}, "paths": {
    "/tag/{id}": {
      "get": {"operationId": "GetTag", "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "integer", "format": "int32"}}], "responses": {"200": {"description": "ok"}}},
      "put": {"operationId": "PutTag", "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "ok"}}}},
    "/log/{name}": {
      "get": {"operationId": "GetLog", "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "ok"}}},
      "delete": {"operationId": "DeleteLog", "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "gone"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var notes []string
	w := workarounds.NumberedPathIDs(workarounds.About{Name: "bare-put-ids", Service: "bare", Bug: "a PUT's id is text"})
	noting, ok := w.(interface {
		ApplyNoting(*openapi.Spec, func(string)) error
	})
	if !ok {
		t.Fatal("the workaround does not note what it reached")
	}
	if err := noting.ApplyNoting(spec, func(n string) { notes = append(notes, n) }); err != nil {
		t.Fatal(err)
	}
	put := spec.Paths["/tag/{id}"].Put.Parameters[0].Schema
	if put.Type != "integer" || put.Format != "int32" {
		t.Errorf("the PUT's id = %+v, want the GET's integer int32", put)
	}
	if name := spec.Paths["/log/{name}"].Delete.Parameters[0].Schema; name.Type != "string" {
		t.Errorf("a name only ever text = %+v", name)
	}
	if len(notes) != 1 || notes[0] != "the text id of 1 place is the number another operation of the path declares" {
		t.Errorf("notes = %q", notes)
	}

	// applied again, or to a document with no such parameter, it reaches nothing
	if err := w.Apply(spec); err == nil || err.Error() != "no text path parameter has a number of the same name beside it" {
		t.Errorf("a second application: %v", err)
	}
}
