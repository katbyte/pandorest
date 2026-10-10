// Package client is the smallest base client the code pandorest generates
// builds and passes its tests on: what a generated package needs of its base
// client, in code. The generator's tests build a generated package against
// it, so a change to what generated code asks of a base client shows up here
// first.
//
// It is not the base client of any SDK. Each repository keeps its own beside
// its generated packages, after go-azure-sdk's sdk/client, because that is
// where a service's own behaviour belongs: how it takes a credential, what
// it retries, how it words a failure. This one authenticates with a bearer
// token and does nothing more than it must.
package client

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// DefaultPageSize is the page a Complete method asks for when the options
// leave the page size unset.
const DefaultPageSize = 500

// Authorizer adds credentials to a request.
type Authorizer interface {
	Authorize(req *http.Request)
}

// Token authenticates with a bearer token.
type Token string

// Authorize sends the token in the Authorization header.
func (t Token) Authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+string(t))
}

// Client sends requests to one server.
type Client struct {
	// BaseURL is the server address without a trailing slash.
	BaseURL    string
	HTTPClient *http.Client
	Authorizer Authorizer
}

// New returns a client for the server at baseURL (scheme and host, optionally
// a path prefix).
func New(baseURL string, authorizer Authorizer) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("server URL %q must include a scheme and host", baseURL)
	}
	if authorizer == nil {
		return nil, errors.New("an authorizer is required")
	}

	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTPClient: &http.Client{}, Authorizer: authorizer}, nil
}

// Options is what a generated options struct implements: the query and
// header parameters it holds.
type Options interface {
	ToHeaders() *Headers
	ToQuery() *QueryParams
}

// Headers are request headers from an options object.
type Headers struct {
	values http.Header
}

// Append adds a header.
func (h *Headers) Append(key, value string) {
	if h.values == nil {
		h.values = http.Header{}
	}
	h.values.Add(key, value)
}

// Values returns the headers.
func (h *Headers) Values() http.Header { return h.values }

// QueryParams are query parameters from an options object.
type QueryParams struct {
	values url.Values
}

// Append adds a query parameter.
func (q *QueryParams) Append(key, value string) {
	if q.values == nil {
		q.values = url.Values{}
	}
	q.values.Add(key, value)
}

// Values returns the parameters.
func (q *QueryParams) Values() url.Values { return q.values }

// RequestOptions describes one request.
type RequestOptions struct {
	// ContentType is the media type of the request body, when there is one.
	ContentType string
	// ExpectedStatusCodes are the statuses the operation documents; any
	// other is a *StatusError.
	ExpectedStatusCodes []int
	HTTPMethod          string
	// OptionsObject supplies query parameters and headers; nil for none.
	OptionsObject Options
	// Path is the escaped path below BaseURL.
	Path string
	// StreamResponse leaves a successful response's body unread for the
	// caller, for operations that answer a file.
	StreamResponse bool
}

// Request is a request being built.
type Request struct {
	*http.Request

	ExpectedStatusCodes []int
	StreamResponse      bool

	client      *Client
	contentType string
}

// NewRequest builds a request with authentication and the options object
// applied.
func (c *Client) NewRequest(ctx context.Context, input RequestOptions) (*Request, error) {
	u := c.BaseURL + input.Path
	var headers http.Header
	if input.OptionsObject != nil {
		if q := input.OptionsObject.ToQuery(); q != nil && len(q.Values()) > 0 {
			u += "?" + q.Values().Encode()
		}
		if h := input.OptionsObject.ToHeaders(); h != nil {
			headers = h.Values()
		}
	}

	req, err := http.NewRequestWithContext(ctx, input.HTTPMethod, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	c.Authorizer.Authorize(req)
	maps.Copy(req.Header, headers)

	return &Request{Request: req, ExpectedStatusCodes: input.ExpectedStatusCodes, StreamResponse: input.StreamResponse, client: c, contentType: input.ContentType}, nil
}

// Marshal sets the request body to payload as JSON.
func (r *Request) Marshal(payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling request body: %w", err)
	}

	return r.SetBody(bytes.NewReader(b), cmp.Or(r.contentType, "application/json"))
}

// SetBody sets the request body to raw bytes of the given media type, or the
// operation's own when contentType is empty. A nil body sends none. An
// operation that takes a range of types (image/*) needs the concrete one.
func (r *Request) SetBody(body io.Reader, contentType string) error {
	if body == nil {
		return nil
	}
	contentType = cmp.Or(contentType, r.contentType)
	if strings.Contains(contentType, "*") {
		return fmt.Errorf("%s %s: name the body's content type (the operation takes %s)", r.Method, r.URL.Path, contentType)
	}
	r.Body = io.NopCloser(body)
	r.ContentLength = -1
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}

	return nil
}

// Execute sends the request. The response is returned whenever the server
// answered, including with a *StatusError, so the caller can read its status
// and body. Its body is buffered (and readable again) unless the request
// streams a successful response.
func (r *Request) Execute(ctx context.Context) (*Response, error) {
	httpResp, err := r.client.HTTPClient.Do(r.WithContext(ctx)) //nolint:bodyclose // buffered and closed below, or a stream handed to the caller to close
	if err != nil {
		return nil, err
	}
	resp := &Response{Response: httpResp}

	if !slices.Contains(r.ExpectedStatusCodes, httpResp.StatusCode) {
		body, _ := resp.buffer()

		return resp, &StatusError{Method: r.Method, Path: r.URL.Path, StatusCode: httpResp.StatusCode, Body: string(body)}
	}
	if r.StreamResponse {
		return resp, nil
	}
	if _, err := resp.buffer(); err != nil {
		return resp, fmt.Errorf("%s %s: reading response: %w", r.Method, r.URL.Path, err)
	}

	return resp, nil
}

// Response is a server's answer.
type Response struct {
	*http.Response
}

// buffer reads the body into memory, closes the connection's reader and
// replaces it with one over the bytes, so the body can be read again.
func (r *Response) buffer() ([]byte, error) {
	conn := r.Body
	defer func() { _ = conn.Close() }()
	body, err := io.ReadAll(conn)
	r.Body = io.NopCloser(bytes.NewReader(body))

	return body, err
}

// Unmarshal decodes the JSON body into model, leaving the body readable
// again. An empty body leaves model as it is.
func (r *Response) Unmarshal(model any) error {
	body, err := r.buffer()
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, model); err != nil {
		return fmt.Errorf("%s %s: decoding response: %w", r.Request.Method, r.Request.URL.Path, err)
	}

	return nil
}

// StatusError is an answer with a status the operation does not document.
type StatusError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// StatusCode returns the status of a *StatusError in err's chain, or 0.
func StatusCode(err error) int {
	if se, ok := errors.AsType[*StatusError](err); ok {
		return se.StatusCode
	}

	return 0
}

// CSV joins a list parameter into one comma-separated value.
func CSV[T ~string | ~int | ~int64](values []T) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprint(v)
	}

	return strings.Join(parts, ",")
}

// DeepObject renders an object query parameter the deepObject way, one
// parameter per key as name[key]=value, in key order.
func DeepObject[K ~string](name string, value map[K]string) [][2]string {
	out := make([][2]string, 0, len(value))
	for _, k := range slices.Sorted(maps.Keys(value)) {
		out = append(out, [2]string{fmt.Sprintf("%s[%v]", name, k), value[k]})
	}

	return out
}

// JSONObject renders an object query parameter as one JSON string.
func JSONObject[T any](value map[string]T) string {
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}

	return string(b)
}
