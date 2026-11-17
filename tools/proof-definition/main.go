// SPDX-License-Identifier: Apache-2.0

// proof-definition writes the marketplace definition a clean-machine proof
// builds: the published one (a marketplace.yaml) narrowed to
// the solution a fresh install opens on, with its config-sets and vaults as
// published, and only what a proof must change - where it goes, its key, its
// platforms, the values its vaults are sealed from, their password, where its
// cases run (authoring.Definition.Narrow). scripts/proof-channel.sh runs it, so
// a proof and a release cannot differ on what is installed.
//
//	go run ./tools/proof-definition --from <marketplace.yaml> --out <file> \
//	    --output <dir> --key-folder <dir> --key-prefix proof --platforms windows-amd64 \
//	    --env-file <proof.env> --password-env KTW_PROOF_VAULT_PASSWORD --scenario-root <dir>
//
// The published definition's variables (${{env:VAR}}) must be set, as where
// it is published from.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

func main() {
	from := flag.String("from", "", "the published definition: a marketplace.yaml, or its folder")
	out := flag.String("out", "", "the proof's definition file to write")
	var n authoring.Narrowing
	flag.StringVar(&n.Name, "name", "", "the proof marketplace's name (default: <published>-proof)")
	flag.StringVar(&n.Output, "output", "", "where the proof's channel is built")
	flag.StringVar(&n.KeyFolder, "key-folder", "", "the throwaway signing key's folder")
	flag.StringVar(&n.KeyPrefix, "key-prefix", "proof", "the throwaway signing key's prefix")
	platforms := flag.String("platforms", "windows-amd64", "the platforms built, comma-separated")
	flag.StringVar(&n.EnvFile, "env-file", "", "the values the vaults are sealed from")
	flag.StringVar(&n.PasswordEnv, "password-env", "KTW_PROOF_VAULT_PASSWORD", "the variable the vaults' password is read from")
	flag.StringVar(&n.ScenarioRoot, "scenario-root", "", "where the solution's case runs")
	flag.Parse()
	n.Platforms = strings.Split(*platforms, ",")
	if *from == "" || *out == "" || n.Output == "" || n.KeyFolder == "" || n.EnvFile == "" || n.ScenarioRoot == "" {
		fmt.Fprintln(os.Stderr, "usage: proof-definition --from <def> --out <file> --output <dir> --key-folder <dir> --env-file <file> --scenario-root <dir>")
		os.Exit(2)
	}
	if err := run(*from, *out, n); err != nil {
		fmt.Fprintln(os.Stderr, "proof-definition:", err)
		os.Exit(1)
	}
}

func run(from, out string, n authoring.Narrowing) error {
	d, err := authoring.LoadDefinition(from)
	if err != nil {
		return err
	}
	if n.Name == "" {
		n.Name = d.Name + "-proof"
	}
	p, err := d.Narrow(n)
	if err != nil {
		return err
	}
	data, err := p.Marshal()
	if err != nil {
		return err
	}
	header := "# The clean-machine proof's marketplace, never published: " + d.Path() + " narrowed\n# to the solution a fresh install opens on (" + p.Demo + ").\n"
	if err := os.WriteFile(out, append([]byte(header), data...), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %s narrowed to %s\n", out, d.Path(), p.Demo)
	return nil
}
