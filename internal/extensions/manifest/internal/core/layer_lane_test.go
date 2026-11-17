// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	core "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// A consumer view holds the index only; a skill's instructions are a
// non-resident layer. Until 2026-09-22 (e9) OpenLayer resolved such a layer
// through the BODY lane by hash, which the aggregator answers with the data
// layer only — so the view would have handed `{}` back under the name
// `instructions`, or failed the hash check. The layer lane opens the layer
// BY NAME through the aggregator, checks the address and caches by hash.
func TestMemoryCatalog_OpenLayer_UsesTheLayerLaneAndChecksTheAddress(t *testing.T) {
	body := []byte("# leanix-tools\n\nHow to use them.\n")
	skill := contribapi.Contribution{Provider: "ktw-example", Type: "skill", ID: "leanix-tools"}
	skill.SetData([]byte(`{"name":"leanix-tools"}`))
	skill.Layers[mbapi.LayerInstructions] = contribapi.LayerRef{Hash: contribapi.HashContent(body), Size: int64(len(body)), MediaType: "text/markdown"}
	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-example", Contributions: []contribapi.Contribution{skill}}}, nil)
	uri := "ktw-example:/skill/leanix-tools"

	// No lane wired: an honest refusal, never the data layer under another name.
	if _, _, err := cat.OpenLayer(context.Background(), uri, mbapi.LayerInstructions); !errors.As(err, new(contribapi.ErrLayerNotCarried)) {
		t.Fatalf("without a layer lane: err = %v, want contribapi.ErrLayerNotCarried", err)
	}

	calls := 0
	cat.SetContentLane(core.NewContentStore(t.TempDir()), nil)
	cat.SetLayerLane(func(_ context.Context, gotURI, gotLayer string) ([]byte, contribapi.LayerRef, error) {
		calls++
		if gotLayer != mbapi.LayerInstructions {
			t.Errorf("layer lane asked for layer %q", gotLayer)
		}
		if gotURI != uri {
			return []byte("something else\n"), contribapi.LayerRef{}, nil // the stale entry below
		}
		return body, contribapi.LayerRef{MediaType: "text/markdown"}, nil
	})
	rc, ref, err := cat.OpenLayer(context.Background(), uri, mbapi.LayerInstructions)
	if err != nil {
		t.Fatalf("open through the layer lane: %v", err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != string(body) || ref.Hash != contribapi.HashContent(body) {
		t.Fatalf("got %q ref %+v", got, ref)
	}
	// Cached by hash: the second open costs no call.
	if _, _, err := cat.OpenLayer(context.Background(), uri, mbapi.LayerInstructions); err != nil || calls != 1 {
		t.Fatalf("second open: err=%v calls=%d, want a content-store hit", err, calls)
	}

	// Bytes that do not match the index's address are refused.
	other := contribapi.Contribution{Provider: "ktw-example", Type: "skill", ID: "edited"}
	other.SetData([]byte(`{}`))
	other.Layers[mbapi.LayerInstructions] = contribapi.LayerRef{Hash: "sha256:stale", Size: 5}
	cat.Put("ktw-example", &mbapi.Manifest{Provider: "ktw-example", Contributions: []contribapi.Contribution{skill, other}})
	if _, _, err := cat.OpenLayer(context.Background(), "ktw-example:/skill/edited", mbapi.LayerInstructions); err == nil || !strings.Contains(err.Error(), "does not match its address") {
		t.Fatalf("a mismatching fetch must be refused, got %v", err)
	}

	// The data layer never takes the layer lane: it is inline here.
	rc, _, err = cat.OpenLayer(context.Background(), uri, contribapi.LayerData)
	if err != nil {
		t.Fatalf("data: %v", err)
	}
	got, _ = io.ReadAll(rc)
	if string(got) != `{"name":"leanix-tools"}` || calls != 2 {
		t.Fatalf("data layer: %q, calls=%d (one per uncached instructions open, none for data)", got, calls)
	}
}
