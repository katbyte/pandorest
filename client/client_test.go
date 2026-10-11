package client_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katbyte/go-kt/chttp"
	"github.com/katbyte/pandorest/client"
)

// noWait retries at once, so a test of a request tried again does not sleep.
var noWait = client.WithRetry(chttp.Retry{Wait: func(int) time.Duration { return 0 }})

// arr is a service in the shape a repository declares one.
var arr = client.Service{
	Name: "Arr", UserAgent: "arr-test/1.2.3", Example: "nas:8989", SecretHeaders: []string{"X-Arr-Token"}, RedirectAdvice: "set the url to the address Arr answers on, with its URL base", WebPageAdvice: "check the server url and its URL base",
	Messages: func(body []byte) []string {
		if !bytes.HasPrefix(body, []byte("failed: ")) {
			return nil
		}

		return strings.Split(strings.TrimPrefix(string(body), "failed: "), ", ")
	},
	Note: func(f client.Failure) string {
		switch {
		case f.StatusCode == http.StatusUnauthorized && f.Credentialed:
			return "the key was refused"
		case f.StatusCode == http.StatusUnauthorized:
			return "no key was sent"
		case f.StatusCode == http.StatusNotFound && !bytes.HasPrefix(f.Body, []byte("{")):
			return "the server has no such call"
		}

		return ""
	},
}

// seen is what a test server was sent.
type seen struct {
	sync.Mutex

	requests []*http.Request
	bodies   []string
}

func (s *seen) last(t *testing.T) (r *http.Request, body string) {
	t.Helper()

	s.Lock()
	defer s.Unlock()
	if len(s.requests) == 0 {
		t.Fatal("the server saw no request")
	}

	return s.requests[len(s.requests)-1], s.bodies[len(s.bodies)-1]
}

func (s *seen) count() int {
	s.Lock()
	defer s.Unlock()

	return len(s.requests)
}

// serve starts a server answering with handler, and a client for it made by
// the service.
func serve(t *testing.T, service client.Service, authorizer client.Authorizer, handler http.HandlerFunc, opts ...client.Option) (c *client.Client, got *seen) {
	t.Helper()

	got = &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Lock()
		got.requests, got.bodies = append(got.requests, r.Clone(r.Context())), append(got.bodies, string(b))
		got.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := service.New(srv.URL+"/", authorizer, append([]client.Option{noWait}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}

	return c, got
}

func answer(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func execute(t *testing.T, c *client.Client, opts client.RequestOptions) (*client.Response, error) {
	t.Helper()

	req, err := c.NewRequest(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	return req.Execute(t.Context())
}

func get(path string) client.RequestOptions {
	return client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: path}
}

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

// accepting is the options of an operation with an Accept option, set.
type accepting string

func (a accepting) ToHeaders() *client.Headers {
	out := client.Headers{}
	out.Append("Accept", string(a))

	return &out
}

func (accepting) ToQuery() *client.QueryParams { return &client.QueryParams{} }

// A server url is the address the server answers on and nothing more, and a
// refusal says so in the service's words without repeating a password.
func TestNew(t *testing.T) {
	t.Parallel()

	for url, want := range map[string]string{ //nolint:gosec // the password is the test: a refusal must not repeat it
		"":                             "a server url is required",
		"http://nas:8989\x7f":          "the server url cannot be read as one: it must be the address Arr answers on, e.g. http://nas:8989",
		"http://user:hunter2@nas:8989": "the server url must not contain credentials",
		"nas:8989":                     `must be the address Arr answers on, with its scheme and host, e.g. http://nas:8989`,
		"ftp://nas":                    "must be the address Arr answers on",
		"http://nas:8989?apikey=abc":   `server url "http://nas:8989?apikey=REDACTED" must not carry a query or a fragment`,
		"http://nas:8989#top":          "must not carry a query or a fragment",
	} {
		_, err := arr.New(url, client.APIKey("k"))
		if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), "abc") {
			t.Errorf("New(%q) = %v, want containing %q", url, err, want)
		}
	}
	if _, err := arr.New("http://nas:8989", nil); err == nil || err.Error() != "an authorizer is required" {
		t.Errorf("New without an authorizer = %v", err)
	}

	// a path is the server's URL base and is kept; a trailing slash is not
	c, err := arr.New("https://nas/arr/", client.APIKey("k"))
	if err != nil || c.BaseURL != "https://nas/arr" {
		t.Errorf("New = %+v, %v", c, err)
	}
	// a hosted API gives its whole address as the example, and it is shown as given
	if _, err := (client.Service{Name: "TMDB", Example: "https://api.example.org"}).New("api.example.org", client.Bearer("t")); err == nil || !strings.Contains(err.Error(), "must be the address TMDB answers on, with its scheme and host, e.g. https://api.example.org") {
		t.Errorf("New for a hosted API = %v", err)
	}
	// a server of no particular kind is named as one
	if _, err := client.New("nas", client.Bearer("t")); err == nil || !strings.Contains(err.Error(), `server url "nas" must be the address the server answers on, with its scheme and host`) || strings.Contains(err.Error(), "e.g.") {
		t.Errorf("New for no service = %v", err)
	}
}

