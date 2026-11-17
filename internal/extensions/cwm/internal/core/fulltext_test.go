// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

func fulltextFixture(t *testing.T) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "content"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "note", Capabilities: []string{"container", "content"}},
		{Name: "memo", Capabilities: []string{"container", "content"}},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	mk := func(id, typ, parent, title, body string) {
		o := api.Object{ID: id, Type: typ, Name: title, Parent: parent}
		if body != "" {
			o.Body = body
		}
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("m.root", "note", "", "Root", "")
	mk("m.cache", "note", "m.root", "Cache invalidation", "When the index goes stale the reader rebuilds it.")
	mk("m.body", "note", "m.root", "Reader notes", "Every write updates the cache so a read never waits; cache entries expire.")
	mk("m.memo", "memo", "", "Décision sur la mémoire", "Le cache garde la décision prise.")
	mk("m.other", "note", "", "Unrelated", "Nothing about it here.")
	return svc, ctx
}

func ftSearch(t *testing.T, svc *Service, ctx stdctx.Context, in api.ObjectSearchInput) api.ObjectSearchOutput {
	t.Helper()
	in.Mode = api.SearchFullText
	out, err := svc.ObjectSearch(ctx, in)
	if err != nil {
		t.Fatalf("search %q: %v", in.Query, err)
	}
	return out
}

func hitIDs(out api.ObjectSearchOutput) string {
	ids := make([]string, len(out.Hits))
	for i, h := range out.Hits {
		ids[i] = h.ID
	}
	return strings.Join(ids, ",")
}

// topName is the title of the object that leads a search, "" for none.
func topName(out api.ObjectSearchOutput) string {
	if len(out.Objects) == 0 {
		return ""
	}
	return out.Objects[0].Name
}

// TestFullTextRanksAndSaysWhy: the body is searched (the substring mode does
// not), a word in the title outranks the same word in a body, more of the
// query's words outrank a higher score on fewer, and each hit names its field.
func TestFullTextRanksAndSaysWhy(t *testing.T) {
	svc, ctx := fulltextFixture(t)

	out := ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "cache"})
	if got := hitIDs(out); !strings.HasPrefix(got, "m.cache,") || !strings.Contains(got, "m.body") {
		t.Fatalf("cache: %s — want the title match first, the body matches after", got)
	}
	if out.Hits[0].Field != "name" {
		t.Errorf("the title hit names field %q", out.Hits[0].Field)
	}
	for _, h := range out.Hits {
		if h.ID == "m.body" && (h.Field != "body" || !strings.Contains(strings.ToLower(h.Snippet), "cache")) {
			t.Errorf("the body hit: field %q snippet %q", h.Field, h.Snippet)
		}
	}
	if len(out.Objects) != len(out.Hits) || out.Objects[0].ID != out.Hits[0].ID {
		t.Errorf("objects and hits are not in the same order")
	}

	// Two words: the one object holding both comes first — even against one
	// that holds a single rare word in its title, which BM25 alone scores
	// higher.
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "cache expire"})); !strings.HasPrefix(got, "m.body,") {
		t.Fatalf("cache expire: %s — want m.body (both words) first", got)
	}
	for id, text := range map[string][2]string{
		"m.airship": {"Zeppelin", "Zeppelin zeppelin zeppelin."},
		"m.long":    {"A long note", "This long note wanders through many unrelated words before it names a zeppelin once and a cache once and then goes on for a while longer about other matters entirely."},
	} {
		o := api.Object{ID: id, Type: "note", Name: text[0], Body: text[1]}
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	out = ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "zeppelin cache"})
	if got := hitIDs(out); !strings.HasPrefix(got, "m.long,m.airship") {
		t.Fatalf("zeppelin cache: %s — want m.long (both words) before m.airship (one word, scored higher)", got)
	}
	if out.Hits[0].Score >= out.Hits[1].Score {
		t.Fatalf("the case does not test coverage: m.long %.3f >= m.airship %.3f", out.Hits[0].Score, out.Hits[1].Score)
	}
	// The substring mode never reads a body.
	sub, _ := svc.ObjectSearch(ctx, api.ObjectSearchInput{Query: "expire"})
	if sub.Total != 0 {
		t.Errorf("substring mode found a body word: %d", sub.Total)
	}
}

func TestFullTextFoldsAccentsAndMatchesPrefixes(t *testing.T) {
	svc, ctx := fulltextFixture(t)
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "memoire decision"})); !strings.HasPrefix(got, "m.memo") {
		t.Fatalf("memoire decision: %s — want m.memo (accents folded)", got)
	}
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "invalid"})); got != "m.cache" {
		t.Fatalf("invalid: %s — want m.cache (a prefix of invalidation)", got)
	}
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "in"})); strings.Contains(got, "m.cache") {
		t.Fatalf("a two-letter word matched as a prefix: %s", got)
	}
}

