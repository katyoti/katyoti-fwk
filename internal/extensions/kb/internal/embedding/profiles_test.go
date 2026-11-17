// SPDX-License-Identifier: Apache-2.0

package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// recorded is one request the fake embeddings server received.
type recorded struct {
	Path, Query, Bearer, APIKey string
	Input                       []string
	Model                       string
}

// fakeServer answers an OpenAI-shaped embeddings request: each text gets the
// vector [len(text), position in the batch], returned in reverse order with
// its index so a client that ignores the index scrambles them.
func fakeServer(t *testing.T) (*httptest.Server, func() []recorded) {
	t.Helper()
	var mu sync.Mutex
	var reqs []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		reqs = append(reqs, recorded{
			Path: r.URL.Path, Query: r.URL.RawQuery,
			Bearer: r.Header.Get("Authorization"), APIKey: r.Header.Get("api-key"),
			Input: body.Input, Model: body.Model,
		})
		mu.Unlock()
		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		var data []item
		for i := len(body.Input) - 1; i >= 0; i-- {
			data = append(data, item{Embedding: []float32{float32(len(body.Input[i])), float32(i)}, Index: i})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv, func() []recorded {
		mu.Lock()
		defer mu.Unlock()
		return append([]recorded(nil), reqs...)
	}
}

func embedder(t *testing.T, p Profile) *httpEmbedder {
	t.Helper()
	ps, err := newProfiles(&EmbeddingProfilesConfig{Default: "p", Profiles: map[string]Profile{"p": p}}, http.DefaultClient)
	if err != nil {
		t.Fatalf("profiles: %v", err)
	}
	e, err := ps.Embedder("")
	if err != nil {
		t.Fatalf("embedder: %v", err)
	}
	return e.(*httpEmbedder)
}

// Each type reaches its endpoint with its auth header and the OpenAI body.
func TestEachTypeSendsItsEndpointAuthAndBody(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	srv, got := fakeServer(t)
	cases := []struct {
		name        string
		p           Profile
		path, query string
		bearer, key string
	}{
		{"openai", Profile{Type: TypeOpenAI, Endpoint: srv.URL + "/v1/embeddings", KeyEnv: "KB_TEST_KEY", Model: "m1", Dimensions: 2},
			"/v1/embeddings", "", "Bearer secret", ""},
		{"foundry", Profile{Type: TypeFoundry, Endpoint: srv.URL + "/openai/v1/embeddings", KeyEnv: "KB_TEST_KEY", Model: "dep", Dimensions: 2},
			"/openai/v1/embeddings", "", "Bearer secret", ""},
		{"azure-openai", Profile{Type: TypeAzureOpenAI, Endpoint: srv.URL + "/openai/deployments/dep/embeddings", APIVersion: "2024-10-21", KeyEnv: "KB_TEST_KEY", Model: "dep", Dimensions: 2},
			"/openai/deployments/dep/embeddings", "api-version=2024-10-21", "", "secret"},
		{"openai-compatible", Profile{Type: TypeOpenAICompatible, Endpoint: srv.URL + "/embed", KeyEnv: "KB_TEST_KEY", Model: "nomic", Auth: AuthAPIKey, Dimensions: 2},
			"/embed", "", "", "secret"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := len(got())
			vecs, err := embedder(t, c.p).Embed(context.Background(), []string{"a", "bb"})
			if err != nil {
				t.Fatalf("embed: %v", err)
			}
			if len(vecs) != 2 || vecs[0][0] != 1 || vecs[1][0] != 2 {
				t.Fatalf("vectors %v, want one per text in order", vecs)
			}
			r := got()[before]
			if r.Path != c.path || r.Query != c.query {
				t.Errorf("request %s?%s, want %s?%s", r.Path, r.Query, c.path, c.query)
			}
			if r.Bearer != c.bearer || r.APIKey != c.key {
				t.Errorf("auth Authorization=%q api-key=%q, want %q / %q", r.Bearer, r.APIKey, c.bearer, c.key)
			}
			if r.Model != c.p.Model || strings.Join(r.Input, ",") != "a,bb" {
				t.Errorf("body model=%q input=%v", r.Model, r.Input)
			}
		})
	}
}