// A request carries the credential, the service's user agent, the options
// and a JSON body, and its answer decodes and can be read again.
func TestRequest(t *testing.T) {
	t.Parallel()

	c, got := serve(t, arr, client.APIKey("secret"), answer(http.StatusOK, `{"Name": "a"}`))
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
	r, body := got.last(t)
	if r.Method != http.MethodPost || r.URL.EscapedPath() != "/Items/a%2Fb" || r.URL.Query().Get("ids") != "1,2" || r.Header.Get("X-Client") != "h" || r.Header.Get("X-Api-Key") != "secret" || r.Header.Get("User-Agent") != "arr-test/1.2.3" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" || r.ContentLength != 7 || body != `{"n":1}` {
		t.Errorf("the server saw %s %s, headers %v, body %q", r.Method, r.URL, r.Header, body)
	}

	var model struct{ Name string }
	if err := resp.Unmarshal(&model); err != nil || model.Name != "a" {
		t.Errorf("Unmarshal = %+v, %v", model, err)
	}
	if err := resp.Unmarshal(&[]int{}); err == nil || client.StatusCode(err) != 0 || !strings.Contains(err.Error(), `POST /Items/a%2Fb: the answer is not what this client reads there`) && !strings.Contains(err.Error(), "the answer is not what this client reads there") {
		t.Errorf("an answer that does not decode = %v", err)
	}
	if again, _ := io.ReadAll(resp.Body); string(again) != `{"Name": "a"}` {
		t.Errorf("the body read again = %q", again)
	}
	if err := req.Marshal(func() {}); err == nil {
		t.Error("Marshal of what is not JSON succeeded")
	}

	// an empty answer leaves the model alone
	c, _ = serve(t, arr, client.APIKey("k"), answer(http.StatusOK, ""))
	resp, err = execute(t, c, get("/x"))
	kept := "kept"
	if err != nil || resp.Unmarshal(&kept) != nil || kept != "kept" {
		t.Errorf("an empty answer = %q, %v", kept, err)
	}
}

