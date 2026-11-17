// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const publishedTwoSolutions = `name: acme
output: out
mirrors: [mirror]
key: {folder: keys, prefix: katyoti}
keep: 3
platforms: [windows-amd64, linux-amd64]
config_sets: [{path: config-sets/acme-base}, {path: config-sets/archifactory-base}]
vaults: [{name: acme-keys, env_file: .env, keys: vaults/acme-keys.yaml, password_env: ACME_VAULT_PASSWORD}]
solutions:
  - {case: scenarios/acme-demo, config_sets: [acme-base], vaults: [acme-keys]}
  - {case: scenarios/archifactory, config_sets: [archifactory-base], vaults: [acme-keys]}
demo: %s
`

// A proof builds what a fresh install from the published definition installs:
// the solution it opens on, its config-sets and vaults as published, changed
// only in where it goes, its key, platforms, vault values and password.
func TestAProofDefinitionIsThePublishedOneNarrowed(t *testing.T) {
	for _, demo := range []string{"archifactory", "acme-demo"} {
		dir := writeDef(t, strings.Replace(publishedTwoSolutions, "%s", demo, 1))
		d, err := LoadDefinition(dir)
		if err != nil {
			t.Fatal(err)
		}
		work := t.TempDir()
		n, err := d.Narrow(Narrowing{Name: "acme-proof", Output: filepath.Join(work, "out"), KeyFolder: filepath.Join(work, "keys"),
			KeyPrefix: "proof", Platforms: []string{"windows-amd64"}, EnvFile: filepath.Join(work, "proof.env"),
			PasswordEnv: "KTW_PROOF_VAULT_PASSWORD", ScenarioRoot: filepath.Join(work, "cases")})
		if err != nil {
			t.Fatal(err)
		}
		if n.Demo != demo || len(n.Solutions) != 1 || n.Solutions[0].ItemName() != demo {
			t.Fatalf("%s: solutions %+v demo %q, want %s alone", demo, n.Solutions, n.Demo, demo)
		}
		wantSet := map[string]string{"archifactory": "archifactory-base", "acme-demo": "acme-base"}[demo]
		if len(n.ConfigSets) != 1 || n.ConfigSets[0].ItemName() != wantSet || n.ConfigSets[0].Path != filepath.Join(dir, "config-sets", wantSet) {
			t.Errorf("%s: config-sets %+v, want %s as published", demo, n.ConfigSets, wantSet)
		}
		if len(n.Vaults) != 1 || n.Vaults[0].Keys != filepath.Join(dir, "vaults", "acme-keys.yaml") ||
			n.Vaults[0].EnvFile != filepath.Join(work, "proof.env") || n.Vaults[0].PasswordEnv != "KTW_PROOF_VAULT_PASSWORD" {
			t.Errorf("%s: vaults %+v, want acme-keys with its keys, the proof's values and password", demo, n.Vaults)
		}
		if len(n.Mirrors) != 0 || n.Keep != 1 || !reflect.DeepEqual(n.Platforms, []string{"windows-amd64"}) || n.Key.Prefix != "proof" {
			t.Errorf("%s: mirrors %v keep %d platforms %v key %+v", demo, n.Mirrors, n.Keep, n.Platforms, n.Key)
		}
		// Written out, it is a definition the build reads as it is.
		data, err := n.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(work, DefinitionFile)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		back, err := LoadDefinition(p)
		if err != nil {
			t.Fatalf("%s: the narrowed definition does not load: %v\n%s", demo, err, data)
		}
		if back.Demo != demo || back.Solutions[0].Case != filepath.Join(dir, "scenarios", demo) {
			t.Errorf("%s: read back %+v", demo, back.Solutions)
		}
	}
}

func TestAProofNeedsTheSolutionAFreshInstallOpensOn(t *testing.T) {
	d, err := LoadDefinition(writeDef(t, "name: m\noutput: out\nkey: {folder: k, prefix: p}\nplatforms: [windows-amd64]\nsolutions: [{case: c}]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Narrow(Narrowing{}); err == nil || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("err = %v, want a refusal naming the demo", err)
	}
}
