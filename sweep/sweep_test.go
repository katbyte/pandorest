package sweep_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/client"
	"github.com/katbyte/pandorest/definitions"
	"github.com/katbyte/pandorest/sweep"
)

// item and page are models, as a generated package has them.
type item struct {
	ID   string `json:"Id,omitempty"`
	Name string `json:"Name,omitempty"`
}

type page struct {
	Items []item `json:"Items,omitzero"`
	Total int    `json:"Total,omitempty"`
}

// result is what an operation that answers JSON returns, and file what one
// that answers a file does.
type result[M any] struct {
	HttpResponse *http.Response //nolint:revive,staticcheck // go-azure-sdk's spelling, which generated code keeps
	Model        *M
}

type file struct {
	HttpResponse *http.Response //nolint:revive,staticcheck // go-azure-sdk's spelling, which generated code keeps
}

// options are an operation's, as generated: a string and a list sent when not
// empty, a number when set.
type options struct {
	User  string
	Limit *int
	IDs   []string
	Exact *bool
}

func (options) ToHeaders() *client.Headers { return &client.Headers{} }

func (o options) ToQuery() *client.QueryParams {
	out := client.QueryParams{}
	if o.User != "" {
		out.Append("userId", o.User)
	}
	if o.Limit != nil {
		out.Append("limit", "set")
	}
	if len(o.IDs) > 0 {
		out.Append("ids", client.CSV(o.IDs))
	}
	if o.Exact != nil {
		out.Append("exact", "set")
	}

	return &out
}

// sdk is a client in the shape of a generated one: a method for each
// operation, taking the context, the path parameters and the options.
type sdk struct{ c *client.Client }

func get[M any](ctx context.Context, c *client.Client, path string, o client.Options) (res result[M], err error) {
	req, err := c.NewRequest(ctx, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: path, OptionsObject: o})
	if err != nil {
		return res, err
	}
	resp, err := req.Execute(ctx)
	if resp != nil {
		res.HttpResponse = resp.Response
	}
	if err != nil {
		return res, err
	}
	var model M
	res.Model = &model

	return res, resp.Unmarshal(res.Model)
}

func (s sdk) GetItems(ctx context.Context, o options) (result[page], error) {
	return get[page](ctx, s.c, "/items", o)
}

func (s sdk) GetItem(ctx context.Context, id string) (result[item], error) {
	return get[item](ctx, s.c, "/items/"+url.PathEscape(id), nil)
}

func (s sdk) GetItemSettings(ctx context.Context, id string) (result[item], error) {
	return get[item](ctx, s.c, "/config/items/"+url.PathEscape(id), nil)
}

func (s sdk) GetThing(ctx context.Context, id int) (result[item], error) {
	return get[item](ctx, s.c, "/things/"+url.PathEscape(strings.Repeat("x", id)), nil)
}

func (s sdk) GetNone(ctx context.Context) (result[page], error) {
	return get[page](ctx, s.c, "/none", nil)
}

func (s sdk) GetGone(ctx context.Context) (result[item], error) {
	return get[item](ctx, s.c, "/gone", nil)
}

func (s sdk) GetPage(ctx context.Context) (result[item], error) {
	return get[item](ctx, s.c, "/page", nil)
}

func (s sdk) GetLost(ctx context.Context) (result[page], error) {
	return get[page](ctx, s.c, "/lost", nil)
}

func (s sdk) GetExtra(ctx context.Context) (result[page], error) {
	return get[page](ctx, s.c, "/extra", nil)
}