// Text is the body as it was sent, whatever the server calls it, and leaves
// the body to be read again.
func TestText(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, arr, client.APIKey("secret"), func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, " /media\n")
	})
	resp, err := execute(t, c, client.RequestOptions{Accept: "text/plain", ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/path"})
	if err != nil {
		t.Fatal(err)
	}

	var text string
	if err := resp.Text(&text); err != nil || text != " /media\n" {
		t.Errorf("Text = %q, %v", text, err)
	}
	if again, _ := io.ReadAll(resp.Body); string(again) != " /media\n" {
		t.Errorf("the body read again = %q", again)
	}
}

// A request asks for what its operation is documented to answer: JSON alone
// unless that is something else, which is then asked for first with JSON
// after it. A server may choose its answer by the header, and one that tests
// for "application/json" alone must not find it alone on a call for a stream.
func TestAccept(t *testing.T) {
	t.Parallel()

	for documented, want := range map[string]string{
		"":                  "application/json",
		"application/json":  "application/json",
		"text/event-stream": "text/event-stream, application/json;q=0.9",
		"image/*":           "image/*, application/json;q=0.9",
	} {
		c, got := serve(t, arr, client.APIKey("secret"), answer(http.StatusOK, `{}`))
		req, err := c.NewRequest(t.Context(), client.RequestOptions{Accept: documented, ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/x"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := req.Execute(t.Context()); err != nil {
			t.Fatal(err)
		}

		if r, _ := got.last(t); r.Header.Get("Accept") != want {
			t.Errorf("an operation documented to answer %q asked for %q, want %q", documented, r.Header.Get("Accept"), want)
		}
	}

	// an Accept among the options is the caller's own, and is the whole header
	c, got := serve(t, arr, client.APIKey("secret"), answer(http.StatusOK, `{}`))
	req, err := c.NewRequest(t.Context(), client.RequestOptions{Accept: "image/*", ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/x", OptionsObject: accepting("*/*")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := req.Execute(t.Context()); err != nil {
		t.Fatal(err)
	}

	if r, _ := got.last(t); !slices.Equal(r.Header.Values("Accept"), []string{"*/*"}) {
		t.Errorf("an Accept option asked for %q, want it alone", r.Header.Values("Accept"))
	}
}

// How a credential travels is the authorizer's, and one that sends nothing
// sends nothing.
func TestAuthorizers(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		authorizer client.Authorizer
		header     string
		want       string
	}{
		"a key":            {client.APIKey("k1"), "X-Api-Key", "k1"},
		"a bearer token":   {client.Bearer("t1"), "Authorization", "Bearer t1"},
		"no token":         {client.Bearer(""), "Authorization", ""},
		"a way of its own": {client.AuthorizerFunc(func(r *http.Request) { r.Header.Set("X-Arr-Token", "own") }), "X-Arr-Token", "own"},
	} {
		c, got := serve(t, client.Service{}, tc.authorizer, answer(http.StatusOK, "{}"))
		if _, err := execute(t, c, get("/x")); err != nil {
			t.Fatal(err)
		}
		if r, _ := got.last(t); r.Header.Get(tc.header) != tc.want {
			t.Errorf("%s: %s = %q, want %q", name, tc.header, r.Header.Get(tc.header), tc.want)
		}
	}
}

// A status the operation does not document is an error that carries it, what
// the server said, and what the service knows of it, with the response.
func TestStatusError(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, arr, client.APIKey("k"), answer(http.StatusBadRequest, "failed: Title: must not be empty, Year: must be a year"))
	resp, err := execute(t, c, get("/movie"))
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("the response beside the error = %+v", resp)
	}
	if want := "GET /movie: HTTP 400 (expected 200): Title: must not be empty; Year: must be a year"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %s", err, want)
	}
	if got := client.Messages(err); !slices.Equal(got, []string{"Title: must not be empty", "Year: must be a year"}) {
		t.Errorf("Messages = %q", got)
	}
	if client.StatusCode(err) != http.StatusBadRequest || chttp.StatusCode(fmt.Errorf("wrapped: %w", err)) != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, chttp's through a wrap = %d", client.StatusCode(err), chttp.StatusCode(fmt.Errorf("wrapped: %w", err)))
	}
	if se, ok := errors.AsType[*client.StatusError](err); !ok || se.Tries != 0 || se.Path != "/movie" {
		t.Errorf("the status error = %+v", se)
	}
	if again, _ := io.ReadAll(resp.Body); !strings.HasPrefix(string(again), "failed: ") {
		t.Errorf("the body beside the error = %q", again)
	}

	// what is not one of the service's error shapes is shown as it came
	c, _ = serve(t, arr, client.APIKey("k"), answer(http.StatusInternalServerError, " it broke "))
	_, err = execute(t, c, get("/x"))
	if err == nil || err.Error() != "GET /x: HTTP 500 (expected 200): it broke" || client.Messages(err) != nil {
		t.Errorf("an error of no known shape = %v, messages %q", err, client.Messages(err))
	}
	if client.Messages(errors.New("other")) != nil || client.StatusCode(errors.New("other")) != 0 {
		t.Error("another error has messages or a status")
	}

	// the note is the service's, and knows whether a credential was sent
	for name, tc := range map[string]struct {
		authorizer client.Authorizer
		status     int
		body, note string
		is         func(error) bool
	}{
		"a key refused":         {client.APIKey("k"), http.StatusUnauthorized, "", "(the key was refused)", client.IsUnauthorized},
		"no key sent":           {client.Bearer(""), http.StatusUnauthorized, "", "(no key was sent)", client.IsUnauthorized},
		"a path it lacks":       {client.APIKey("k"), http.StatusNotFound, "Not Found", "(the server has no such call)", client.IsNotFound},
		"a thing that is gone":  {client.APIKey("k"), http.StatusNotFound, `{"message": "no"}`, "", client.IsNotFound},
		"a thing not permitted": {client.APIKey("k"), http.StatusForbidden, "", "", client.IsForbidden},
	} {
		c, _ := serve(t, arr, tc.authorizer, answer(tc.status, tc.body))
		resp, err := execute(t, c, get("/x"))
		// "(expected 200)" is there for all of them; a note is a second pair of brackets
		if err == nil || !tc.is(err) || (strings.Count(err.Error(), "(") == 2) != (tc.note != "") || !strings.Contains(err.Error(), tc.note) {
			t.Errorf("%s: error = %v, want the note %q", name, err, tc.note)
		}
		if client.WasNotFound(resp.Response) != (tc.status == http.StatusNotFound) {
			t.Errorf("%s: WasNotFound = %t", name, client.WasNotFound(resp.Response))
		}
	}
	if client.WasNotFound(nil) {
		t.Error("no response was not found")
	}
}

