// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDef(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, DefinitionFile)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Paths are relative to the file, ${{env:VAR}} reads a variable, the defaults
// fill in, and the folder stands for its marketplace.yaml.
func TestADefinitionResolvesAgainstItsFile(t *testing.T) {
	t.Setenv("KTW_TEST_SHARE", filepath.Join(t.TempDir(), "share"))
	dir := writeDef(t, `# a comment may show ${{env:NOT_SET_ANYWHERE}}
name: team
output: ${{env:KTW_TEST_SHARE}}/market
key: {folder: keys, prefix: team}
platforms: [windows-amd64]
config_sets: [{path: config-sets/base}]
vaults: [{name: keys, env_file: .env, keys: config-sets/base/keys.yaml, password_env: TEAM_PW}]
solutions: [{case: scenarios/demo, config_sets: [base], vaults: [keys], other_universes: [{name: review, config_sets: [base]}]}]
demo: demo
`)
	d, err := LoadDefinition(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d.Output != filepath.Join(os.Getenv("KTW_TEST_SHARE"), "market") {
		t.Errorf("output %s", d.Output)
	}
	if d.SecretKey() != filepath.Join(dir, "keys", "team.key") || d.Keep != 3 {
		t.Errorf("key %s keep %d", d.SecretKey(), d.Keep)
	}
	if d.ConfigSets[0].ItemName() != "base" || d.Solutions[0].ItemName() != "demo" || d.Solutions[0].Case != filepath.Join(dir, "scenarios", "demo") {
		t.Errorf("items %+v %+v", d.ConfigSets, d.Solutions)
	}
	if o := d.Solutions[0].OtherUniverses; len(o) != 1 || o[0].Name != "review" || len(o[0].ConfigSets) != 1 || o[0].ConfigSets[0] != "base" {
		t.Errorf("other universes %+v, want review with base", o)
	}
	if d.Vaults[0].EnvFile != filepath.Join(dir, ".env") {
		t.Errorf("vault env %s", d.Vaults[0].EnvFile)
	}
}

// What a definition cannot mean is refused, by name.
func TestADefinitionRefusesWhatItCannotMean(t *testing.T) {
	base := "name: team\noutput: out\nkey: {folder: keys, prefix: team}\nplatforms: [windows-amd64]\n"
	for want, body := range map[string]string{
		"KTW_SURELY_UNSET":                           "name: team\noutput: ${{env:KTW_SURELY_UNSET}}/x\nkey: {folder: k, prefix: p}\nplatforms: [a-b]\n",
		"no platforms":                               "name: team\noutput: out\nkey: {folder: keys, prefix: team}\n",
		`config-set "missing"`:                       base + "solutions: [{case: c, config_sets: [missing]}]\n",
		`vault "nope"`:                               base + "solutions: [{case: c, vaults: [nope]}]\n",
		`the demo "other"`:                           base + "solutions: [{case: c}]\ndemo: other\n",
		"password_env are all needed":                base + "vaults: [{name: v, env_file: .env, keys: k.yaml}]\n",
		"two config-sets":                            base + "config_sets: [{path: a/base}, {path: b/base}]\n",
		`universe review pins the config-set "gone"`: base + "solutions: [{case: c, other_universes: [{name: review, config_sets: [gone]}]}]\n",
		"other_universes is named, once":             base + "solutions: [{case: c, other_universes: [{name: r}, {name: r}]}]\n",
	} {
		_, err := LoadDefinition(writeDef(t, body))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want an error naming %q, got %v", want, err)
		}
	}
}
