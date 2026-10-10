package workarounds_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/importer/workarounds"
	"github.com/katbyte/pandorest/openapi"
)

// document has the two bugs the workarounds below fix: GET /Items takes a
// searchTerm it does not declare, and GET /Things declares no answer.
const document = `{"openapi": "3.0.1", "info": {"title": "Mini", "version": "1"}, "paths": {
  "/Items": {"get": {"operationId": "GetItems", "tags": ["Items"], "parameters": [{"name": "limit", "in": "query", "schema": {"type": "integer"}}],
    "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Item"}}}}}}},
  "/Things": {"get": {"operationId": "GetThings", "tags": ["Items"], "responses": {"200": {"content": {"application/json": {}}}}}}},
  "components": {"schemas": {"Item": {"type": "object", "properties": {"Id": {"type": "string"}}}}}}`

func load(string) (*openapi.Spec, error) { return openapi.Parse([]byte(document)) }

// fake is a workaround made of its parts.
type fake struct {
	name, service, bug string
	apply              func(*openapi.Spec) error
}

func (f fake) Name() string                   { return f.name }
func (f fake) Service() string                { return f.service }
func (f fake) Bug() string                    { return f.bug }
func (f fake) Apply(spec *openapi.Spec) error { return f.apply(spec) }

// searchTerm declares the parameter GET /Items takes, and fails once the
// document declares it.
var searchTerm = fake{"mini-search-term", "mini", "GET /Items does not declare searchTerm; the server filters by it", func(spec *openapi.Spec) error {
	return workarounds.AddParameters(spec, []string{"GET /Items"}, workarounds.Param{Name: "searchTerm", In: openapi.InQuery, Type: openapi.TypeString, Description: "what to look for"})
}}

// thingsAnswer says what GET /Things answers, and fails once the document does.
var thingsAnswer = fake{"mini-things-answer", "mini", "GET /Things declares no schema; the server answers an Item", func(spec *openapi.Spec) error {
	op, err := workarounds.Operation(spec, "GET", "/Things")
	if err != nil {
		return err
	}
	media, err := workarounds.JSONResponse(op, "GET /Things")
	if err != nil {
		return err
	}
	if media.Schema != nil {
		return errors.New("it declares its answer")
	}
	media.Schema = &openapi.Schema{Ref: openapi.SchemaRefPrefix + "Item"}

	return nil
}}

func TestApply(t *testing.T) {
	t.Parallel()

	all := []workarounds.Workaround{searchTerm, fake{"other-fix", "other", "another service's", func(*openapi.Spec) error { return errors.New("never applied") }}, thingsAnswer}
	spec, err := load("mini")
	if err != nil {
		t.Fatal(err)
	}
	var logged []string
	applied, err := workarounds.Apply(all, "mini", spec, func(s string) { logged = append(logged, s) })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mini-search-term", "mini-things-answer"}
	if !slices.Equal(applied, want) || !slices.Equal(logged, []string{"mini: applied workaround mini-search-term", "mini: applied workaround mini-things-answer"}) {
		t.Errorf("applied %v (logged %v), want %v", applied, logged, want)
	}
	if p := spec.Operation("GET", "/Items").Parameter(openapi.InQuery, "searchTerm"); p == nil || p.Schema.Type != openapi.TypeString || p.Description != "what to look for" {
		t.Errorf("searchTerm was not added: %+v", p)
	}
	if got := spec.Operation("GET", "/Things").Responses["200"].Content["application/json"].Schema.RefName(); got != "Item" {
		t.Errorf("GET /Things answers %q, want Item", got)
	}

	// a second pass over the patched document fails and names the workaround
	_, err = workarounds.Apply(all, "mini", spec, nil)
	if err == nil || !strings.Contains(err.Error(), "workaround mini-search-term no longer applies, so remove it") || !strings.Contains(err.Error(), "(it was for: GET /Items does not declare searchTerm") {
		t.Errorf("second Apply = %v", err)
	}

	if applied, err := workarounds.Apply(all, "nothing", spec, nil); err != nil || applied == nil || len(applied) != 0 {
		t.Errorf("Apply for a service without workarounds = %v, %v", applied, err)
	}
}

