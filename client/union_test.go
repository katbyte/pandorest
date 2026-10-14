package client_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/client"
)

// A union holds a record as it came, says what kind it names itself, hands it over as the member asked for and refuses
// another; without a field to tell them apart a member of another shape is refused by its keys.
func TestUnion(t *testing.T) {
	t.Parallel()

	type s3 struct {
		Type   string `json:"type"`
		Bucket string `json:"bucket"`
	}
	type b2 struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	var u client.Union
	if err := json.Unmarshal([]byte(`{"type": "S3", "bucket": "b"}`), &u); err != nil {
		t.Fatal(err)
	}
	if u.Kind("type") != "S3" || u.Kind("missing") != "" {
		t.Errorf("Kind = %q", u.Kind("type"))
	}
	if got, err := client.UnionAs[s3](u, "type", "S3"); err != nil || got.Bucket != "b" {
		t.Errorf("as S3 = %+v, %v", got, err)
	}
	if _, err := client.UnionAs[b2](u, "type", "B2"); err == nil || !strings.Contains(err.Error(), `"S3", not a "B2"`) {
		t.Errorf("as B2 = %v", err)
	}
	if _, err := client.UnionAs[b2](u, "", ""); err == nil {
		t.Error("a record of another shape decoded without a kind to go by")
	}
	if got, err := client.UnionAs[s3](u, "", ""); err != nil || got.Bucket != "b" {
		t.Errorf("as S3 by shape = %+v, %v", got, err)
	}

	out, err := json.Marshal(u)
	if err != nil || string(out) != `{"type":"S3","bucket":"b"}` {
		t.Errorf("written = %s, %v", out, err)
	}
	if out, err := json.Marshal(client.Union{}); err != nil || string(out) != "null" {
		t.Errorf("an empty union = %s, %v", out, err)
	}
	if _, err := client.UnionAs[s3](client.Union{}, "type", "S3"); err == nil {
		t.Error("an empty union handed a record over")
	}

	made, err := client.UnionOf(b2{Type: "B2", Key: "k"})
	if err != nil || made.Kind("type") != "B2" {
		t.Errorf("UnionOf = %+v, %v", made, err)
	}
}
