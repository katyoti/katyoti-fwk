// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

// The key is assembled at run time, so this file holds no secret shape.
func scanFakeKey() string { return "AK" + "IA" + strings.Repeat("WXYZ", 4) }

func scanWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Every artefact a release publishes is scanned before anything is written
// : a binary holding a credential shape and a config-set
// holding a secret file refuse the build naming each file, and the output is
// never created; a deny-list term refuses it only where the definition names a
// deny-list; KaTyoti's own library samples pass.
func TestABuildRefusesAnArtefactHoldingASecretNamingTheFile(t *testing.T) {
	dir := t.TempDir()
	host := runtime.GOOS + "-" + runtime.GOARCH
	bin := filepath.Join(dir, "dist", host)
	scanWrite(t, filepath.Join(bin, "ktw-leaky"), "\x7fELF\x00\x01"+scanFakeKey()+"\x00")
	scanWrite(t, filepath.Join(bin, "ktw-sync"), "\x00see http://"+"user:pass@host.test:8080\x00")
	scanWrite(t, filepath.Join(dir, "base", "Settings.yaml"), "team: Zorblax Corp\n")
	scanWrite(t, filepath.Join(dir, "base", ".env"), "X=1\n")
	scanWrite(t, filepath.Join(dir, "deny.txt"), "# private\n"+`\bzorblax\b`+"\n")
	write := func(deny string) string {
		def := "name: team\noutput: out\nkey: {folder: keys, prefix: team}\nplatforms: [" + host + "]\nconfig_sets: [{path: base}]\n" + deny
		p := filepath.Join(dir, "marketplace.yaml")
		scanWrite(t, p, def)
		return p
	}
	build := func(def string) error {
		_, err := New(nil).BuildFromDefinition(context.Background(), api.BuildFromDefinitionInput{
			Definition: def, Version: "v1", Dist: filepath.Join(dir, "dist"),
		})
		return err
	}

	err := build(write(""))
	if err == nil {
		t.Fatal("a build holding a secret was published")
	}
	for _, want := range []string{
		filepath.Join(bin, "ktw-leaky") + ":1: aws-key, a secret",
		filepath.Join(dir, "base", ".env") + ": secret path, a secret",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should name %q:\n%v", want, err)
		}
	}
	for _, not := range []string{"deny-list", "url-credential"} {
		if strings.Contains(err.Error(), not) {
			t.Errorf("the refusal names %q, which passes here:\n%v", not, err)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out")); statErr == nil {
		t.Fatal("a refused build wrote its output folder")
	}

	err = build(write("secret_scan: {deny: deny.txt}\n"))
	want := filepath.Join(dir, "base", "Settings.yaml") + `:1: deny-list term \bzorblax\b`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("with a deny-list the refusal should name %q:\n%v", want, err)
	}
}

// A definition whose artefacts are clean has no scan problem.
func TestACleanDefinitionPassesTheArtefactScan(t *testing.T) {
	dir := t.TempDir()
	scanWrite(t, filepath.Join(dir, "dist", "linux-amd64", "ktw-x"), "\x00plain\x00")
	scanWrite(t, filepath.Join(dir, "base", "Settings.yaml"), "team: ok\n")
	scanWrite(t, filepath.Join(dir, "deny.txt"), `\bzorblax\b`+"\n")
	def := &authoring.Definition{
		Platforms:  []string{"linux-amd64"},
		ConfigSets: []authoring.DefinitionConfigSet{{Path: filepath.Join(dir, "base")}},
		SecretScan: authoring.DefinitionSecretScan{Deny: filepath.Join(dir, "deny.txt")},
	}
	if got := scanArtefacts(def, filepath.Join(dir, "dist"), "", false); len(got) != 0 {
		t.Fatalf("a clean definition: %v", got)
	}
}
