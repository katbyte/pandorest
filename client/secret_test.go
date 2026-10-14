package client_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/client"
)

// A secret reads the value, or the placeholder as hidden whatever its type, and writes back what it read.
func TestSecret(t *testing.T) {
	t.Parallel()

	type record struct {
		Port     client.Secret[int]      `json:"port"`
		Token    client.Secret[string]   `json:"token"`
		Names    client.Secret[[]string] `json:"names"`
		Region   client.Secret[bool]     `json:"region"`
		Provider client.Secret[struct {
			Key string `json:"key"`
		}] `json:"provider"`
	}
	var r record
	if err := json.Unmarshal([]byte(`{"port": "********", "token": "abc", "names": "********", "region": true, "provider": {"key": "k"}}`), &r); err != nil {
		t.Fatal(err)
	}
	if !r.Port.Hidden || r.Port.Value != 0 || r.Token.Hidden || r.Token.Value != "abc" || !r.Names.Hidden || r.Names.Value != nil || r.Region.Hidden || !r.Region.Value || r.Provider.Hidden || r.Provider.Value.Key != "k" {
		t.Errorf("read = %+v", r)
	}

	out, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"port":"********","token":"abc","names":"********","region":true,"provider":{"key":"k"}}`; string(out) != want {
		t.Errorf("written = %s, want %s", out, want)
	}

	// a hidden text is told from the same text as a value
	var token client.Secret[string]
	if err := json.Unmarshal([]byte(`"********"`), &token); err != nil || !token.Hidden {
		t.Errorf("the placeholder as text = %+v, %v", token, err)
	}

	// a value of the wrong type is still an error
	var port client.Secret[int]
	if err := json.Unmarshal([]byte(`"seven"`), &port); err == nil {
		t.Error("text that is not the placeholder read into a number")
	}

	// a record printed any way shows nothing of the secret
	for _, printed := range []string{fmt.Sprint(r), fmt.Sprintf("%v", r), fmt.Sprintf("%+v", r), fmt.Sprintf("%#v", r), fmt.Sprintf("%s %v", r.Token, r.Port), r.Token.String()} {
		if strings.Contains(printed, "abc") || !strings.Contains(printed, "***") {
			t.Errorf("printed = %s", printed)
		}
	}
}
