// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
)

// roundTripPayload is a tiny request/response shape used by the round-trip
// tests to verify JSON marshalling end-to-end.
type roundTripPayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestClient_PostJSON_RoundTrip(t *testing.T) {
	const want = "schema-001"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var in roundTripPayload
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if in.Name != want {
			t.Errorf("request.Name = %q, want %q", in.Name, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(roundTripPayload{Name: in.Name, Count: in.Count + 1})
	}))
	defer srv.Close()

	c := newHTTPClient(NewStaticEndpoint(srv.URL, ""))
	var out roundTripPayload
	if err := c.PostJSON(context.Background(), "/api/parse", roundTripPayload{Name: want, Count: 41}, &out); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if out.Name != want || out.Count != 42 {
		t.Errorf("response = %+v, want {Name:%q Count:42}", out, want)
	}
}

func TestClient_Get_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(roundTripPayload{Name: "ok", Count: 7})
	}))
	defer srv.Close()

	c := newHTTPClient(NewStaticEndpoint(srv.URL, ""))
	var out roundTripPayload
	if err := c.Get(context.Background(), "/api/types", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.Count != 7 {
		t.Errorf("Count = %d, want 7", out.Count)
	}
}

func TestClient_AuthHeader_PresentWhenTokenSet(t *testing.T) {
	const token = "abc123"
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newHTTPClient(NewStaticEndpoint(srv.URL, token))
	if err := c.Get(context.Background(), "/api/types", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := "Bearer " + token; seen != want {
		t.Errorf("Authorization = %q, want %q", seen, want)
	}
}

func TestClient_AuthHeader_AbsentWhenTokenEmpty(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newHTTPClient(NewStaticEndpoint(srv.URL, ""))
	if err := c.Get(context.Background(), "/api/types", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if seen != "" {
		t.Errorf("Authorization = %q, want empty (no token)", seen)
	}
}

func TestClient_RetryOn5xx_EventualSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			http.Error(w, `{"error":"transient"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(roundTripPayload{Name: "ok", Count: 1})
	}))
	defer srv.Close()

	c := newHTTPClient(
		NewStaticEndpoint(srv.URL, ""),
		withMaxRetry(3),
		withBackoff(1*time.Millisecond),
	)
	var out roundTripPayload
	if err := c.PostJSON(context.Background(), "/api/parse", roundTripPayload{Name: "x"}, &out); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("server hits = %d, want 3 (2 failures + 1 success)", got)
	}
}

func TestClient_RetryOn5xx_Exhausted(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, `{"error":"persistent"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newHTTPClient(
		NewStaticEndpoint(srv.URL, ""),
		withMaxRetry(2),
		withBackoff(1*time.Millisecond),
	)
	err := c.PostJSON(context.Background(), "/api/parse", roundTripPayload{}, nil)
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	var be *BridgeError
	if !errors.As(err, &be) {
		t.Fatalf("err type = %T, want *BridgeError", err)
	}
	if !be.IsServerError() {
		t.Errorf("IsServerError = false, want true (status %d)", be.Status)
	}
	// 1 initial + 2 retries = 3 attempts total
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("server hits = %d, want 3 (1 initial + 2 retries)", got)
	}
}

func TestClient_NoRetryOn4xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	c := newHTTPClient(
		NewStaticEndpoint(srv.URL, ""),
		withMaxRetry(3),
		withBackoff(1*time.Millisecond),
	)
	err := c.PostJSON(context.Background(), "/api/parse", roundTripPayload{}, nil)
	if err == nil {
		t.Fatal("expected error on 400")
	}
	var be *BridgeError
	if !errors.As(err, &be) {
		t.Fatalf("err type = %T, want *BridgeError", err)
	}
	if be.Status != 400 {
		t.Errorf("Status = %d, want 400", be.Status)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("server hits = %d, want 1 (no retry on 4xx)", got)
	}
}

func TestClient_PostNoRetryByDefault(t *testing.T) {
	// Plain Post (no WithRetry) should NOT retry — even on 5xx — because
	// the slot may be non-idempotent.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, `{"error":"oops"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newHTTPClient(
		NewStaticEndpoint(srv.URL, ""),
		withMaxRetry(3),
		withBackoff(1*time.Millisecond),
	)
	err := c.Post(context.Background(), "/api/install-schema", roundTripPayload{}, nil)
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("server hits = %d, want 1 (Post default = no retry)", got)
	}
}

