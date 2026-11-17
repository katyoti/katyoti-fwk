// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"testing"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/bundling"
	"github.com/katyoti/katyoti-fwk/internal/extensions/kb/internal/embedding"
)

// : a document its source no longer brings (its bundle marked stale
// by the extraction) leaves the knowledge base at the next index - every
// chunk of it deleted and counted, its bundle removed - and a query finds
// nothing of it.
func TestADocumentNoLongerBroughtLeavesTheIndex(t *testing.T) {
	t.Setenv("KB_TEST_KEY", "secret")
	srv, _ := embedServer(t)
	s := serviceWith(t, "main", map[string]embedding.Profile{
		"main": {Type: embedding.TypeOpenAI, Endpoint: srv.URL + "/main", KeyEnv: "KB_TEST_KEY", Model: "m", Dimensions: 3},
	})
	root := newKB(t, "main")
	ctx := context.Background()

	first, err := s.Index(ctx, kbapi.IndexInput{Path: root})
	if err != nil || first.ChunksIndexed == 0 {
		t.Fatalf("index: %+v %v", first, err)
	}
	meta, err := bundling.LoadMetadata(root, "doc-1")
	if err != nil {
		t.Fatal(err)
	}
	meta.Status = "stale" // what the extraction writes when its source dropped it
	if err := bundling.SaveMetadata(root, meta); err != nil {
		t.Fatal(err)
	}

	second, err := s.Index(ctx, kbapi.IndexInput{Path: root})
	if err != nil || len(second.Errors) > 0 {
		t.Fatalf("index again: %+v %v", second, err)
	}
	if second.ChunksDeleted != first.ChunksIndexed {
		t.Errorf("chunks deleted = %d; want the %d its document had", second.ChunksDeleted, first.ChunksIndexed)
	}
	if _, err := os.Stat(bundling.BundleDir(root, "doc-1")); !os.IsNotExist(err) {
		t.Errorf("the stale bundle is still there (%v)", err)
	}
	q, err := s.Query(ctx, kbapi.QueryInput{Path: root, Query: "auth"})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Results) != 0 {
		t.Errorf("a query still finds the dropped document: %+v", q.Results)
	}
}