// A read is asked again for a gateway's answer and says how often it was
// sent; a write is sent once, since it may have been made.
func TestRetries(t *testing.T) {
	t.Parallel()

	flaky := func() http.HandlerFunc {
		var mu sync.Mutex
		n := 0

		return func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			n++
			first := n == 1
			mu.Unlock()
			if first {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = io.WriteString(w, `{"ok": true}`)
		}
	}

	c, got := serve(t, arr, client.APIKey("k"), flaky())
	if _, err := execute(t, c, get("/x")); err != nil || got.count() != 2 {
		t.Errorf("a read after a 502 = %v, sent %d times", err, got.count())
	}

	c, got = serve(t, arr, client.APIKey("k"), flaky())
	if _, err := execute(t, c, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodPost, Path: "/x"}); client.StatusCode(err) != http.StatusBadGateway || got.count() != 1 {
		t.Errorf("a write after a 502 = %v, sent %d times", err, got.count())
	}

	c, got = serve(t, arr, client.APIKey("k"), answer(http.StatusServiceUnavailable, "busy"))
	_, err := execute(t, c, get("/x"))
	if se, ok := errors.AsType[*client.StatusError](err); !ok || se.Tries != 3 || got.count() != 3 || !strings.Contains(err.Error(), "(tried 3 times)") {
		t.Errorf("a read that keeps failing = %v, sent %d times", err, got.count())
	}
}