// Embed splits the texts per batch_size and keeps their order.
func TestEmbedBatchesAndKeepsOrder(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	srv, got := fakeServer(t)
	e := embedder(t, Profile{Type: TypeOpenAI, Endpoint: srv.URL, KeyEnv: "KB_TEST_KEY", BatchSize: 2, Dimensions: 2})
	texts := []string{"a", "bb", "ccc", "dddd", "eeeee"}
	vecs, err := e.Embed(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got()); n != 3 {
		t.Errorf("%d requests, want 3 batches of at most 2", n)
	}
	for i, r := range got() {
		if len(r.Input) > 2 {
			t.Errorf("batch %d holds %d texts", i, len(r.Input))
		}
	}
	if len(vecs) != len(texts) {
		t.Fatalf("%d vectors for %d texts", len(vecs), len(texts))
	}
	for i, v := range vecs {
		if int(v[0]) != len(texts[i]) {
			t.Errorf("vector %d is of a text of length %v, want %d", i, v[0], len(texts[i]))
		}
	}
	if d := embedder(t, Profile{Type: TypeOpenAI, Endpoint: srv.URL, KeyEnv: "KB_TEST_KEY"}); d.Dimension() != 1536 || d.Model() != "text-embedding-3-small" {
		t.Errorf("defaults: dimension %d model %q", d.Dimension(), d.Model())
	}
}

// An endpoint returning vectors of another length than the profile declares
// is refused, naming both lengths and the setting to change: indexed, they
// would carry a false dimension the re-index guard trusts.
func TestVectorsOfAnotherDimensionAreRefused(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	srv, _ := fakeServer(t)
	_, err := embedder(t, Profile{Type: TypeFoundry, Endpoint: srv.URL, KeyEnv: "KB_TEST_KEY", Model: "dep", Dimensions: 1536}).Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("2-dimension vectors accepted for a profile of 1536")
	}
	for _, want := range []string{`"p"`, "2 dimensions", "1536", "dimensions to 2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestMissingKeyNamesTheVariableAndProfile(t *testing.T) {
	t.Setenv("KB_TEST_UNSET_KEY", "")
	ps, err := newProfiles(&EmbeddingProfilesConfig{Default: "corp", Profiles: map[string]Profile{
		"corp": {Type: TypeFoundry, Endpoint: "https://x.example/embeddings", KeyEnv: "KB_TEST_UNSET_KEY", Model: "dep"},
	}}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ps.Embedder("")
	if err == nil || !strings.Contains(err.Error(), "KB_TEST_UNSET_KEY") || !strings.Contains(err.Error(), `"corp"`) {
		t.Fatalf("error %v, want one naming KB_TEST_UNSET_KEY and profile corp", err)
	}
}

func TestInvalidProfilesAreRefused(t *testing.T) {
	cases := map[string]struct {
		p    Profile
		want string
	}{
		"unknown type":      {Profile{Type: "cohere", Endpoint: "https://x.example", KeyEnv: "K", Model: "m"}, "unknown type"},
		"literal api_key":   {Profile{Type: TypeOpenAI, APIKey: "sk-123"}, "key_env"},
		"azure no version":  {Profile{Type: TypeAzureOpenAI, Endpoint: "https://x.example", KeyEnv: "K", Model: "m"}, "api_version"},
		"foundry no url":    {Profile{Type: TypeFoundry, KeyEnv: "K", Model: "m"}, "endpoint"},
		"unknown auth":      {Profile{Type: TypeOpenAI, Auth: "basic"}, "auth"},
		"compatible no key": {Profile{Type: TypeOpenAICompatible, Endpoint: "https://x.example", Model: "m"}, "key_env"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newProfiles(&EmbeddingProfilesConfig{Default: "p", Profiles: map[string]Profile{"p": c.p}}, http.DefaultClient)
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), `"p"`) {
				t.Fatalf("error %v, want one naming the profile and %q", err, c.want)
			}
			if err != nil && strings.Contains(err.Error(), "sk-123") {
				t.Errorf("the error repeats the literal key: %v", err)
			}
		})
	}
}

