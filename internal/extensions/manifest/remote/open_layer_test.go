// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
)

// The remote catalog opens a NAMED layer over the wire — the content lane a
// GUI or another sidecar uses — and a refusal from the aggregator is an error
// that names the layer, never a substitute body.
func TestRemoteOpenLayer_OverTheWire(t *testing.T) {
	md := []byte("# skill\n")
	sender := &fakeSender{opens: map[string]mbapi.OpenLayerOutput{
		"p:/skill/s#source": {Ref: contribapi.LayerRef{Hash: contribapi.HashContent(md), Size: int64(len(md)), MediaType: "text/markdown"}, Content: md},
		"p:/reference/big#source": {Ref: contribapi.LayerRef{Hash: "sha256:big", Size: contribapi.BandControl + 1},
			Refused: "layer \"source\" is in the bulk band (262145 bytes, sha256:big): stream it out of band by hash"},
	}}
	cat := remote.New(sender)
	var _ mbapi.LayerOpener = cat

	rc, ref, err := cat.OpenLayer(context.Background(), "p:/skill/s", "source")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if b, _ := io.ReadAll(rc); string(b) != string(md) || ref.MediaType != "text/markdown" {
		t.Errorf("got %q / %+v", b, ref)
	}
	if !strings.HasSuffix(sender.paths[len(sender.paths)-1], "OpenLayer") {
		t.Errorf("the content lane must call OpenLayer, called %v", sender.paths)
	}

	_, ref, err = cat.OpenLayer(context.Background(), "p:/reference/big", "source")
	var bulk contribapi.ErrBulkLayer
	if !errors.As(err, &bulk) || !strings.Contains(err.Error(), "bulk band") {
		t.Fatalf("a refusal must surface typed (ErrBulkLayer), naming the reason, got %v", err)
	}
	if ref.Size != contribapi.BandControl+1 {
		t.Errorf("the descriptor travels with the refusal so the caller can stream by hash, got %+v", ref)
	}
}
