// SPDX-License-Identifier: Apache-2.0

package local

import (
	"bytes"
	"encoding/json"
	"fmt"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	composecore "github.com/katyoti/katyoti-fwk/internal/extensions/compose/internal/core"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// InspectUsage is the verb's usage line.
const InspectUsage = "inspect --uri <provider>:/<kind>/<id> (--exe <sidecar> | --universe) [--out yaml|json]"

// newInspectCommand is `ktw-compose inspect`: one entry of any kind, as the
// model holds it. Hand-written only because it renders YAML by default; the
// body is composecore's api.Inspector.
func newInspectCommand() *cobra.Command {
	var in api.InspectInput
	var out string
	cmd := &cobra.Command{
		Use:   InspectUsage,
		Short: "Show one contribution of any kind: kind, layers (hash, size, band), provider identity, provenance, verdict",
		Long: `Inspect one entry as the model holds it — for ANY kind, with no kind-specific code:

  uri, provider (+ its identity rows: digest, mutable, declared),
  kind (schema, declared layers, default layer, budget, severity, execution),
  meta as published, layers (hash, size, band, media type, inline),
  provenance (when signed) and the validation verdict.

--exe reads a compiled sidecar through its own 'sc resources manifest' in a
throwaway workspace (a folder pack served by ktw-host takes its folder from
KTW_CONTENT_ROOT, or freeze it into one exe). A single binary carries no
verdict; --universe reads the aggregate, where the hub recorded one.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := composecore.NewInspector().Inspect(cmd.Context(), in)
			if err != nil {
				return err
			}
			var b []byte
			switch out {
			case "yaml", "":
				var buf bytes.Buffer
				enc := yaml.NewEncoder(&buf)
				enc.SetIndent(2)
				err = enc.Encode(doc)
				_ = enc.Close()
				b = buf.Bytes()
			case "json":
				b, err = json.MarshalIndent(doc, "", "  ")
				b = append(b, '\n')
			default:
				return fmt.Errorf("inspect: --out %q: want yaml or json", out)
			}
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	}
	cmd.Flags().StringVar(&in.URI, "uri", "", "Resource URI <provider>:/<kind>/<id>")
	cmd.Flags().StringVar(&in.Exe, "exe", "", "Compiled sidecar that publishes the URI")
	cmd.Flags().BoolVar(&in.Universe, "universe", false, "Read the universe this binary belongs to (the aggregate)")
	cmd.Flags().StringVar(&out, "out", "yaml", "Output: yaml | json")
	_ = cmd.MarkFlagRequired("uri")
	return cmd
}