func (s sdk) GetFile(ctx context.Context) (res file, err error) {
	req, err := s.c.NewRequest(ctx, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/file", StreamResponse: true})
	if err != nil {
		return res, err
	}
	resp, err := req.Execute(ctx)
	if resp != nil {
		res.HttpResponse = resp.Response
	}

	return res, err
}

func (sdk) GetOdd(context.Context) string { return "not a generated method" }

// answers is what the server answers each path with, a status and a body.
type answers map[string]struct {
	status int
	body   string
}

var served = answers{
	"/items":          {http.StatusOK, `{"Items": [{"Id": "a", "Name": "one"}], "Total": 1}`},
	"/items/a":        {http.StatusOK, `{"Id": "a", "Name": "one"}`},
	"/config/items/1": {http.StatusOK, `{"Id": "1", "Name": "settings"}`},
	"/things/":        {http.StatusOK, `{"Id": "none"}`},
	"/none":           {http.StatusOK, `{"Items": [], "Total": 0}`},
	"/gone":           {http.StatusNotFound, "no"},
	"/page":           {http.StatusOK, "<html>"},
	"/lost":           {http.StatusOK, `{"Items": [{"Title": "t", "Year": 1999}], "Total": 1}`},
	"/extra":          {http.StatusOK, `{"Items": [{"Id": "a", "Year": 1999}], "Total": 1, "More": true}`},
	"/file":           {http.StatusOK, "bytes"},
}

func str() definitions.TypeRef { return definitions.TypeRef{Type: definitions.String} }

func ref(name string) *definitions.Body {
	return &definitions.Body{ContentType: "application/json", Type: definitions.TypeRef{Type: definitions.Reference, ReferenceName: name}}
}

// service is the definitions the fake client was "generated" from.
func service() *definitions.Service {
	op := func(name, path string, response *definitions.Body) definitions.Operation {
		return definitions.Operation{Name: name, Method: http.MethodGet, Path: path, Response: response, ExpectedStatusCodes: []int{http.StatusOK}}
	}
	list := op("GetItems", "/items", ref("Page"))
	list.Options = []definitions.Option{
		{Name: "userId", Field: "User", In: definitions.InQuery, Type: str()},
		{Name: "limit", Field: "Limit", In: definitions.InQuery, Type: definitions.TypeRef{Type: definitions.Integer}},
		{Name: "ids", Field: "IDs", In: definitions.InQuery, Type: definitions.TypeRef{Type: definitions.List, NestedItem: &definitions.TypeRef{Type: definitions.String}}},
		{Name: "exact", Field: "Exact", In: definitions.InQuery, Type: definitions.TypeRef{Type: definitions.Boolean}},
	}
	one := op("GetItem", "/items/{id}", ref("Item"))
	one.PathParameters = []definitions.PathParameter{{Name: "id", Argument: "id", Type: str()}}
	settings := op("GetItemSettings", "/config/items/{id}", ref("Item"))
	settings.PathParameters = []definitions.PathParameter{{Name: "id", Argument: "id", Type: str()}}
	thing := op("GetThing", "/things/{id}", ref("Item"))
	thing.PathParameters = []definitions.PathParameter{{Name: "id", Argument: "id", Type: definitions.TypeRef{Type: definitions.Integer}}}

	return &definitions.Service{Name: "mini", Groups: []definitions.Group{{Name: "Items", Operations: []definitions.Operation{
		list, one, settings, thing,
		op("GetNone", "/none", ref("Page")), op("GetGone", "/gone", ref("Item")), op("GetPage", "/page", ref("Item")),
		op("GetLost", "/lost", ref("Page")), op("GetExtra", "/extra", ref("Page")),
		op("GetFile", "/file", &definitions.Body{ContentType: "application/octet-stream", Type: definitions.TypeRef{Type: definitions.RawFile}}),
		{Name: "DeleteItem", Method: http.MethodDelete, Path: "/items/{id}", ExpectedStatusCodes: []int{http.StatusNoContent}},
	}}}}
}

// newSweep is a sweep of the fake service against a server that answers as
// served says, recording the query each request carried.
func newSweep(t *testing.T, served answers) (s sweep.Sweep, queries *[]string) {
	t.Helper()

	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery)
		a, ok := served[r.URL.Path]
		if !ok {
			a.status, a.body = http.StatusInternalServerError, "nothing is served there"
		}
		w.WriteHeader(a.status)
		_, _ = io.WriteString(w, a.body)
	}))
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL, client.Bearer("t"))
	if err != nil {
		t.Fatal(err)
	}
	c.HTTPClient = srv.Client()

	return sweep.Sweep{
		Definitions: service(), Client: sdk{c}, StatusCode: client.StatusCode,
		Fixtures: sweep.Fixtures{Path: map[string]string{"Items/Id": "a", "config/items/id": "1", "id": "0"}},
	}, &seen
}