func TestFullTextFiltersAndScope(t *testing.T) {
	svc, ctx := fulltextFixture(t)
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "cache", Within: "m.root"})); strings.Contains(got, "m.memo") || !strings.Contains(got, "m.body") {
		t.Fatalf("within m.root: %s", got)
	}
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "cache", Types: []string{"memo"}})); got != "m.memo" {
		t.Fatalf("types memo: %s", got)
	}
	out := ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "cache", Limit: 1})
	if len(out.Hits) != 1 || out.Total < 3 {
		t.Fatalf("limit 1: %d hits, total %d", len(out.Hits), out.Total)
	}
	if _, err := svc.ObjectSearch(ctx, api.ObjectSearchInput{Mode: api.SearchFullText}); err == nil {
		t.Error("an empty fulltext query was accepted")
	}
	if _, err := svc.ObjectSearch(ctx, api.ObjectSearchInput{Query: "x", Mode: "fuzzy"}); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

// TestFullTextFollowsTheWrites: an update replaces an object's words, a delete
// removes them — no rescan.
func TestFullTextFollowsTheWrites(t *testing.T) {
	svc, ctx := fulltextFixture(t)
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.other"})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Body = "Now about zeppelins."
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "zeppelins"})); got != "m.other" {
		t.Fatalf("after the update, zeppelins: %q", got)
	}
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "nothing"})); got != "" {
		t.Fatalf("the replaced words still match: %q", got)
	}
	if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "m", ID: "m.other"}); err != nil {
		t.Fatal(err)
	}
	if got := hitIDs(ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "zeppelins"})); got != "" {
		t.Fatalf("a deleted object still matches: %q", got)
	}
	if _, kept := svc.index().ft.docs["m.other"]; kept {
		t.Fatal("a deleted object's words are still in the index")
	}
}

// TestFullTextOnTheRealCorpus: over the repository's objects, the goal titled
// "knowledge cannot rot silently" leads that query, and the goal titled
// "production scales through an agent hierarchy over deterministic tools" leads
// a paraphrase of its title. A query answers well under a second.
func TestFullTextOnTheRealCorpus(t *testing.T) {
	long.Test(t)
	cfg, _ := realCorpusConfig(t)
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	out := ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "knowledge cannot rot silently", Limit: 1})
	if got := topName(out); got != "Knowledge cannot rot silently" {
		t.Fatalf("knowledge cannot rot silently: %s leads (%s)", got, hitIDs(out))
	}
	// The query's cost is its fastest of five: one alone can wait on a
	// machine busy with the rest of the suite, which is not the index's time.
	// Alone it answers in about 100 ms; the 1 s budget holds under a loaded
	// go test ./... (the best of five reached 320 ms there) and still catches
	// a change in kind — a scan of the corpus per query takes seconds.
	var took time.Duration
	for i := 0; i < 5; i++ {
		start := time.Now()
		out = ftSearch(t, svc, ctx, api.ObjectSearchInput{Query: "hierarchy of agents over deterministic tools", Limit: 1})
		if d := time.Since(start); i == 0 || d < took {
			took = d
		}
	}
	if got := topName(out); got != "Production scales through an agent hierarchy over deterministic tools" {
		t.Fatalf("hierarchy of agents over deterministic tools: %s leads (%s)", got, hitIDs(out))
	}
	if took > time.Second {
		t.Errorf("a query over the corpus took %s at best of five", took)
	}
	t.Logf("%s in %s: %q (query %s)", out.Hits[0].ID, out.Hits[0].Field, out.Hits[0].Snippet, took)
}

// A full-text search never re-enters the index under its own read lock: with
// writers waiting for the write lock, a re-entered read lock blocks forever
// (a pending Lock blocks new RLocks) and every later call queues behind it.
func TestFullTextSearchDoesNotDeadlockWithWriters(t *testing.T) {
	svc, ctx := fulltextFixture(t)
	done := make(chan struct{})
	var stop = make(chan struct{})
	go func() {
		defer close(done)
		errs := make(chan error, 8)
		var running int
		for g := 0; g < 4; g++ {
			running++
			go func() {
				for i := 0; i < 150; i++ {
					if _, err := svc.ObjectSearch(ctx, api.ObjectSearchInput{Query: "cache", Mode: api.SearchFullText}); err != nil {
						errs <- err
						return
					}
				}
				errs <- nil
			}()
		}
		for g := 0; g < 2; g++ {
			running++
			go func(g int) {
				for i := 0; i < 150; i++ {
					o, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.other"})
					if err != nil {
						errs <- err
						return
					}
					obj := o.Object
					obj.Body = strings.Repeat("x", i%7) + " cache"
					if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: obj}); err != nil {
						errs <- err
						return
					}
					// The read path the planner's cycle takes: problems, which
					// takes the index's write lock to drain what changed.
					if _, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "m"}); err != nil {
						errs <- err
						return
					}
				}
				errs <- nil
			}(g)
		}
		for ; running > 0; running-- {
			select {
			case err := <-errs:
				if err != nil {
					t.Error(err)
				}
			case <-stop:
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		close(stop)
		t.Fatal("full-text searches and writers did not finish in 30s: the index deadlocked")
	}
}
