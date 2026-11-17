// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"errors"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// pieceCWM is a CWM over the bus: a write larger than a message is refused.
type pieceCWM struct {
	cwmapi.CWMService
	files map[string][]byte
}

func (c *pieceCWM) ObjectFileWrite(_ context.Context, in cwmapi.ObjectFileWriteInput) (cwmapi.ObjectFileWriteOutput, error) {
	if len(in.Content) > writePiece {
		return cwmapi.ObjectFileWriteOutput{}, errors.New("nats: maximum payload exceeded")
	}
	if in.Append {
		c.files[in.Path] = append(c.files[in.Path], in.Content...)
	} else {
		c.files[in.Path] = append([]byte(nil), in.Content...)
	}
	return cwmapi.ObjectFileWriteOutput{Path: in.Path, ProjectPath: "modules/m/" + in.Path}, nil
}

// A file larger than one message (a template of a few megabytes) is written
// whole, piece by piece; an empty file is still written.
func TestWriteInPieces(t *testing.T) {
	cwm := &pieceCWM{files: map[string][]byte{}}
	big := bytes.Repeat([]byte("0123456789"), 130_000) // 1.3 MB
	if _, err := writeInPieces(context.Background(), cwm, "m", "themes/acme/reference.pptx", big); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cwm.files["themes/acme/reference.pptx"], big) {
		t.Fatalf("written %d bytes, want %d", len(cwm.files["themes/acme/reference.pptx"]), len(big))
	}
	if _, err := writeInPieces(context.Background(), cwm, "m", "themes/acme/empty.txt", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := cwm.files["themes/acme/empty.txt"]; !ok {
		t.Error("an empty file was not written")
	}
}