// A redirect is refused with the service's advice unless the service follows
// them, and then the credentials do not leave the server's host.
func TestRedirects(t *testing.T) {
	t.Parallel()

	elsewhere, got := serve(t, arr, client.APIKey("k"), answer(http.StatusOK, "{}"))
	redirect := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.BaseURL+"/landed", http.StatusFound)
	}

	c, _ := serve(t, arr, client.APIKey("k"), redirect)
	if _, err := execute(t, c, get("/x")); err == nil || !strings.Contains(err.Error(), "set the url to the address Arr answers on, with its URL base") || got.count() != 0 {
		t.Errorf("a redirect on a service that refuses them = %v (the target saw %d requests)", err, got.count())
	}
	c, _ = serve(t, client.Service{}, client.APIKey("k"), redirect)
	if _, err := execute(t, c, get("/x")); err == nil || !strings.Contains(err.Error(), "set the server url to the address the server itself answers on") {
		t.Errorf("a redirect on a server of no kind = %v", err)
	}

	following := arr
	following.FollowRedirects = true
	own := client.AuthorizerFunc(func(r *http.Request) {
		r.Header.Set("X-Arr-Token", "t")
		r.Header.Set("X-Api-Key", "k")
	})
	c, _ = serve(t, following, own, redirect)
	if _, err := execute(t, c, get("/x")); err != nil {
		t.Fatal(err)
	}
	// httptest servers share a host and differ in port, which is the same machine: the credentials stay
	if r, _ := got.last(t); r.URL.Path != "/landed" || r.Header.Get("X-Arr-Token") != "t" {
		t.Errorf("the redirect's target saw %s with headers %v", r.URL, r.Header)
	}
}

// A redirect an operation documents as its answer is handed back as it came,
// whatever the service does with the rest: a login sends its caller on, and
// what it says is where, and the cookie it set.
func TestRedirectThatIsTheAnswer(t *testing.T) {
	t.Parallel()

	login := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/landed" {
			t.Error("the redirect was followed")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "s1"}) //nolint:gosec // a test server's cookie, over plain http
		w.Header().Set("Content-Type", "text/html")
		http.Redirect(w, r, "/landed", http.StatusFound)
	}
	for name, service := range map[string]client.Service{"a service that refuses redirects": arr, "one that follows them": {FollowRedirects: true}} {
		for _, stream := range []bool{false, true} {
			c, got := serve(t, service, client.APIKey("k"), login)
			resp, err := execute(t, c, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusFound}, HTTPMethod: http.MethodPost, Path: "/login", StreamResponse: stream})
			if err != nil {
				t.Fatalf("%s (stream %t): %v", name, stream, err)
			}
			if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/landed" || len(resp.Cookies()) != 1 || got.count() != 1 {
				t.Errorf("%s (stream %t): the answer = %d to %q with cookies %v after %d requests", name, stream, resp.StatusCode, resp.Header.Get("Location"), resp.Cookies(), got.count())
			}
			_ = resp.Body.Close()
		}
	}
}

// A redirect an operation documents beside its answer is on the way to that
// answer: it is followed, on a service that refuses every other redirect too,
// and the credentials go with it while it stays on the server's host.
func TestRedirectOnTheWayToTheAnswer(t *testing.T) {
	t.Parallel()

	moved := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/landed" {
			http.Redirect(w, r, "/landed", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, `{"Name": "there"}`)
	}
	for _, stream := range []bool{false, true} {
		c, got := serve(t, arr, client.APIKey("k"), moved)
		resp, err := execute(t, c, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK, http.StatusFound}, HTTPMethod: http.MethodGet, Path: "/audio", StreamResponse: stream})
		if err != nil {
			t.Fatalf("stream %t: %v", stream, err)
		}
		var model struct{ Name string }
		if resp.StatusCode != http.StatusOK || resp.Unmarshal(&model) != nil || model.Name != "there" || got.count() != 2 {
			t.Errorf("stream %t: the answer = %d %+v after %d requests", stream, resp.StatusCode, model, got.count())
		}
		if r, _ := got.last(t); r.URL.Path != "/landed" || r.Header.Get("X-Api-Key") != "k" {
			t.Errorf("stream %t: where it landed saw %s with key %q", stream, r.URL.Path, r.Header.Get("X-Api-Key"))
		}

		// the same redirect on an operation that documents none is still refused
		c, _ = serve(t, arr, client.APIKey("k"), moved)
		if _, err := execute(t, c, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/audio", StreamResponse: stream}); err == nil || !strings.Contains(err.Error(), "set the url to the address Arr answers on") {
			t.Errorf("stream %t: an undocumented redirect = %v", stream, err)
		}
	}
}

