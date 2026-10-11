package sweep_test

import (
	"cmp"
	"context"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/pandorest/client"
	"github.com/katbyte/pandorest/definitions"
	"github.com/katbyte/pandorest/sweep"
)

// item and page are models, as a generated package has them.
type item struct {
	ID   string `json:"Id,omitempty"`
	Name string `json:"Name,omitempty"`
	Seen stamp  `json:"Seen,omitzero"`
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
	Since stamp
}

// stamp is a date in the shape of a generated package's Time.
type stamp struct {
	time.Time
	Unread string
}

func (s *stamp) UnmarshalJSON(data []byte) error {
	parsed, err := client.ParseTime(data, []string{time.RFC3339})
	if unread, ok := errors.AsType[*client.UnreadDateError](err); ok {
		s.Unread = unread.Text

		return nil
	}
	if err != nil {
		return err
	}
	s.Time = parsed

	return nil
}

func (s stamp) UnreadDate() string { return s.Unread }

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
	if !o.Since.IsZero() {
		out.Append("since", o.Since.Format(time.RFC3339))
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

// single is an options struct of one option, which an inlined argument fills.
type single struct{ Term string }

func (single) ToHeaders() *client.Headers { return &client.Headers{} }

func (o single) ToQuery() *client.QueryParams {
	out := client.QueryParams{}
	if o.Term != "" {
		out.Append("term", o.Term)
	}

	return &out
}

func (s sdk) GetLookup(ctx context.Context, term ...string) (result[item], error) {
	var o single
	if len(term) == 1 {
		o.Term = term[0]
	}

	return get[item](ctx, s.c, "/lookup", o)
}

func (s sdk) GetByIDs(ctx context.Context, ids ...string) (result[item], error) {
	return get[item](ctx, s.c, "/byids", single{Term: client.CSV(ids)})
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

func stream(ctx context.Context, c *client.Client, path string) (res file, err error) {
	req, err := c.NewRequest(ctx, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: path, StreamResponse: true})
	if err != nil {
		return res, err
	}
	resp, err := req.Execute(ctx)
	if resp != nil {
		res.HttpResponse = resp.Response
	}

	return res, err
}

func (s sdk) GetFile(ctx context.Context) (file, error) { return stream(ctx, s.c, "/file") }

func (s sdk) GetEvents(ctx context.Context) (file, error) { return stream(ctx, s.c, "/events") }

func (s sdk) GetFeed(ctx context.Context) (file, error) { return stream(ctx, s.c, "/events") }

func (s sdk) GetQuiet(ctx context.Context) (file, error) { return stream(ctx, s.c, "/quiet") }

func (s sdk) GetEnded(ctx context.Context) (file, error) { return stream(ctx, s.c, "/ended") }

func (s sdk) GetDropped(ctx context.Context) (file, error) { return stream(ctx, s.c, "/dropped") }

func (s sdk) GetNoStream(ctx context.Context) (file, error) { return stream(ctx, s.c, "/nostream") }

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
	"/lookup":         {http.StatusOK, `{"Id": "l", "Name": "looked up"}`},
	"/byids":          {http.StatusOK, `{"Id": "b", "Name": "by ids"}`},
	"/gone":           {http.StatusNotFound, "no"},
	"/page":           {http.StatusOK, "<html>"},
	"/lost":           {http.StatusOK, `{"Items": [{"Title": "t", "Year": 1999}], "Total": 1}`},
	"/extra":          {http.StatusOK, `{"Items": [{"Id": "a", "Year": 1999}], "Total": 1, "More": true}`},
	"/file":           {http.StatusOK, "bytes"},
}

// What a stream's server does once it has sent what it has: keep the
// connection open, as an event stream does, end the answer, or drop the
// connection.
const (
	hold = iota
	end
	drop
)

// streams is what the server answers each stream's path with: a content
// type, what it sends at once, and what it does then.
var streams = map[string]struct {
	contentType, body string
	then              int
}{
	"/events":   {"text/event-stream; charset=utf-8", ": connected\n\nevent: tick\ndata: {\"n\": 1}\n\n", hold},
	"/quiet":    {"text/event-stream", ": keep-alive\n\nevent: tick\n\n", hold},
	"/ended":    {"text/event-stream", ": bye\n\n", end},
	"/dropped":  {"text/event-stream", "data: half an ev", drop},
	"/nostream": {"application/json", `{"Id": "a"}`, end},
}

