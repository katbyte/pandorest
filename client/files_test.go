package client_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/katbyte/pandorest/client"
)

// files is a connection that carries files, recording what it was sent.
type files struct {
	method string
	sent   string
}

func (*files) Call(context.Context, string, []any, any) error { return nil }

func (f *files) Download(_ context.Context, method string, _ []any, w io.Writer) (int64, error) {
	f.method = method
	n, err := io.WriteString(w, "file bytes")

	return int64(n), err
}

func (f *files) Upload(_ context.Context, method string, _ []any, into any, readers ...io.Reader) error {
	f.method = method
	var b strings.Builder
	for _, r := range readers {
		_, _ = io.Copy(&b, r)
	}
	f.sent = b.String()
	if p, ok := into.(*bool); ok {
		*p = true
	}

	return nil
}

// A file goes through the connection's Download or Upload, and a connection without them refuses by name.
func TestFiles(t *testing.T) {
	t.Parallel()

	f := &files{}
	var out bytes.Buffer
	if n, err := client.DownloadWith(t.Context(), f, "config.save", nil, &out); err != nil || n != 10 || out.String() != "file bytes" || f.method != "config.save" {
		t.Errorf("DownloadWith = %d, %v, %q", n, err, out.String())
	}
	var done bool
	if err := client.UploadWith(t.Context(), f, "config.upload", nil, &done, strings.NewReader("a"), strings.NewReader("b")); err != nil || f.sent != "ab" || !done || f.method != "config.upload" {
		t.Errorf("UploadWith = %v, sent %q, done %t", err, f.sent, done)
	}

	plain := client.CallerFunc(func(context.Context, string, []any, any) error { return nil })
	if _, err := client.DownloadWith(t.Context(), plain, "config.save", nil, &out); !errors.Is(err, client.ErrCannotCarryFiles) {
		t.Errorf("a plain connection downloaded: %v", err)
	}
	if err := client.UploadWith(t.Context(), plain, "config.upload", nil, nil); !errors.Is(err, client.ErrCannotCarryFiles) {
		t.Errorf("a plain connection uploaded: %v", err)
	}
}
