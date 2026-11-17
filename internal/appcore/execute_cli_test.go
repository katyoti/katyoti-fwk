// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

type closeRecorder struct{ closed bool }

func (c *closeRecorder) Close() error { c.closed = true; return nil }

// A one-shot command that FAILS closes the application's services as one that
// succeeds does — the CWM service among them, releasing the workspace lock, so
// the next command is not refused on it.
func TestAFailingCommandClosesTheApplication(t *testing.T) {
	for _, fails := range []bool{true, false} {
		app := application.New(application.Config(),
			application.DeclareSimpleService[*closeRecorder](func(context.Context) (*closeRecorder, error) { return &closeRecorder{}, nil }))
		if err := app.Run(); err != nil {
			t.Fatal(err)
		}
		ctx := application.WithApplication(context.Background(), app)
		rec, err := application.UseService[*closeRecorder](ctx)
		if err != nil {
			t.Fatal(err)
		}
		root := &cobra.Command{Use: "x", SilenceUsage: true, SilenceErrors: true, RunE: func(*cobra.Command, []string) error {
			if fails {
				return errors.New("refused")
			}
			return nil
		}}
		root.SetArgs(nil)
		if err := ExecuteCLI(ctx, root); (err != nil) != fails {
			t.Fatalf("fails=%v: err = %v", fails, err)
		}
		if !rec.closed {
			t.Errorf("fails=%v: the application's services were left open", fails)
		}
	}
}