func TestClient_PostWithRetryOpt(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, `{"error":"oops"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newHTTPClient(
		NewStaticEndpoint(srv.URL, ""),
		withMaxRetry(2),
		withBackoff(1*time.Millisecond),
	)
	err := c.Post(context.Background(), "/api/parse", roundTripPayload{}, nil, WithRetry())
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("server hits = %d, want 3 (1 initial + 2 retries)", got)
	}
}

func TestClient_ErrorMapping_Diagnostics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		body := map[string]any{
			"error": "validation failed",
			"diagnostics": []map[string]any{
				{"severity": "error", "code": "missing_field", "message": "name is required", "path": "types.User.fields.name"},
			},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	c := newHTTPClient(NewStaticEndpoint(srv.URL, ""))
	err := c.PostJSON(context.Background(), "/api/validate", roundTripPayload{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var be *BridgeError
	if !errors.As(err, &be) {
		t.Fatalf("err type = %T, want *BridgeError", err)
	}
	if be.Status != http.StatusUnprocessableEntity {
		t.Errorf("Status = %d, want 422", be.Status)
	}
	if be.Message != "validation failed" {
		t.Errorf("Message = %q, want %q", be.Message, "validation failed")
	}
	if len(be.Diagnostics) != 1 {
		t.Fatalf("len(Diagnostics) = %d, want 1", len(be.Diagnostics))
	}
	d := be.Diagnostics[0]
	if d.Severity != api.SeverityError || d.Code != "missing_field" || d.Path != "types.User.fields.name" {
		t.Errorf("Diagnostic = %+v, want error/missing_field at types.User.fields.name", d)
	}
}

func TestClient_ErrorMapping_NotImplemented(t *testing.T) {
	// Phase 1 normal state: bun returns 501 for stub routes. Although 501
	// is in the 5xx range, retrying does not help (the handler stays
	// unimplemented), so the client must NOT retry.
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = io.WriteString(w, `{"error":"not implemented","detail":"see codegen-target-* todos"}`)
	}))
	defer srv.Close()

	c := newHTTPClient(
		NewStaticEndpoint(srv.URL, ""),
		withMaxRetry(3),
		withBackoff(1*time.Millisecond),
	)
	err := c.PostJSON(context.Background(), "/api/gen/json-schema", roundTripPayload{}, nil)
	var be *BridgeError
	if !errors.As(err, &be) {
		t.Fatalf("err type = %T, want *BridgeError", err)
	}
	if !be.IsNotImplemented() {
		t.Errorf("IsNotImplemented = false, want true")
	}
	if be.IsTransient() {
		t.Errorf("IsTransient = true, want false (501 is permanent in Phase 1)")
	}
	if be.Detail != "see codegen-target-* todos" {
		t.Errorf("Detail = %q, want detail preserved", be.Detail)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("server hits = %d, want 1 (501 must not be retried)", got)
	}
}

func TestClient_ErrorMapping_NonJSONBody(t *testing.T) {
	// Some bun error paths emit plain text (e.g. early-stage routes); the
	// client should still produce a BridgeError with the body in Detail.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream went away")
	}))
	defer srv.Close()

	c := newHTTPClient(
		NewStaticEndpoint(srv.URL, ""),
		withMaxRetry(0),
		withBackoff(1*time.Millisecond),
	)
	err := c.PostJSON(context.Background(), "/api/parse", roundTripPayload{}, nil)
	var be *BridgeError
	if !errors.As(err, &be) {
		t.Fatalf("err type = %T, want *BridgeError", err)
	}
	if be.Status != http.StatusBadGateway {
		t.Errorf("Status = %d, want 502", be.Status)
	}
	if be.Detail != "upstream went away" {
		t.Errorf("Detail = %q, want plain-text body preserved", be.Detail)
	}
}

func TestClient_EndpointUnavailable(t *testing.T) {
	c := newHTTPClient(NewStaticEndpoint("", ""))
	err := c.Get(context.Background(), "/api/types", nil)
	if !errors.Is(err, ErrEndpointUnavailable) {
		t.Errorf("err = %v, want ErrEndpointUnavailable", err)
	}
}

func TestClient_ContextCancel_Aborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block long enough that ctx cancel beats the response.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newHTTPClient(NewStaticEndpoint(srv.URL, ""))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.Get(ctx, "/api/types", nil)
	if err == nil {
		t.Fatal("expected ctx error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		// Transport-wrapped context errors are also acceptable
		if !isContextError(err) {
			t.Errorf("err = %v, want context error", err)
		}
	}
}

// isContextError reports whether err mentions a context deadline/cancel,
// since the transport may wrap them.
func isContextError(err error) bool {
	s := fmt.Sprint(err)
	return contains(s, "context") || contains(s, "deadline") || contains(s, "canceled")
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestEnvEndpoint_ReadsEnv(t *testing.T) {
	t.Setenv("KTW_DISPLAY_PORT", "12345")
	t.Setenv("KTW_DISPLAY_TOKEN", "tokz")

	e := EnvEndpoint{}
	if got, want := e.BaseURL(), "http://127.0.0.1:12345"; got != want {
		t.Errorf("BaseURL = %q, want %q", got, want)
	}
	if got := e.Token(); got != "tokz" {
		t.Errorf("Token = %q, want tokz", got)
	}
}

func TestEnvEndpoint_EmptyWhenUnset(t *testing.T) {
	t.Setenv("KTW_DISPLAY_PORT", "")
	t.Setenv("KTW_DISPLAY_TOKEN", "")

	e := EnvEndpoint{}
	if got := e.BaseURL(); got != "" {
		t.Errorf("BaseURL = %q, want empty", got)
	}
	if got := e.Token(); got != "" {
		t.Errorf("Token = %q, want empty", got)
	}
}

func TestStaticEndpoint_Set(t *testing.T) {
	e := NewStaticEndpoint("http://a.test:1", "t1")
	if e.BaseURL() != "http://a.test:1" || e.Token() != "t1" {
		t.Fatalf("initial state wrong: %s / %s", e.BaseURL(), e.Token())
	}
	e.Set("http://b.test:2", "t2")
	if e.BaseURL() != "http://b.test:2" || e.Token() != "t2" {
		t.Errorf("after Set: %s / %s, want http://b.test:2 / t2", e.BaseURL(), e.Token())
	}
}