func events() *definitions.Body {
	return &definitions.Body{ContentType: "text/event-stream", Type: definitions.TypeRef{Type: definitions.RawFile}}
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
		{Name: "since", Field: "Since", In: definitions.InQuery, Type: definitions.TypeRef{Type: definitions.DateTime}},
	}
	one := op("GetItem", "/items/{id}", ref("Item"))
	one.PathParameters = []definitions.PathParameter{{Name: "id", Argument: "id", Type: str()}}
	settings := op("GetItemSettings", "/config/items/{id}", ref("Item"))
	settings.PathParameters = []definitions.PathParameter{{Name: "id", Argument: "id", Type: str()}}
	thing := op("GetThing", "/things/{id}", ref("Item"))
	thing.PathParameters = []definitions.PathParameter{{Name: "id", Argument: "id", Type: definitions.TypeRef{Type: definitions.Integer}}}

	lookup := op("GetLookup", "/lookup", ref("Item"))
	lookup.Options = []definitions.Option{{Name: "term", Field: "Term", In: definitions.InQuery, Type: str()}}
	byIDs := op("GetByIDs", "/byids", ref("Item"))
	byIDs.Options = []definitions.Option{{Name: "ids", Field: "IDs", In: definitions.InQuery, Type: definitions.TypeRef{Type: definitions.List, NestedItem: &definitions.TypeRef{Type: definitions.String}}}}

	return &definitions.Service{Name: "mini", Groups: []definitions.Group{{Name: "Items", Operations: []definitions.Operation{
		list, one, settings, thing, lookup, byIDs,
		op("GetNone", "/none", ref("Page")), op("GetGone", "/gone", ref("Item")), op("GetPage", "/page", ref("Item")),
		op("GetLost", "/lost", ref("Page")), op("GetExtra", "/extra", ref("Page")),
		op("GetFile", "/file", &definitions.Body{ContentType: "application/octet-stream", Type: definitions.TypeRef{Type: definitions.RawFile}}),
		op("GetEvents", "/events", events()),
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
		if s, ok := streams[r.URL.Path]; ok {
			w.Header().Set("Content-Type", s.contentType)
			_, _ = io.WriteString(w, s.body)
			_ = http.NewResponseController(w).Flush()

			switch s.then {
			case hold:
				<-r.Context().Done()
			case drop:
				panic(http.ErrAbortHandler)
			}

			return
		}

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

// An event stream never ends, so the sweep reads one as far as its first
// event and hangs up; one that sends none while the sweep waits, or ends
// without one, answered with nothing, and one that is not an event stream at
// all, or breaks off, is a failure.
func TestEventStream(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		op   string
		// documented is the content type the document gives the operation, an event stream unless set
		documented string
		c          sweep.Case
		fail, note string
	}{
		{name: "a stream that sends an event", op: "GetEvents", note: "GET /events: an event stream, read as far as its first event and no further"},
		{name: "a stream its document calls a file", op: "GetFeed", documented: "application/octet-stream", note: "an event stream, read as far as its first event"},
		{name: "a stream that sends no event", op: "GetQuiet", fail: "GET /quiet is an event stream that sent no event in 150ms, which proves only its status"},
		{name: "one that sends none, as its case says", op: "GetQuiet", c: sweep.Case{Empty: "nothing happens on a fresh server"}, note: "answers with nothing, as expected: nothing happens on a fresh server"},
		{name: "one that may send none", op: "GetQuiet", c: sweep.Case{MayBeEmpty: "it follows what the server is doing"}, note: "may answer with nothing (it did: true)"},
		{name: "a stream its case says sends none", op: "GetEvents", c: sweep.Case{Empty: "stale"}, fail: "GET /events now answers with something; drop its Empty case (stale)"},
		{name: "a stream that ends without an event", op: "GetEnded", fail: "GET /ended is an event stream that sent no event"},
		{name: "a stream that breaks off", op: "GetDropped", fail: "GET /dropped: reading the event stream: "},
		{name: "an answer that is no event stream", op: "GetNoStream", fail: "GET /nostream: answers application/json where its document says text/event-stream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s, _ := newSweep(t, served)
			body := events()
			body.ContentType = cmp.Or(tc.documented, body.ContentType)
			s.Definitions.Groups[0].Operations = append(s.Definitions.Groups[0].Operations, definitions.Operation{Name: tc.op, Method: http.MethodGet, Path: "/" + strings.ToLower(strings.TrimPrefix(tc.op, "Get")), Response: body, ExpectedStatusCodes: []int{http.StatusOK}})
			s.Cases = map[string]sweep.Case{tc.op: tc.c}

			// a stream with an event is hung up on as soon as it comes, so only one with none waits this long
			if tc.op != "GetEvents" && tc.op != "GetFeed" {
				s.EventWait = 150 * time.Millisecond
			}

			ops := s.Definitions.Operations()
			out := s.Operation(t.Context(), ops[len(ops)-1])

			switch {
			case tc.fail != "":
				if !strings.Contains(strings.Join(out.Failures, "\n"), tc.fail) {
					t.Errorf("failures = %q, want one containing %q", out.Failures, tc.fail)
				}
			case len(out.Failures) > 0:
				t.Errorf("failures = %q, want a pass", out.Failures)
			case !strings.Contains(strings.Join(out.Notes, "\n"), tc.note):
				t.Errorf("notes = %q, want one containing %q", out.Notes, tc.note)
			}
		})
	}
}

// A client of the grouped layout has each tag's operations on a field named
// for the tag; the sweep finds a method there by the operation's group.
func TestGroupedClient(t *testing.T) {
	t.Parallel()

	s, _ := newSweep(t, served)
	flat, ok := s.Client.(sdk)
	if !ok {
		t.Fatal("the test client is not the fake sdk")
	}
	s.Client = &struct{ Items sdk }{flat}
	if out := s.Operation(t.Context(), operation(t, s, "GetItems")); len(out.Failures) > 0 {
		t.Errorf("a grouped client = %q", out.Failures)
	}
	s.Client = &struct{ Other sdk }{sdk{}}
	if out := s.Operation(t.Context(), operation(t, s, "GetItems")); len(out.Failures) != 1 || !strings.Contains(out.Failures[0], "the client has no such method") {
		t.Errorf("a grouped client without the group = %q", out.Failures)
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
	s.Cases = map[string]sweep.Case{"GetItems": {Options: map[string]any{"Limit": 5, "Exact": false, "Since": "2024-03-04T05:06:07Z"}}}
	s.Definitions.Groups[0].Operations[0].Options[2].Required = true // ids

	if out := s.Operation(t.Context(), operation(t, s, "GetItems")); len(out.Failures) > 0 {
		t.Fatalf("failures = %q", out.Failures)
	}
	if want := "/items?exact=set&ids=1%2C2&limit=set&since=2024-03-04T05%3A06%3A07Z&userId=u1"; !slices.Contains(*queries, want) {
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

// An option taken as an argument (InlineSingleOptions) is filled from the
// case or the fixtures by name as a struct's field is, a list as its
// elements, and left out when nothing names it.
func TestInlinedOption(t *testing.T) {
	t.Parallel()

	s, queries := newSweep(t, served)
	s.Cases = map[string]sweep.Case{"GetLookup": {Options: map[string]any{"Term": "x"}}, "GetByIDs": {Options: map[string]any{"IDs": "1,2"}}}
	for _, name := range []string{"GetLookup", "GetByIDs"} {
		if out := s.Operation(t.Context(), operation(t, s, name)); len(out.Failures) > 0 {
			t.Fatalf("%s: failures = %q", name, out.Failures)
		}
	}
	for _, want := range []string{"/lookup?term=x", "/byids?term=1%2C2"} {
		if !slices.Contains(*queries, want) {
			t.Errorf("the server saw %q, want %s", *queries, want)
		}
	}

	s.Cases = nil
	if out := s.Operation(t.Context(), operation(t, s, "GetLookup")); len(out.Failures) > 0 || !slices.Contains(*queries, "/lookup?") {
		t.Errorf("without a value = %q, the server saw %q", out.Failures, *queries)
	}

	s.Definitions.Groups[0].Operations[4].Options[0].Required = true // term
	if out := s.Operation(t.Context(), operation(t, s, "GetLookup")); len(out.Failures) != 1 || !strings.Contains(out.Failures[0], "no value for required option term") {
		t.Errorf("a required argument nothing supplies = %q", out.Failures)
	}
}

// A date the model kept as text because no layout read it fails the
// operation, naming the text and where it was.
func TestUnreadDate(t *testing.T) {
	t.Parallel()

	replies := maps.Clone(served)
	replies["/items/a"] = struct {
		status int
		body   string
	}{http.StatusOK, `{"Id": "a", "Name": "one", "Seen": "04/03/2024"}`}
	s, _ := newSweep(t, replies)
	if out := s.Operation(t.Context(), operation(t, s, "GetItem")); len(out.Failures) != 1 || !strings.Contains(out.Failures[0], `GET /items/{id} answers a date no layout reads: Model.Seen = "04/03/2024"; add the layout to the service's Dates or Days`) {
		t.Errorf("an unread date = %q", out.Failures)
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
	if len(*queries) != 12 || slices.ContainsFunc(*queries, func(q string) bool { return strings.HasPrefix(q, "/lost") }) {
		t.Errorf("the server saw %d requests: %q", len(*queries), *queries)
	}
}
