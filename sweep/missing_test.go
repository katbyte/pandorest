package sweep

import (
	"net/http"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/definitions"
)

// What a missing id got is held to what its Missing wants: the same answer passes with a note, another fails naming both, and
// no want only notes.
func TestMissingVerdict(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		got     string
		want    string
		note    string
		failure string
	}{
		"the status wanted":      {got: "404", want: "404", note: "a missing ItemID answers HTTP 404 (GET /items/{id})"},
		"another status":         {got: "500", want: "404", note: "a missing ItemID answers HTTP 500 (GET /items/{id})", failure: "a missing ItemID answers HTTP 500 (GET /items/{id}), want HTTP 404"},
		"nothing wanted":         {got: "nothing", want: "nothing", note: "a missing ItemID answers nothing (GET /items/{id})"},
		"something, not nothing": {got: "something", want: "nothing", note: "a missing ItemID answers something (GET /items/{id})", failure: "a missing ItemID answers something (GET /items/{id}), want nothing"},
		"no want":                {got: "500", note: "a missing ItemID answers HTTP 500 (GET /items/{id})"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			note, failure := missingVerdict("ItemID", "GET /items/{id}", tc.got, Missing{ID: "zz", Want: tc.want})
			if note != tc.note || failure != tc.failure {
				t.Errorf("missingVerdict = %q, %q; want %q, %q", note, failure, tc.note, tc.failure)
			}
		})
	}
}

// A Missing naming an operation that is not a GET taking its kind in the path fails the probe before any call is made.
func TestMissingNamesNoGET(t *testing.T) {
	t.Parallel()

	s := Sweep{Definitions: &definitions.Service{Name: "mini", Groups: []definitions.Group{{Name: "Items", Operations: []definitions.Operation{
		{Name: "GetItem", Method: http.MethodGet, Path: "/items/{id}", PathParameters: []definitions.PathParameter{{Name: "id", Type: definitions.TypeRef{Type: definitions.ID, ReferenceName: "ItemID"}}}},
		{Name: "DeleteItem", Method: http.MethodDelete, Path: "/items/{id}", PathParameters: []definitions.PathParameter{{Name: "id", Type: definitions.TypeRef{Type: definitions.ID, ReferenceName: "ItemID"}}}},
	}}}}}
	for name, m := range map[string]Missing{"a name no operation has": {ID: "zz", Operation: "GetGhost"}, "a DELETE": {ID: "zz", Operation: "DeleteItem"}} {
		if note, failure := s.probeMissing(t.Context(), "ItemID", m); note != "" || !strings.Contains(failure, m.Operation+" is no GET taking a ItemID in its path") {
			t.Errorf("%s: probeMissing = %q, %q", name, note, failure)
		}
	}
}