// How long a server has to start answering is the service's to say, apart
// from how long the whole request has.
func TestHeaderWait(t *testing.T) {
	t.Parallel()

	impatient := arr
	impatient.Timeout, impatient.HeaderWait = 30*time.Second, 100*time.Millisecond
	c, _ := serve(t, impatient, client.APIKey("k"), func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = io.WriteString(w, "{}")
	})
	started := time.Now()
	if _, err := execute(t, c, get("/slow")); err == nil || !strings.Contains(err.Error(), "timeout awaiting response headers") || time.Since(started) > 10*time.Second {
		t.Errorf("a server slower to start than the service allows = %v after %s", err, time.Since(started))
	}

	// and one that sets no wait of its own gives the server the whole of the request's time
	patient := arr
	patient.Timeout = 10 * time.Second
	c, _ = serve(t, patient, client.APIKey("k"), func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, "{}")
	})
	if _, err := execute(t, c, get("/slow")); err != nil {
		t.Errorf("a slow start within the request's time = %v", err)
	}
}

// A web page where the API was expected, and an answer longer than the
// client holds, are errors that say what to do, with the response.
func TestAnswersThatAreNotTheAPI(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, arr, client.APIKey("k"), answer(http.StatusOK, "<!DOCTYPE html><html><body>sign in</body></html>"))
	resp, err := execute(t, c, get("/x"))
	if err == nil || err.Error() != "GET /x: answered with a web page, not Arr's API: check the server url and its URL base" || resp == nil || client.StatusCode(err) != 0 {
		t.Errorf("a web page = %v", err)
	}

	small := arr
	small.MaxResponse = 1 << 20
	c, _ = serve(t, small, client.APIKey("k"), answer(http.StatusOK, `"`+strings.Repeat("x", 2<<20)+`"`))
	if _, err := execute(t, c, get("/x")); err == nil || !strings.Contains(err.Error(), "GET /x: the answer is over 1 MiB, more than this client holds") {
		t.Errorf("an answer too large = %v", err)
	}

	// a server that is not there says so without the key the url might carry
	c, _ = serve(t, arr, client.AuthorizerFunc(func(r *http.Request) { r.URL.RawQuery = "apikey=abc" }), answer(http.StatusOK, "{}"))
	c.BaseURL = "http://127.0.0.1:1"
	if _, err := execute(t, c, get("/x")); err == nil || strings.Contains(err.Error(), "abc") || !strings.Contains(err.Error(), "GET /x: ") {
		t.Errorf("a server that is not there = %v", err)
	}
}

