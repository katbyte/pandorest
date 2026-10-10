package client_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/internal/client"
)

// options is an options object, as a generated one is.
type options struct{ query, header string }

func (o options) ToHeaders() *client.Headers {
	out := client.Headers{}
	out.Append("X-Client", o.header)

	return &out
}

func (o options) ToQuery() *client.QueryParams {
	out := client.QueryParams{}
	out.Append("ids", o.query)

	return &out
}

// serve answers every request with one status and body, and hands back the
// last request it saw with its body.
func serve(t *testing.T, status int, body string) (c *client.Client, last func() (*http.Request, string)) {
	t.Helper()

	var seen *http.Request
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen, sent = r.Clone(r.Context()), string(b)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL+"/", client.Token("secret"))
	if err != nil {
		t.Fatal(err)
	}
	c.HTTPClient = srv.Client()

	return c, func() (*http.Request, string) { return seen, sent }
}

func TestNew(t *testing.T) {
	t.Parallel()

	if _, err := client.New("server", client.Token("t")); err == nil {
		t.Error("New without a scheme succeeded")
	}
	if _, err := client.New("http://server", nil); err == nil {
		t.Error("New without an authorizer succeeded")
	}
	if c, err := client.New("http://server/base/", client.Token("t")); err != nil || c.BaseURL != "http://server/base" {
		t.Errorf("New = %+v, %v", c, err)
	}
}

// A request carries the credential, the options and a JSON body, and its
// answer decodes and can be read again.
func TestRequest(t *testing.T) {
	t.Parallel()

	c, last := serve(t, http.StatusOK, `{"Name": "a"}`)
	req, err := c.NewRequest(t.Context(), client.RequestOptions{
		ContentType: "application/json", ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodPost, Path: "/Items/a%2Fb", OptionsObject: options{query: client.CSV([]string{"1", "2"}), header: "h"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := req.Marshal(map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	resp, err := req.Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	r, body := last()
	if r.Method != http.MethodPost || r.URL.EscapedPath() != "/Items/a%2Fb" || r.URL.Query().Get("ids") != "1,2" || r.Header.Get("X-Client") != "h" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "application/json" || body != `{"n":1}` {
		t.Errorf("the server saw %s %s, headers %v, body %q", r.Method, r.URL, r.Header, body)
	}

	var model struct{ Name string }
	if err := resp.Unmarshal(&model); err != nil || model.Name != "a" {
		t.Errorf("Unmarshal = %+v, %v", model, err)
	}
	if err := resp.Unmarshal(&[]int{}); err == nil || client.StatusCode(err) != 0 {
		t.Errorf("an answer that does not decode = %v", err)
	}
	if again, _ := io.ReadAll(resp.Body); string(again) != `{"Name": "a"}` {
		t.Errorf("the body read again = %q", again)
	}
}

// A status the operation does not document is an error that carries it, with
// the response; an empty answer leaves the model alone.
func TestStatuses(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusTeapot, "no")
	req, err := c.NewRequest(t.Context(), client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := req.Execute(t.Context())
	if client.StatusCode(err) != http.StatusTeapot || resp == nil || !strings.Contains(err.Error(), "GET /x: HTTP 418: no") {
		t.Errorf("an undocumented status = %v, response %v", err, resp)
	}
	if client.StatusCode(errors.New("other")) != 0 {
		t.Error("StatusCode of another error is not 0")
	}

	c, _ = serve(t, http.StatusNoContent, "")
	req, err = c.NewRequest(t.Context(), client.RequestOptions{ExpectedStatusCodes: []int{http.StatusNoContent}, HTTPMethod: http.MethodGet, Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err = req.Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	model := "kept"
	if err := resp.Unmarshal(&model); err != nil || model != "kept" {
		t.Errorf("an empty answer = %q, %v", model, err)
	}
}

// Raw bytes go as the type the caller names, a range of types must be
// narrowed to one, and a streamed answer is left unread for the caller.
func TestBodiesAndStreams(t *testing.T) {
	t.Parallel()

	c, last := serve(t, http.StatusOK, "file bytes")
	opts := client.RequestOptions{ContentType: "image/*", ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodPost, Path: "/image", StreamResponse: true}
	req, err := c.NewRequest(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := req.SetBody(strings.NewReader("raw"), ""); err == nil || !strings.Contains(err.Error(), "name the body's content type") {
		t.Errorf("SetBody with a range of types = %v", err)
	}
	if err := req.SetBody(nil, "image/png"); err != nil {
		t.Errorf("SetBody with no body = %v", err)
	}
	if err := req.SetBody(strings.NewReader("raw"), "image/png"); err != nil {
		t.Fatal(err)
	}
	resp, err := req.Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	streamed, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if r, body := last(); string(streamed) != "file bytes" || body != "raw" || r.Header.Get("Content-Type") != "image/png" {
		t.Errorf("streamed %q after sending %q as %q", streamed, body, r.Header.Get("Content-Type"))
	}

	if err := req.Marshal(func() {}); err == nil {
		t.Error("Marshal of what is not JSON succeeded")
	}
}

func TestParameterForms(t *testing.T) {
	t.Parallel()

	if got := client.CSV([]int{1, 2, 3}); got != "1,2,3" {
		t.Errorf("CSV = %q", got)
	}
	if got := client.DeepObject("o", map[string]string{"k": "v", "a": "b"}); len(got) != 2 || got[0] != [2]string{"o[a]", "b"} || got[1] != [2]string{"o[k]", "v"} {
		t.Errorf("DeepObject = %v", got)
	}
	if got := client.JSONObject(map[string]string{"k": "v"}); got != `{"k":"v"}` {
		t.Errorf("JSONObject = %q", got)
	}
	if got := client.JSONObject(map[string]func(){"k": nil}); got != "" {
		t.Errorf("JSONObject of what is not JSON = %q", got)
	}
}
