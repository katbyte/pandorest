package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/katbyte/pandorest/client"
)

type record struct {
	Name string `json:"name,omitempty"`
}

// CallParams sends every required argument whatever its value, and after them only up to the last one set: an empty record,
// list or map, a zero number, "" or false at the end is left off, and none at all is nil.
// A list or map sent unset goes as an empty one, since a server that asks for a list reads null as none; a list sent empty
// or set, and every other value, goes as it is.
func TestCallParamsEmptied(t *testing.T) {
	t.Parallel()

	got := client.CallParams(1, []any(nil), map[string]int(nil), record{Name: "n"})
	out, err := json.Marshal(got)
	if err != nil || string(out) != `[[],{},{"name":"n"}]` {
		t.Errorf("sent = %s, %v", out, err)
	}
	if got := client.CallParams(1, []any(nil)); len(got) != 1 || got[0] == nil {
		t.Errorf("a required nil list = %#v", got)
	}
	if got := client.CallParams(0, []string{"x"}, (*record)(nil), 0); len(got) != 1 || fmt.Sprint(got[0]) != "[x]" {
		t.Errorf("a set list = %#v", got)
	}
}

func TestCallParams(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		required int
		args     []any
		want     int
	}{
		"none":                      {0, nil, 0},
		"all set":                   {0, []any{[]any{"x"}, record{Name: "n"}}, 2},
		"trailing empty record":     {0, []any{[]any{"x"}, record{}}, 1},
		"trailing empty list":       {0, []any{[]any{"x"}, []string{}}, 1},
		"trailing nil":              {0, []any{"x", nil}, 1},
		"all empty":                 {0, []any{[]any(nil), record{}}, 0},
		"required zero kept":        {2, []any{"", 0, false}, 2},
		"zero in the middle kept":   {0, []any{0, "", true}, 3},
		"a pointer to nothing":      {0, []any{"x", (*record)(nil)}, 1},
		"a pointer to an empty one": {0, []any{"x", &record{}}, 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := client.CallParams(tc.required, tc.args...)
			if len(got) != tc.want || (tc.want == 0 && got != nil) {
				t.Errorf("CallParams = %#v, want %d", got, tc.want)
			}
		})
	}
	// what is sent is a copy the caller cannot grow into
	args := []any{"a", "b"}
	sent := client.CallParams(1, args[0], args[1])
	if cap(sent) != 2 {
		t.Errorf("cap = %d", cap(sent))
	}
}

// A refused call names its method and says why in the server's words, and is told from any other error and by its name.
func TestCallError(t *testing.T) {
	t.Parallel()

	refusal := &client.CallError{Method: "pool.query", Code: -32001, Message: "Method call error", Errno: 2, ErrName: "ENOENT", Reason: "Pool tank not found"}
	wrapped := fmt.Errorf("asking: %w", refusal)
	if got := refusal.Error(); got != "pool.query: Pool tank not found (ENOENT)" {
		t.Errorf("Error = %q", got)
	}
	if got := (&client.CallError{Code: -32601, Message: "Method not found"}).Error(); got != "Method not found" {
		t.Errorf("Error without a reason = %q", got)
	}
	if got := (&client.CallError{Method: "x.y", Code: -32000}).Error(); got != "x.y: the server refused the call (code -32000)" {
		t.Errorf("Error with nothing said = %q", got)
	}
	if client.CallErrorOf(wrapped) != refusal || client.CallErrorOf(errors.New("gone")) != nil {
		t.Error("CallErrorOf did not find the refusal in the chain, or found one where there is none")
	}
	if !client.IsRefusedCall(wrapped) || !client.IsRefusedCall(wrapped, "enoent") || client.IsRefusedCall(wrapped, "EINVAL") || client.IsRefusedCall(errors.New("gone")) {
		t.Error("IsRefusedCall does not tell a refusal by name")
	}
	if !client.IsNotFound(wrapped) || client.IsNotFound(&client.CallError{ErrName: "EINVAL"}) {
		t.Error("IsNotFound does not read ENOENT")
	}
	var fn client.Caller = client.CallerFunc(func(context.Context, string, []any, any) error { return refusal })
	if err := fn.Call(t.Context(), "x", nil, nil); !errors.Is(err, refusal) {
		t.Errorf("CallerFunc = %v", err)
	}
}

// A date written as {"$date": milliseconds} is read when the layouts name that form, written so when it is the first, and
// has its milliseconds as its text, which has no other form.
func TestDateObjectMillis(t *testing.T) {
	t.Parallel()

	layouts := []string{client.DateObjectMillis, time.RFC3339}
	at := time.Date(2026, 8, 31, 18, 33, 14, 0, time.UTC)
	got, err := client.ParseTime([]byte(`{"$date": 1788201194000}`), layouts)
	if err != nil || !got.Equal(at) {
		t.Errorf("ParseTime = %v, %v", got, err)
	}
	if got, err := client.ParseTime([]byte(`"2026-08-31T18:33:14Z"`), layouts); err != nil || !got.Equal(at) {
		t.Errorf("ParseTime of the text form = %v, %v", got, err)
	}
	if got, err := client.ParseTime([]byte(`{"$date": 0}`), layouts); err != nil || !got.IsZero() {
		t.Errorf("ParseTime of 0 = %v, %v", got, err)
	}
	if _, err := client.ParseTime([]byte(`{"date": 1}`), layouts); err == nil {
		t.Error("an object that is no date was read")
	}
	if _, err := client.ParseTime([]byte(`{"$date": 1}`), []string{time.RFC3339}); err == nil {
		t.Error("the object form was read by a layout that does not name it")
	}
	if data, err := client.MarshalTime(at, client.DateObjectMillis); err != nil || string(data) != `{"$date":1788201194000}` {
		t.Errorf("MarshalTime = %s, %v", data, err)
	}
	if data, err := client.MarshalTime(at, time.RFC3339); err != nil || string(data) != `"2026-08-31T18:33:14Z"` {
		t.Errorf("MarshalTime of the text form = %s, %v", data, err)
	}
	if got := client.FormatTime(at, client.DateObjectMillis); got != "1788201194000" {
		t.Errorf("FormatTime = %q", got)
	}
}