func operation(t *testing.T, s sweep.Sweep, name string) *definitions.Operation {
	t.Helper()

	for _, op := range s.Definitions.Operations() {
		if op.Name == name {
			return op
		}
	}
	t.Fatalf("no operation %s", name)

	return nil
}

// What the sweep makes of each kind of answer, with no case and with the
// case that says the answer is the server's to give.
func TestOperation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		op   string
		c    sweep.Case
		// one of: a failure containing fail, a note containing note, a skip, or a pass
		fail, note, skip string
	}{
		{name: "an answer with something in it", op: "GetItems"},
		{name: "a path parameter from the fixtures, by its segment", op: "GetItem"},
		{name: "a path parameter by the segments before it, the most of them first", op: "GetItemSettings"},
		{name: "a numeric path parameter, by its name", op: "GetThing"},
		{name: "a file", op: "GetFile"},

		{name: "an empty answer", op: "GetNone", fail: "GET /none answers with nothing, which decodes into any model"},
		{name: "an empty answer, as its case says", op: "GetNone", c: sweep.Case{Empty: "nothing is there on a fresh server"}, note: "answers with nothing, as expected: nothing is there on a fresh server"},
		{name: "an answer its case says is empty", op: "GetItems", c: sweep.Case{Empty: "stale"}, fail: "GET /items now answers with something; drop its Empty case (stale)"},
		{name: "an answer that may be empty", op: "GetNone", c: sweep.Case{MayBeEmpty: "it follows the image"}, note: "may answer with nothing (it did: true): it follows the image"},

		{name: "an undocumented status", op: "GetGone", fail: "GET /gone: GET /gone: HTTP 404"},
		{name: "a status, as its case says", op: "GetGone", c: sweep.Case{Status: http.StatusNotFound, Why: "gone from the server"}, note: "GET /gone: HTTP 404, as expected: gone from the server"},
		{name: "another status than its case says", op: "GetGone", c: sweep.Case{Status: http.StatusBadRequest, Why: "x"}, fail: "HTTP 404"},
		{name: "an answer its case says fails", op: "GetItems", c: sweep.Case{Status: http.StatusNotFound, Why: "stale"}, fail: "GET /items now answers; drop its case (stale)"},
		{name: "an answer its case says fails sometimes", op: "GetItems", c: sweep.Case{Status: http.StatusBadGateway, Sometimes: true, Why: "the catalogue is fetched"}},

		{name: "an answer that does not decode", op: "GetPage", fail: "GET /page answers what its model cannot decode"},
		{name: "one that does not decode, as its case says", op: "GetPage", c: sweep.Case{Decode: true, Why: "it answers a page"}, note: "does not decode, as expected: it answers a page"},

		{name: "an object lost whole", op: "GetLost", fail: "the model drops what the server sent at the answer.Items[0] (the model has no field for any of Title, Year)"},
		{name: "a key the model has no field for, beside one it has", op: "GetExtra"},

		{name: "a skipped operation", op: "GetGone", c: sweep.Case{Skip: "it would hang"}, skip: "it would hang"},
		{name: "a path parameter from its case", op: "GetItem", c: sweep.Case{Path: map[string]string{"id": "missing"}}, fail: "HTTP 500"},
		{name: "a path parameter that is no number", op: "GetThing", c: sweep.Case{Path: map[string]string{"id": "x"}}, fail: "GET /things/{id}: {id}: "},
		{name: "an option that is no field", op: "GetItems", c: sweep.Case{Options: map[string]any{"Limit": "many"}}, fail: "option limit: cannot set *int"},
		{name: "a method that is not a generated one", op: "GetOdd", fail: "the method does not return a result and an error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, _ := newSweep(t, served)
			s.Definitions.Groups[0].Operations = append(s.Definitions.Groups[0].Operations, definitions.Operation{Name: "GetOdd", Method: http.MethodGet, Path: "/odd"})
			s.Cases = map[string]sweep.Case{tc.op: tc.c}
			out := s.Operation(t.Context(), operation(t, s, tc.op))

			all := strings.Join(out.Failures, "\n")
			switch {
			case tc.skip != "":
				if out.Skipped != tc.skip || len(out.Failures) > 0 {
					t.Errorf("outcome = %+v, want skipped for %q", out, tc.skip)
				}
			case tc.fail != "":
				if !strings.Contains(all, tc.fail) {
					t.Errorf("failures = %q, want one containing %q", out.Failures, tc.fail)
				}
			case len(out.Failures) > 0 || out.Skipped != "":
				t.Errorf("outcome = %+v, want a pass", out)
			case tc.note != "" && !strings.Contains(strings.Join(out.Notes, "\n"), tc.note):
				t.Errorf("notes = %q, want one containing %q", out.Notes, tc.note)
			}
		})
	}
}

