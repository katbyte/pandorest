package client_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/client"
)

// A raw answer decodes into a local type; nothing, and a key the strict form
// has no field for, are errors that say so.
func TestDecode(t *testing.T) {
	t.Parallel()

	type job struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	raw := json.RawMessage(`{"id": "j1", "state": "done", "extra": true}`)
	j, err := client.Decode[job](raw)
	if err != nil || j.ID != "j1" || j.State != "done" {
		t.Errorf("Decode = %+v, %v", j, err)
	}
	if _, err := client.DecodeStrict[job](raw); err == nil || !strings.Contains(err.Error(), `"extra"`) {
		t.Errorf("DecodeStrict of a key the type lacks = %v", err)
	}
	if j, err := client.DecodeStrict[job](json.RawMessage(`{"id": "j2"}`)); err != nil || j.ID != "j2" {
		t.Errorf("DecodeStrict = %+v, %v", j, err)
	}
	if _, err := client.Decode[job](nil); err == nil || err.Error() != "no answer to decode" {
		t.Errorf("Decode of nothing = %v", err)
	}
	if _, err := client.Decode[job](json.RawMessage(`[1]`)); err == nil || !strings.Contains(err.Error(), "client_test.job") {
		t.Errorf("Decode of the wrong shape = %v", err)
	}
	if list, err := client.Decode[[]job](json.RawMessage(`[{"id": "a"}]`)); err != nil || len(list) != 1 {
		t.Errorf("Decode of a list = %v, %v", list, err)
	}
}
