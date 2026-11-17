// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/bundling"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/embedding"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/indexing"
)

// embedServer is a fake OpenAI-shaped embeddings endpoint counting the
// requests per path; every text gets a 3-dimension vector.
func embedServer(t *testing.T) (*httptest.Server, func(path string) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		type item struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		data := make([]item, len(body.Input))
		for i, s := range body.Input {
			data[i] = item{Embedding: []float32{float32(len(s)), 1, 0.5}, Index: i}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv, func(path string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[path]
	}
}

// newKB writes a knowledge base naming the given profile, with one
// extracted bundle, under a temporary folder.
func newKB(t *testing.T, profile string) string {
	t.Helper()
	root := t.TempDir()
	kb := "name: testkb\nsources: []\nindexing:\n  backend: chromem-go\n  embedding:\n    profile: " + profile + "\n"
	if err := os.WriteFile(filepath.Join(root, "KnowledgeBase.yaml"), []byte(kb), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := &bundling.BundleMetadata{
		Slug: "doc-1", Status: "extracted",
		Source:     bundling.SourceMeta{Path: "local/doc.md", Hash: "sha256:1"},
		Extraction: bundling.ExtractionMeta{Profile: "fast", ExtractedAt: time.Now()},
		Content:    []bundling.ContentEntry{{File: "content.md", Part: "whole"}},
	}
	if err := bundling.SaveMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
	text := "# Authentication\n\nWe decided to use OAuth for every service of the platform.\n"
	if err := os.WriteFile(filepath.Join(bundling.BundleDir(root, "doc-1"), "content.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// serviceWith is a KBService whose EmbeddingProfiles is built from profiles.
func serviceWith(t *testing.T, def string, profiles map[string]embedding.Profile) *kbService {
	t.Helper()
	ps, err := embedding.NewEmbeddingProfiles(&embedding.EmbeddingProfilesConfig{Default: def, Profiles: profiles})
	if err != nil {
		t.Fatal(err)
	}
	return &kbService{cfg: &KBServiceConfig{}, profiles: func(context.Context) (kbapi.EmbeddingProfiles, error) { return ps, nil }}
}

// A knowledge base naming a profile embeds through it at index and query time.
func TestKBEmbedsThroughTheProfileItNames(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	srv, hits := embedServer(t)
	s := serviceWith(t, "main", map[string]embedding.Profile{
		"main": {Type: embedding.TypeOpenAI, Endpoint: srv.URL + "/main", KeyEnv: "KB_TEST_KEY", Model: "m", Dimensions: 3},
		"corp": {Type: embedding.TypeFoundry, Endpoint: srv.URL + "/corp", KeyEnv: "KB_TEST_KEY", Model: "m", Dimensions: 3},
	})
	root := newKB(t, "corp")
	ctx := context.Background()

	out, err := s.Index(ctx, kbapi.IndexInput{Path: root})
	if err != nil || out.ChunksIndexed == 0 || len(out.Errors) > 0 {
		t.Fatalf("index: %+v %v", out, err)
	}
	if hits("/corp") == 0 || hits("/main") != 0 {
		t.Fatalf("index embedded corp=%d main=%d, want only through corp", hits("/corp"), hits("/main"))
	}
	before := hits("/corp")
	q, err := s.Query(ctx, kbapi.QueryInput{Path: root, Query: "auth"})
	if err != nil || len(q.Results) == 0 {
		t.Fatalf("query: %+v %v", q, err)
	}
	if hits("/corp") != before+1 || hits("/main") != 0 {
		t.Fatalf("query embedded corp=%d main=%d", hits("/corp")-before, hits("/main"))
	}
}

// A query through a profile of another model or dimension is refused saying
// to re-index; the same model and dimensions from another type is accepted.
func TestQueryRefusesAnIndexOfAnotherModelOrDimension(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	srv, _ := embedServer(t)
	profiles := map[string]embedding.Profile{
		"openai":   {Type: embedding.TypeOpenAI, Endpoint: srv.URL + "/a", KeyEnv: "KB_TEST_KEY", Model: "m", Dimensions: 3},
		"foundry":  {Type: embedding.TypeFoundry, Endpoint: srv.URL + "/b", KeyEnv: "KB_TEST_KEY", Model: "m", Dimensions: 3},
		"othermod": {Type: embedding.TypeOpenAI, Endpoint: srv.URL + "/c", KeyEnv: "KB_TEST_KEY", Model: "m2", Dimensions: 3},
		"otherdim": {Type: embedding.TypeOpenAI, Endpoint: srv.URL + "/d", KeyEnv: "KB_TEST_KEY", Model: "m", Dimensions: 4},
	}
	root := newKB(t, "openai")
	ctx := context.Background()
	if _, err := serviceWith(t, "openai", profiles).Index(ctx, kbapi.IndexInput{Path: root}); err != nil {
		t.Fatal(err)
	}
	query := func(profile string) error {
		_, err := serviceWith(t, profile, profiles).Query(ctx, kbapi.QueryInput{Path: root, Query: "auth"})
		return err
	}
	// The KB names openai; the service default stands in for other profiles
	// once the KB names none.
	rewrite := func() {
		kb := "name: testkb\nsources: []\nindexing:\n  backend: chromem-go\n"
		if err := os.WriteFile(filepath.Join(root, "KnowledgeBase.yaml"), []byte(kb), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rewrite()
	if err := query("foundry"); err != nil {
		t.Errorf("same model and dimensions from another type refused: %v", err)
	}
	for _, p := range []string{"othermod", "otherdim"} {
		err := query(p)
		if err == nil || !strings.Contains(err.Error(), "re-index") || !strings.Contains(err.Error(), "m/3") {
			t.Errorf("query through %s: %v, want a refusal naming m/3 and re-index", p, err)
		}
	}
	// Index refuses too, until forced.
	if _, err := serviceWith(t, "othermod", profiles).Index(ctx, kbapi.IndexInput{Path: root}); err == nil || !strings.Contains(err.Error(), "re-index") {
		t.Errorf("index through othermod over an m/3 index: %v, want a refusal", err)
	}
	if _, err := serviceWith(t, "othermod", profiles).Index(ctx, kbapi.IndexInput{Path: root, Force: true}); err != nil {
		t.Fatalf("forced re-index: %v", err)
	}
	if err := query("othermod"); err != nil {
		t.Errorf("query after the forced re-index: %v", err)
	}
}

// An index built before profiles recorded "openai/<model>" and no
// dimensions: it is compared on its model only.
func TestOldIndexIsComparedOnItsModel(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	srv, _ := embedServer(t)
	root := newKB(t, "")
	idx, err := indexing.NewChromemIndexer(map[string]any{"path": indexing.StoragePath(root)})
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Init(context.Background(), "testkb", 1536, map[string]string{"kb_name": "testkb", "embedding_model": "openai/%!s(<nil>)"}); err != nil {
		t.Fatal(err)
	}
	profiles := map[string]embedding.Profile{
		"small": {Type: embedding.TypeFoundry, Endpoint: srv.URL, KeyEnv: "KB_TEST_KEY", Model: "text-embedding-3-small", Dimensions: 3},
		"large": {Type: embedding.TypeOpenAI, Endpoint: srv.URL, KeyEnv: "KB_TEST_KEY", Model: "text-embedding-3-large", Dimensions: 3072},
	}
	ctx := context.Background()
	if _, err := serviceWith(t, "small", profiles).Query(ctx, kbapi.QueryInput{Path: root, Query: "auth"}); err != nil {
		t.Errorf("old index of the default model refused: %v", err)
	}
	if _, err := serviceWith(t, "large", profiles).Query(ctx, kbapi.QueryInput{Path: root, Query: "auth"}); err == nil || !strings.Contains(err.Error(), "re-index") {
		t.Errorf("old index queried with another model: %v, want a refusal", err)
	}
}

// A knowledge base's status names the model of the profile it embeds with;
// a profile whose key is unset leaves the status readable.
func TestStatusNamesTheProfilesModel(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	s := serviceWith(t, "main", map[string]embedding.Profile{
		"main":  {Type: embedding.TypeOpenAI, Endpoint: "http://127.0.0.1:1/main", KeyEnv: "KB_TEST_KEY", Model: "m-main", Dimensions: 3},
		"corp":  {Type: embedding.TypeFoundry, Endpoint: "http://127.0.0.1:1/corp", KeyEnv: "KB_TEST_KEY", Model: "m-corp", Dimensions: 3},
		"nokey": {Type: embedding.TypeFoundry, Endpoint: "http://127.0.0.1:1/nokey", KeyEnv: "KB_TEST_UNSET_KEY", Model: "m-nokey", Dimensions: 3},
	})
	for profile, want := range map[string]string{"corp": "m-corp", "": "m-main", "nokey": ""} {
		out, err := s.Status(context.Background(), kbapi.StatusInput{Path: newKB(t, profile)})
		if err != nil {
			t.Fatalf("status of a KB on %q: %v", profile, err)
		}
		if out.Index.EmbeddingModel != want {
			t.Errorf("status of a KB on %q names model %q; want %q", profile, out.Index.EmbeddingModel, want)
		}
	}
}

// The embedding check names the profile a knowledge base would embed with and
// fails naming the key variable when it is unset.
func TestEmbeddingCheckFailsNamingTheUnsetKey(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	t.Setenv("KB_TEST_UNSET_KEY", "")
	s := serviceWith(t, "main", map[string]embedding.Profile{
		"main":  {Type: embedding.TypeOpenAI, Endpoint: "http://127.0.0.1:1/main", KeyEnv: "KB_TEST_KEY", Model: "m-main", Dimensions: 3},
		"nokey": {Type: embedding.TypeFoundry, Endpoint: "http://127.0.0.1:1/nokey", KeyEnv: "KB_TEST_UNSET_KEY", Model: "m-nokey", Dimensions: 3},
	})
	c, err := s.EmbeddingCheck(context.Background(), kbapi.EmbeddingCheckInput{})
	if err != nil || c.Profile != "main" || c.Model != "m-main" || !c.KeySet {
		t.Fatalf("default profile check %+v %v", c, err)
	}
	c, err = s.EmbeddingCheck(context.Background(), kbapi.EmbeddingCheckInput{Profile: "nokey"})
	if err == nil || !strings.Contains(err.Error(), "KB_TEST_UNSET_KEY") || !strings.Contains(err.Error(), `"nokey"`) {
		t.Fatalf("unset key: error %v, want one naming KB_TEST_UNSET_KEY and profile nokey", err)
	}
	if c.KeyEnv != "KB_TEST_UNSET_KEY" || c.KeySet {
		t.Errorf("unset key check %+v", c)
	}
}
