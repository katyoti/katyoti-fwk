// SPDX-License-Identifier: Apache-2.0

package httputil

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strings"
	"time"
)

// DumpRoundTrip writes an HTTP-level dump of one model call to w and
// forwards the request via next. Credential headers are scrubbed from
// the request dump. Response handling:
//
//   - fullBody + SSE response: the status line is logged and the body
//     is teed into w line-by-line as the consumer reads it.
//   - fullBody + non-SSE: the full response (headers + body) is dumped.
//   - !fullBody: only non-2xx responses are dumped (cheap debug mode).
//
// Shared by the per-session llm-model-calls.log observer and the
// KTW_LLM_HTTP_DEBUG stderr switch, across provider SDKs (the Stainless
// middleware shape is identical modulo the package-local Next type).
func DumpRoundTrip(w io.Writer, fullBody bool, req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	ts := time.Now().UTC().Format(time.RFC3339)
	if dump, err := httputil.DumpRequestOut(req, true); err == nil {
		fmt.Fprintf(w, "── request %s %s ──\n%s\n", ts, req.URL.Path, ScrubAuthHeaders(dump))
	}
	resp, err := next(req)
	if err != nil {
		fmt.Fprintf(w, "── transport error %s: %v\n", ts, err)
		return resp, err
	}
	switch {
	case fullBody && strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream"):
		fmt.Fprintf(w, "── response %s (SSE stream follows) ──\n", resp.Status)
		resp.Body = teeBody(resp.Body, w)
	case fullBody || resp.StatusCode >= 400:
		if dump, derr := httputil.DumpResponse(resp, true); derr == nil {
			fmt.Fprintf(w, "── response %s ──\n%s\n", resp.Status, dump)
		}
	}
	return resp, nil
}

// authHeaderRe matches credential headers in a raw HTTP dump.
var authHeaderRe = regexp.MustCompile(`(?mi)^(x-api-key|authorization|api-key):.*$`)

// ScrubAuthHeaders masks credential header values in a raw HTTP dump.
func ScrubAuthHeaders(dump []byte) []byte {
	return authHeaderRe.ReplaceAll(dump, []byte("$1: ***"))
}

// teeBody mirrors everything read from rc into w, preserving Close.
func teeBody(rc io.ReadCloser, w io.Writer) io.ReadCloser {
	return &teeReadCloser{Reader: io.TeeReader(rc, w), closer: rc}
}

type teeReadCloser struct {
	io.Reader
	closer io.Closer
}

func (t *teeReadCloser) Close() error { return t.closer.Close() }
