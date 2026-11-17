// SPDX-License-Identifier: Apache-2.0

// Command go-recipe-producer is a Go sidecar that publishes entries of a kind
// it does NOT declare, whose declaration and schema another member of the
// universe ships (files-only-pack, beside this fixture). It is the Go
// counterpart of the polyglot producer (blind audit H3): a kind is declared
// once in a universe, and a Go provider — like a files-only pack, like a
// producer in any language — may publish entries of a kind another member
// declares. Its manifest builds with the kind recorded under ForeignKinds;
// the hub validates the entries against the declaring member's schema.
//
// What it publishes is DATA (producer.yaml: the kind and the folder), never a
// Go literal, for the same reason the polyglot producer reads producer.json:
// no Go file in this repository names the pack's kind
// (internal/core/contribution TestPackKindNeedsNoFrameworkCode).
//
// Built by the tests that need it (`go build` into a temp dir, like the
// polyglot producer); never part of bin/.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
)

// Provider is the sidecar's name — the `<provider>` of every entry.
const Provider = "go-recipe-producer"

// producerSpec is producer.yaml: the kind published and the folder holding
// one file per entry.
type producerSpec struct {
	Kind string `yaml:"kind"`
	Dir  string `yaml:"dir"`
}

//go:embed producer.yaml recipes
var content embed.FS

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	raw, err := content.ReadFile("producer.yaml")
	if err != nil {
		return fmt.Errorf("go-recipe-producer: %w", err)
	}
	var spec producerSpec
	if err := config.DecodeBytes(raw, marshal.YAML, &spec); err != nil {
		return fmt.Errorf("go-recipe-producer: producer.yaml: %w", err)
	}
	return appcore.BootstrapCLI("sidecar", sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: Provider,
		Version: "1.0.0",
		Short:   "go-recipe-producer — a Go sidecar publishing entries of a kind another member declares (fixture)",
	},
		sidecarkit.WithoutPack(), // no embedded pack: the entries below are its content
		sidecarkit.WithoutHTTPServer(),
		// The generic producer: every file under the folder is one entry of
		// the kind. No declaration of the kind here, on purpose.
		sidecarkit.WithExtraExtensions(manifestcontrib.ContributeKindResources(Provider, spec.Kind, loader.NewEmbedFSLoader(content, "."), spec.Dir)),
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)...)
}