// A strict sweep also fails a key the model has no field for; a file and an
// answer the model holds raw have none.
func TestStrict(t *testing.T) {
	t.Parallel()

	s, _ := newSweep(t, served)
	s.Strict = true
	out := s.Operation(t.Context(), operation(t, s, "GetExtra"))
	if len(out.Failures) != 1 || out.Failures[0] != "GET /extra answers fields the document does not declare: item.Year, page.More" {
		t.Errorf("a strict sweep of an answer with more in it = %q", out.Failures)
	}
	for _, name := range []string{"GetItems", "GetFile"} {
		if out := s.Operation(t.Context(), operation(t, s, name)); len(out.Failures) > 0 {
			t.Errorf("a strict sweep of %s = %q", name, out.Failures)
		}
	}
}

// Options come from the case, from the fixtures' Always whenever the
// operation has them, and from the fixtures' Options when the document says
// the operation needs them; one it needs and nothing supplies is a failure.
func TestOptions(t *testing.T) {
	t.Parallel()

	s, queries := newSweep(t, served)
	s.Fixtures.Always = map[string]string{"UserID": "u1"}
	s.Fixtures.Options = map[string]string{"ids": "1,2"}
	s.Cases = map[string]sweep.Case{"GetItems": {Options: map[string]any{"Limit": 5, "Exact": false}}}
	s.Definitions.Groups[0].Operations[0].Options[2].Required = true // ids

	if out := s.Operation(t.Context(), operation(t, s, "GetItems")); len(out.Failures) > 0 {
		t.Fatalf("failures = %q", out.Failures)
	}
	if want := "/items?exact=set&ids=1%2C2&limit=set&userId=u1"; !slices.Contains(*queries, want) {
		t.Errorf("the server saw %q, want %s", *queries, want)
	}

	s.Fixtures.Options = nil
	if out := s.Operation(t.Context(), operation(t, s, "GetItems")); len(out.Failures) != 1 || !strings.Contains(out.Failures[0], "GET /items: no value for required option ids; resolve it in the fixtures or give it a case") {
		t.Errorf("a required option nothing supplies = %q", out.Failures)
	}

	s.Fixtures.Path = nil
	if out := s.Operation(t.Context(), operation(t, s, "GetItem")); len(out.Failures) != 1 || !strings.Contains(out.Failures[0], "no value for path parameter {id}") {
		t.Errorf("a path parameter nothing supplies = %q", out.Failures)
	}
}

// A case for an operation that is gone, renamed or not a GET is stale.
func TestStaleCases(t *testing.T) {
	t.Parallel()

	s, _ := newSweep(t, served)
	s.Cases = map[string]sweep.Case{"GetItems": {}, "GetRenamed": {Skip: "x"}, "DeleteItem": {Skip: "x"}}
	if got := s.StaleCases(); !slices.Equal(got, []string{"DeleteItem", "GetRenamed"}) {
		t.Errorf("StaleCases = %v", got)
	}
}

// Run sweeps every GET as a subtest: a service whose every operation answers
// or has a case that says why not passes.
func TestRun(t *testing.T) {
	t.Parallel()

	s, queries := newSweep(t, served)
	s.Cases = map[string]sweep.Case{
		"GetNone": {Empty: "nothing is there on a fresh server"},
		"GetGone": {Status: http.StatusNotFound, Why: "gone from the server"},
		"GetPage": {Decode: true, Why: "it answers a page"},
		"GetLost": {Skip: "its model is wrong, and a workaround is on its way"},
	}
	s.Run(t)

	// every GET but the skipped one was called, and the DELETE was not
	if len(*queries) != 9 || slices.ContainsFunc(*queries, func(q string) bool { return strings.HasPrefix(q, "/lost") }) {
		t.Errorf("the server saw %d requests: %q", len(*queries), *queries)
	}
}
