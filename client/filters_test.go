package client_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/client"
)

// Filters are written out as the server reads them: a list of conditions, each a field, a comparison and a value, and a choice
// among conditions under OR; none is nil, which a call sends as an empty list.
func TestFilters(t *testing.T) {
	t.Parallel()

	filters := client.Filters(client.Where("name", "=", "tank"), client.AnyOf(client.Where("size", ">", 10), client.Where("status", "in", []string{"ONLINE", "DEGRADED"})))
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	err := enc.Encode(filters)
	if want := `[["name","=","tank"],["OR",[["size",">",10],["status","in",["ONLINE","DEGRADED"]]]]]`; err != nil || strings.TrimSpace(out.String()) != want {
		t.Errorf("filters = %s, %v\n want %s", out.String(), err, want)
	}
	if client.Filters() != nil {
		t.Error("no conditions is not nil")
	}
	if out, _ := json.Marshal(client.CallParams(1, client.Filters())); string(out) != "[[]]" {
		t.Errorf("no conditions sent = %s", out)
	}
}