// A file's answer is handed back unread, and a refusal of one is read; raw
// bytes go as the type the caller names, with their length when it is known.
func TestStreamsAndBodies(t *testing.T) {
	t.Parallel()

	c, got := serve(t, arr, client.APIKey("k"), answer(http.StatusOK, "file bytes"))
	opts := client.RequestOptions{ContentType: "image/*", ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodPost, Path: "/image", StreamResponse: true}
	req, err := c.NewRequest(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := req.SetBody(strings.NewReader("raw"), ""); err == nil || !strings.Contains(err.Error(), "name the body's content type (the operation takes image/*)") {
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
	if r, body := got.last(t); string(streamed) != "file bytes" || body != "raw" || r.Header.Get("Content-Type") != "image/png" || r.ContentLength != 3 {
		t.Errorf("streamed %q after sending %q as %q, length %d", streamed, body, r.Header.Get("Content-Type"), r.ContentLength)
	}

	// a reader that knows its length says so, and one that does not is sent as it comes
	for name, tc := range map[string]struct {
		body io.Reader
		want int64
	}{"bytes": {bytes.NewReader([]byte("12345")), 5}, "a buffer": {bytes.NewBufferString("1234"), 4}, "a pipe": {io.MultiReader(strings.NewReader("12")), -1}} {
		sized, sizedErr := c.NewRequest(t.Context(), client.RequestOptions{ContentType: "text/plain", ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodPut, Path: "/text"})
		if sizedErr != nil {
			t.Fatal(sizedErr)
		}
		if err := sized.SetBody(tc.body, ""); err != nil || sized.ContentLength != tc.want || sized.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("%s: SetBody = %v, length %d, type %q", name, err, sized.ContentLength, sized.Header.Get("Content-Type"))
		}
	}

	// a stream that is refused is read, and is a status error like any other
	c, _ = serve(t, arr, client.APIKey("k"), answer(http.StatusNotFound, "failed: no such file"))
	resp, err = execute(t, c, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/file", StreamResponse: true})
	if !client.IsNotFound(err) || !slices.Equal(client.Messages(err), []string{"no such file"}) || resp == nil {
		t.Errorf("a refused stream = %v", err)
	}

	// a streamed answer that is JSON after all can still be decoded
	c, _ = serve(t, arr, client.APIKey("k"), answer(http.StatusOK, `{"Name": "late"}`))
	resp, err = execute(t, c, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/file", StreamResponse: true})
	var model struct{ Name string }
	if err != nil || resp.Unmarshal(&model) != nil || model.Name != "late" {
		t.Errorf("a stream decoded = %+v, %v", model, err)
	}

	// and one that is not JSON says so, with the start of what it was
	c, _ = serve(t, arr, client.APIKey("k"), answer(http.StatusOK, "plain text"))
	resp, err = execute(t, c, client.RequestOptions{ExpectedStatusCodes: []int{http.StatusOK}, HTTPMethod: http.MethodGet, Path: "/file", StreamResponse: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Unmarshal(&model); err == nil || !strings.Contains(err.Error(), "GET /file: the answer is not what this client reads there") || !strings.HasSuffix(err.Error(), ": plain text") {
		t.Errorf("a stream that is not JSON, decoded = %v", err)
	}
}

// tracer is a logger that keeps what it is told.
type tracer struct {
	sync.Mutex

	lines []string
}

func (l *tracer) Debugf(format string, args ...any) { l.Tracef(format, args...) }

func (l *tracer) Tracef(format string, args ...any) {
	l.Lock()
	defer l.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

// What the application decides: a logger, whose trace shows the exchange and
// none of the credentials, a cookie jar, and a transport of its own.
func TestOptions(t *testing.T) {
	t.Parallel()

	log := &tracer{}
	own := client.AuthorizerFunc(func(r *http.Request) {
		r.Header.Set("X-Api-Key", "key-in-a-header")
		r.Header.Set("X-Arr-Token", "token-of-its-own")
	})
	c, _ := serve(t, arr, own, answer(http.StatusOK, `{"ok": true}`), client.WithLog(log))
	if _, err := execute(t, c, get("/traced")); err != nil {
		t.Fatal(err)
	}
	trace := strings.Join(log.lines, "\n")
	if !strings.Contains(trace, "/traced") || strings.Contains(trace, "key-in-a-header") || strings.Contains(trace, "token-of-its-own") {
		t.Errorf("the trace shows a credential, or not the request:\n%s", trace)
	}

	// a session the server gives is kept and sent back, and counts as a credential
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c, got := serve(t, arr, client.Bearer(""), func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("session"); err != nil {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "s1"}) //nolint:gosec // a test server's cookie, over plain http //nolint:gosec // a test server's cookie, over plain http
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}, client.WithCookies(jar))
	if _, err := execute(t, c, get("/login")); err == nil || !strings.Contains(err.Error(), "(no key was sent)") {
		t.Errorf("a 401 with no credential = %v", err)
	}
	if _, err := execute(t, c, get("/again")); err == nil || !strings.Contains(err.Error(), "(the key was refused)") {
		t.Errorf("a 401 with a session = %v", err)
	}
	if r, _ := got.last(t); r.Header.Get("Cookie") != "session=s1" {
		t.Errorf("the second request's cookie = %q", r.Header.Get("Cookie"))
	}

	// a transport of the caller's own carries the requests
	carried := 0
	base := roundTripper(func(r *http.Request) (*http.Response, error) {
		carried++
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
	})
	c, err = arr.New("http://nowhere.invalid", client.APIKey("k"), client.WithTransport(base))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, c, get("/x")); err != nil || carried != 1 {
		t.Errorf("a request through the caller's transport = %v, carried %d", err, carried)
	}

	// and the http client can be replaced whole, as a generated test does
	srv := httptest.NewServer(answer(http.StatusOK, `{"ok": true}`))
	t.Cleanup(srv.Close)
	c, err = arr.New(srv.URL, client.APIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	c.HTTPClient = srv.Client()
	if _, err := execute(t, c, get("/x")); err != nil {
		t.Errorf("a request through a replaced http client = %v", err)
	}

	// the client put in its place has no redirect rule of its own, so the service's still holds
	moved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, srv.URL+"/landed", http.StatusFound) }))
	t.Cleanup(moved.Close)
	c, err = arr.New(moved.URL, client.APIKey("k"))
	if err != nil {
		t.Fatal(err)
	}
	c.HTTPClient = moved.Client()
	if _, err := execute(t, c, get("/x")); err == nil || !strings.Contains(err.Error(), "set the url to the address Arr answers on") {
		t.Errorf("a redirect through a replaced http client = %v", err)
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

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

// A date is read in any of its layouts, null and "" are zero, and one no
// layout reads is an error naming it and the layouts.
func TestParseTime(t *testing.T) {
	t.Parallel()

	layouts := []string{"2006-01-02T15:04:05.0000000Z07:00", time.RFC3339}
	for data, want := range map[string]string{`null`: "0001-01-01T00:00:00Z", `""`: "0001-01-01T00:00:00Z", `"2024-03-04T05:06:07.0000000Z"`: "2024-03-04T05:06:07Z", `"2024-03-04T05:06:07Z"`: "2024-03-04T05:06:07Z"} {
		got, err := client.ParseTime([]byte(data), layouts)
		if err != nil || got.Format(time.RFC3339) != want {
			t.Errorf("ParseTime(%s) = %v, %v, want %s", data, got, err, want)
		}
	}
	if _, err := client.ParseTime([]byte(`"04/03/2024"`), layouts); err == nil || !strings.Contains(err.Error(), `a date no layout reads: "04/03/2024" (layouts 2006-01-02T15:04:05.0000000Z07:00, 2006-01-02T15:04:05Z07:00)`) {
		t.Errorf("a date in no layout = %v", err)
	}
	var unread *client.UnreadDateError
	if _, err := client.ParseTime([]byte(`"04/03/2024"`), layouts); !errors.As(err, &unread) || unread.Text != "04/03/2024" {
		t.Errorf("a date in no layout is not an UnreadDateError with its text: %v", err)
	}
	for data, want := range map[string]int64{`null`: 0, `0`: 0, `""`: 0, `1709528767`: 1709528767, `"1709528767"`: 1709528767} {
		if got, err := client.ParseUnix([]byte(data)); err != nil || got != want {
			t.Errorf("ParseUnix(%s) = %d, %v, want %d", data, got, err, want)
		}
	}
	if _, err := client.ParseUnix([]byte(`"2024-03-04"`)); err == nil || !strings.Contains(err.Error(), "a date that is not a whole number of seconds or milliseconds") {
		t.Errorf("an epoch that is a date string = %v", err)
	}
	if _, err := client.ParseTime([]byte(`7`), layouts); err == nil || !strings.Contains(err.Error(), "a date that is not a string: 7") {
		t.Errorf("a number = %v", err)
	}
}
