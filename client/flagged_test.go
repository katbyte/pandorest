package client_test

import (
	"encoding/json"
	"testing"

	"github.com/katbyte/pandorest/client"
)

// A flagged record goes out with the flag set beside what the caller set, and a record left out goes as the flag alone.
func TestFlagged(t *testing.T) {
	t.Parallel()

	type options struct {
		Limit int      `json:"limit,omitempty"`
		Order []string `json:"order_by,omitzero"`
	}
	for _, tc := range []struct {
		name   string
		record any
		want   string
	}{
		{"a record with a field set", options{Limit: 5}, `{"get":true,"limit":5}`},
		{"a zero record", options{}, `{"get":true}`},
		{"no record", nil, `{"get":true}`},
		{"a map", map[string]any{"limit": 2}, `{"get":true,"limit":2}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := json.Marshal(client.Flagged{Record: tc.record, Flag: "get"})
			if err != nil || string(out) != tc.want {
				t.Errorf("written = %s, %v, want %s", out, err, tc.want)
			}
		})
	}

	if _, err := json.Marshal(client.Flagged{Record: []int{1}, Flag: "count"}); err == nil {
		t.Error("a list took a flag")
	}
}
