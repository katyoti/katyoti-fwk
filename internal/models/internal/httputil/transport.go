// SPDX-License-Identifier: Apache-2.0

package httputil

import (
	"net/http"
)

var (
	// DefaultTransport wraps [http.DefaultTransport] and adds a User-Agent header.
	DefaultTransport http.RoundTripper = &Transport{
		Transport: http.DefaultTransport,
	}

	// DefaultClient is the default HTTP client for LLM provider requests.
	DefaultClient = &http.Client{
		Transport: DefaultTransport,
	}
)

// Transport is an [http.RoundTripper] that adds a User-Agent header
// to outgoing HTTP requests.
type Transport struct {
	Transport http.RoundTripper
}

// RoundTrip adds the User-Agent header and delegates to the underlying transport.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	newReq := req.Clone(req.Context())
	ua := UserAgent()
	if existingUA := req.Header.Get("User-Agent"); existingUA != "" {
		newReq.Header.Set("User-Agent", existingUA+" "+ua)
	} else {
		newReq.Header.Set("User-Agent", ua)
	}
	return transport.RoundTrip(newReq)
}

// ApiKeyTransport adds API keys to URL query parameters.
// Used with Google APIs and other services accepting keys as query params.
type ApiKeyTransport struct {
	Transport http.RoundTripper
	APIKey    string
}

// RoundTrip adds the API key as a "key" query parameter if not already present.
func (t *ApiKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	newReq := req.Clone(req.Context())
	q := newReq.URL.Query()
	if q.Get("key") == "" && t.APIKey != "" {
		q.Set("key", t.APIKey)
		newReq.URL.RawQuery = q.Encode()
	}
	return transport.RoundTrip(newReq)
}
