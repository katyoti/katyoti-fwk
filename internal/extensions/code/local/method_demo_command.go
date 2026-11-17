// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	"github.com/spf13/cobra"
)

// NewMethodDemoCommand is the method-core "demo" step: verify an object and
// everything under it (the verification service — every `verify` hook in force; each
// acceptance criterion records its verdict) and report what passed. After it
// runs, `method-overview` reflects the new state — the bottom of the operating
// loop (produce a deliverable → its criteria go green → "What's Next" shrinks).
func NewMethodDemoCommand() *cobra.Command {
	var objectID string
	cmd := &cobra.Command{
		Use:   "method-demo",
		Short: "Demo step — verify an object and what is under it; verdicts are recorded by the verify hooks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if objectID == "" {
				return fmt.Errorf("method-demo: --node <object-id> is required (the module or object to verify)")
			}
			ctx := cmd.Context()
			ver, err := application.UseService[verificationapi.VerificationService](ctx)
			if err != nil {
				return fmt.Errorf("method-demo: resolve VerificationService: %w", err)
			}
			rep, err := ver.Verify(ctx, verificationapi.VerifyInput{ID: objectID})
			if err != nil {
				return fmt.Errorf("method-demo: %w", err)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "DEMO — %s\n", objectID)
			fmt.Fprintf(w, "  passed: %d / %d  (objects verified: %d)\n", rep.Passed, rep.Runs, rep.Objects)
			for _, f := range rep.Failed {
				fmt.Fprintf(w, "  fail      %s  %s\n", f.ObjectID, f.Evidence)
			}
			for _, u := range rep.Unproven {
				fmt.Fprintf(w, "  %-9s %s  %s\n", u.Status, u.ObjectID, u.Evidence)
			}
			fmt.Fprintf(w, "\nVerdicts recorded by the verify hooks. Re-run `method-overview` to see What's Next update.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&objectID, "node", "", "Object id (a module, a use-case, …) to verify with everything under it")
	return cmd
}
