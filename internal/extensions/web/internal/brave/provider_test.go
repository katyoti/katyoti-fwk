// SPDX-License-Identifier: Apache-2.0

package brave

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webapi "github.com/katyoti/katyoti-fwk/internal/extensions/web/api"
)

func TestNewProvider_MissingAPIKey(t *testing.T) {
	_, err := NewProvider(webapi.BraveConfig{})
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Errorf("err = %v, want api_key error", err)
	}
}

func TestNewProvider_Defaults(t *testing.T) {
	p, err := NewProvider(webapi.BraveConfig{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if p.count != 10 {
		t.Errorf("count = %d, want 10", p.count)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	p, _ := NewProvider(webapi.BraveConfig{APIKey: "test"})
	_, err := p.Search(context.Background(), webapi.SearchInput{Query: ""})
	if err == nil || !strings.Contains(err.Error(), "query") {
		t.Errorf("err = %v, want query required", err)
	}
}

func testProvider(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := NewProvider(webapi.BraveConfig{APIKey: "test-key", TimeoutSeconds: 5, ResultCount: 10})
	if err != nil {
		t.Fatal(err)
	}
	p.endpoint = srv.URL
	return p
}

func TestSearch_Success(t *testing.T) {
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "test-key" {
			t.Error("missing or wrong API key header")
		}
		if r.URL.Query().Get("q") != "golang testing" {
			t.Errorf("query = %q", r.URL.Query().Get("q"))
		}
		resp := braveResponse{}
		resp.Web.Results = []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		}{
			{Title: "Go Testing", URL: "https://go.dev/doc/testing", Description: "Testing in Go"},
			{Title: "Go Blog", URL: "https://go.dev/blog", Description: "Go blog posts"},
		}
		json.NewEncoder(w).Encode(resp)
	})

	out, err := p.Search(context.Background(), webapi.SearchInput{Query: "golang testing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(out.Results))
	}
	if out.Results[0].Title != "Go Testing" {
		t.Errorf("title = %q", out.Results[0].Title)
	}
	if out.Results[1].URL != "https://go.dev/blog" {
		t.Errorf("url = %q", out.Results[1].URL)
	}
}

func TestSearch_HTTPError(t *testing.T) {
	p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	})

	_, err := p.Search(context.Background(), webapi.SearchInput{Query: "test"})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("err = %v, want HTTP 429", err)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(braveResponse{})
	})

	out, err := p.Search(context.Background(), webapi.SearchInput{Query: "obscure query"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != 0 {
		t.Errorf("results = %d, want 0", len(out.Results))
	}
}

func TestSearch_MalformedJSON(t *testing.T) {
	p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json"))
	})

	_, err := p.Search(context.Background(), webapi.SearchInput{Query: "test"})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("err = %v, want decode error", err)
	}
}

func TestSearch_CountPassedToQuery(t *testing.T) {
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("count") != "5" {
			t.Errorf("count = %q, want 5", r.URL.Query().Get("count"))
		}
		json.NewEncoder(w).Encode(braveResponse{})
	})

	_, err := p.Search(context.Background(), webapi.SearchInput{Query: "test", Count: 5})
	if err != nil {
		t.Fatal(err)
	}
}
