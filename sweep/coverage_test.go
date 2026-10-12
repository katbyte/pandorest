package sweep

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/katbyte/pandorest/client"
	"github.com/katbyte/pandorest/definitions"
)

type state string

func (s state) Known() bool { return s == "" || s == "up" || s == "down" }

type when struct{ time.Time }

type box struct {
	State  state `json:"state,omitempty"`
	Seen   when  `json:"seen,omitzero"`
	Open   bool  `json:"open"`
	Locked *bool `json:"locked,omitempty"`
	Inner  *box  `json:"inner,omitempty"`
	Extra  []box `json:"extra,omitzero"`
	ByName map[string]box
}

// The places an answer can hold an enum, a date or a boolean by value are found through the answer type once, a value marks the
// place it came for, and the notes name what no answer carried.
func TestCoverage(t *testing.T) {
	t.Parallel()

	c := &coverage{places: map[string]placeKind{}, enums: map[string]string{}, listed: map[string][]string{"state": {"up", "down", "gone"}}, seen: map[string]bool{}, values: map[string]map[string]bool{}}
	c.walkType(reflect.TypeFor[*box](), map[reflect.Type]bool{})
	if want := map[string]placeKind{"box.State": enumPlace, "box.Seen": datePlace, "box.Open": boolPlace}; !reflect.DeepEqual(c.places, want) {
		t.Errorf("places = %v, want %v", c.places, want)
	}
	if len(c.notes()) != 3 || !strings.Contains(c.notes()[0], "enum state: 1 place, a value seen at 0") {
		t.Errorf("notes before any answer = %q", c.notes())
	}

	var model box
	raw := `{"state": "up", "open": false, "locked": true, "inner": {"state": "", "seen": "2024-01-02T03:04:05Z"}, "extra": [{"state": "down"}], "ByName": {"a": {"open": true}}}`
	if err := json.Unmarshal([]byte(raw), &model); err != nil {
		t.Fatal(err)
	}
	var sent any
	_ = json.Unmarshal([]byte(raw), &sent)
	c.observe(sent, reflect.ValueOf(&model))
	notes := strings.Join(c.notes(), "\n")
	for _, want := range []string{"enum state: 1 place, a value seen at 1; listed values never seen: gone", "dates: 1 place, a value seen at 1", "booleans held by value: 1 place, carried by an answer at 1"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q:\n%s", want, notes)
		}
	}
	if !c.values["state"]["down"] {
		t.Error("a value under a list was not seen")
	}

	// a nil coverage observes nothing, which is what Operation called on its own does
	var none *coverage
	none.observe(sent, reflect.ValueOf(&model))
}

// A list of names is cut after maxNamed, and the report counts each outcome once.
func TestCoverageWording(t *testing.T) {
	t.Parallel()

	names := make([]string, maxNamed+2)
	for i := range names {
		names[i] = "n"
	}
	if got := named(names); !strings.HasSuffix(got, "n and 2 more") {
		t.Errorf("named = %q", got)
	}

	var r Report
	for _, out := range []Outcome{{}, {Skipped: "x"}, {Failures: []string{"f"}}, {empty: true}, {refused: true}} {
		r.add(out)
	}
	if got := r.Summary("mini"); got != "mini: 5 GET operations: 1 answered, 1 answered nothing by their Empty case, 1 refused as their case says, 1 skipped, 1 failed" {
		t.Errorf("Summary = %q", got)
	}
}

// An operation none of the recorded calls reached is a problem unless excused, an excuse naming no operation is one too, and a
// call's path matches its template through placeholders and under a base path.
func TestNeverCalled(t *testing.T) {
	t.Parallel()

	defs := &definitions.Service{Name: "mini", Groups: []definitions.Group{{Name: "Items", Operations: []definitions.Operation{
		{Name: "GetItems", Method: "GET", Path: "/api/v3/items"},
		{Name: "GetItem", Method: "GET", Path: "/api/v3/items/{id}"},
		{Name: "GetItemLookup", Method: "GET", Path: "/api/v3/items/lookup"},
		{Name: "DeleteItem", Method: "DELETE", Path: "/api/v3/items/{id}"},
		{Name: "GetTags", Method: "GET", Path: "/api/v3/tags"},
	}}}}
	// a call a generated method sent names its operation; a hand-written one goes to the most literal template it fits, so the
	// lookup under /items does not count for GetItem
	calls := []client.Call{{Method: "GET", Path: "/sonarr/api/v3/items", Operation: "GetItems"}, {Method: "GET", Path: "/api/v3/items/lookup"}, {Method: "GET", Path: "/api/v3/tags/1"}}
	got := NeverCalled(defs, calls, map[string]string{"GetTags": "needs a tag", "GetGone": "no such thing"})
	want := []string{"GetItem (GET /api/v3/items/{id}) was never called, and is not excused", "DeleteItem (DELETE /api/v3/items/{id}) was never called, and is not excused", "the excuse for GetGone names no operation"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NeverCalled = %q, want %q", got, want)
	}
	// an excused operation the run called after all is a stale excuse
	stale := NeverCalled(defs, append(calls, client.Call{Method: "GET", Path: "/api/v3/items/12"}), map[string]string{"GetItem": "needs an item", "DeleteItem": "never in a read suite", "GetTags": "needs a tag"})
	if want := []string{"GetItem (GET /api/v3/items/{id}) is excused but was called, so drop its excuse (needs an item)"}; !reflect.DeepEqual(stale, want) {
		t.Errorf("a stale excuse = %q, want %q", stale, want)
	}
	if got := NeverCalled(defs, append(calls, client.Call{Method: "GET", Path: "/api/v3/items/12"}), map[string]string{"DeleteItem": "never in a read suite", "GetTags": "needs a tag"}); len(got) != 0 {
		t.Errorf("a run that reached or excused everything = %q", got)
	}
}