func TestVerify(t *testing.T) {
	t.Parallel()

	if err := workarounds.Verify([]workarounds.Workaround{searchTerm, thingsAnswer}, load); err != nil {
		t.Errorf("Verify of two sound workarounds = %v", err)
	}

	noop := func(*openapi.Spec) error { return nil }
	for _, tc := range []struct {
		name string
		all  []workarounds.Workaround
		want string
	}{
		{"a name twice", []workarounds.Workaround{searchTerm, searchTerm}, "two workarounds are named mini-search-term"},
		{"no bug", []workarounds.Workaround{fake{"mini-x", "mini", "", noop}}, "mini-x: its name or its bug \"\" does not describe it"},
		{"not named for its service", []workarounds.Workaround{fake{"search-term", "mini", "a bug", noop}}, "search-term: its name or its bug"},
		{"the bug is not there", []workarounds.Workaround{fake{"mini-gone", "mini", "a bug", func(spec *openapi.Spec) error {
			_, err := workarounds.Operation(spec, "GET", "/Gone")
			return err
		}}}, "mini-gone: it does not apply to the vendored document, so the bug is not there or the workaround is written wrong: GET /Gone is not in the document"},
		{"would not notice a fix", []workarounds.Workaround{fake{"mini-blind", "mini", "a bug", noop}}, "mini-blind: applying it again succeeded, so it would not notice the bug being fixed"},
		{"panics without a 200", []workarounds.Workaround{fake{"mini-panics", "mini", "a bug", func(spec *openapi.Spec) error {
			media := spec.Operation("GET", "/Things").Responses["200"].Content["application/json"]
			if media.Schema != nil {
				return errors.New("it declares its answer")
			}
			media.Schema = &openapi.Schema{}

			return nil
		}}}, "mini-panics: it panicked on a document with no 200 responses"},
		{"no document", []workarounds.Workaround{fake{"plex-x", "plex", "a bug", noop}}, "plex-x: loading the plex document: no plex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := workarounds.Verify(tc.all, func(service string) (*openapi.Spec, error) {
				if service != "mini" {
					return nil, errors.New("no " + service)
				}

				return load(service)
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Verify = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestHelpers(t *testing.T) {
	t.Parallel()

	spec, err := load("mini")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := workarounds.Operation(spec, "POST", "/Items"); err == nil || err.Error() != "POST /Items is not in the document" {
		t.Errorf("Operation for one not there = %v", err)
	}

	limit := workarounds.Param{Name: "limit", In: openapi.InQuery, Type: openapi.TypeInteger}
	if err := workarounds.AddParameters(spec, []string{"GET /Items"}, limit); err == nil || err.Error() != "GET /Items already declares query parameter limit" {
		t.Errorf("AddParameters of one already declared = %v", err)
	}
	if err := workarounds.AddParameters(spec, []string{"GET /Gone"}, limit); err == nil {
		t.Error("AddParameters to an operation not there succeeded")
	}

	if p, err := workarounds.Property(spec, "Item", "Id"); err != nil || p.Type != openapi.TypeString {
		t.Errorf("Property = %+v, %v", p, err)
	}
	if _, err := workarounds.Property(spec, "Gone", "Id"); err == nil || err.Error() != "no schema Gone" {
		t.Errorf("Property of no schema = %v", err)
	}
	if _, err := workarounds.Property(spec, "Item", "Gone"); err == nil || err.Error() != "schema Item has no property Gone" {
		t.Errorf("Property not there = %v", err)
	}

	items := spec.Operation("GET", "/Items")
	if media, err := workarounds.JSONResponse(items, "GET /Items"); err != nil || media.Schema.RefName() != "Item" {
		t.Errorf("JSONResponse = %+v, %v", media, err)
	}
	items.Responses["200"].Content = map[string]*openapi.MediaType{"text/plain": {}}
	if _, err := workarounds.JSONResponse(items, "GET /Items"); err == nil || err.Error() != "GET /Items no longer answers JSON" {
		t.Errorf("JSONResponse without JSON = %v", err)
	}
	delete(items.Responses, "200")
	if _, err := workarounds.JSONResponse(items, "GET /Items"); err == nil || err.Error() != "GET /Items has no 200 response" {
		t.Errorf("JSONResponse without a 200 = %v", err)
	}
}