// The service, decoded from a config through the framework decoder, serves
// the default profile and a named one.
func TestServiceResolvesDefaultAndNamedProfile(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	t.Setenv("KB_TEST_AZ", "az")
	yml := []byte(`default: main
profiles:
  main:
    type: openai
    key_env: KB_TEST_KEY
    model: text-embedding-3-small
  big:
    type: azure-openai
    endpoint: https://r.openai.example/openai/deployments/big/embeddings
    api_version: "2024-10-21"
    key_env: KB_TEST_AZ
    model: big
    dimensions: 3072
`)
	var cfg EmbeddingProfilesConfig
	if err := config.DecodeBytes(yml, marshal.YAML, &cfg); err != nil {
		t.Fatal(err)
	}
	svc, err := NewEmbeddingProfiles(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Default() != "main" || strings.Join(svc.Profiles(), ",") != "big,main" {
		t.Fatalf("default %q profiles %v", svc.Default(), svc.Profiles())
	}
	def, err := svc.Embedder("")
	if err != nil || def.Name() != "main" || def.Dimension() != 1536 {
		t.Fatalf("default embedder %v %v", def, err)
	}
	big, err := svc.Embedder("big")
	if err != nil || big.Name() != "big" || big.Model() != "big" || big.Dimension() != 3072 {
		t.Fatalf("named embedder %v %v", big, err)
	}
	if _, err := svc.Embedder("nope"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("undeclared profile: %v", err)
	}
}

func TestEmptyConfigFallsBackToOpenAI(t *testing.T) {
	svc, err := NewEmbeddingProfiles(&EmbeddingProfilesConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if svc.Default() != "openai" {
		t.Fatalf("default %q", svc.Default())
	}
}

// failingTransport fails the test on any request: Check must call no endpoint.
type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("Check called the endpoint %s", r.URL)
	return nil, http.ErrHandlerTimeout
}

// Check reports a profile's type, model, dimensions and key variable, and
// whether the key is set, without calling its endpoint.
func TestCheckReportsAProfileWithoutCallingIt(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	t.Setenv("KB_TEST_UNSET_KEY", "")
	ps, err := newProfiles(&EmbeddingProfilesConfig{Default: "main", Profiles: map[string]Profile{
		"main": {Type: TypeOpenAI, KeyEnv: "KB_TEST_KEY"},
		"corp": {Type: TypeFoundry, Endpoint: "https://x.example/embeddings", KeyEnv: "KB_TEST_UNSET_KEY", Model: "dep", Dimensions: 3072},
	}}, &http.Client{Transport: failingTransport{t}})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]kbapi.EmbeddingProfileCheck{
		"":     {Profile: "main", Type: TypeOpenAI, Model: defaultOpenAIModel, Dimensions: defaultDimensions, KeyEnv: "KB_TEST_KEY", KeySet: true},
		"corp": {Profile: "corp", Type: TypeFoundry, Model: "dep", Dimensions: 3072, KeyEnv: "KB_TEST_UNSET_KEY", KeySet: false},
	}
	for name, want := range cases {
		got, err := ps.Check(name)
		if err != nil {
			t.Fatalf("check %q: %v", name, err)
		}
		if got != want {
			t.Errorf("check %q = %+v; want %+v", name, got, want)
		}
	}
	if _, err := ps.Check("nope"); err == nil || !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "corp, main") {
		t.Fatalf("undeclared profile: %v", err)
	}
}
