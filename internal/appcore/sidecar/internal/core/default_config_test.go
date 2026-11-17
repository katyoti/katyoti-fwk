// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
)

// L1 — Get honours in.Default by reading the embedded FS instead of disk.
// Regression guard for the SN-07 GetDefault-fold-into-Get migration.
// Reading the embedded path does NOT touch application.Env() so this
// test is independent of the singleton workspace.
func TestDefaultConfigService_Get_DefaultFlagReadsEmbed(t *testing.T) {
	efs := fstest.MapFS{
		"embed/config/SN07Hello.yaml": &fstest.MapFile{Data: []byte("greeting: hi\n")},
	}
	svc := NewDefaultConfigService(ConfigDeps{
		AppName:    "ktw-test",
		EmbeddedFS: efs,
	})

	got, err := svc.Get(context.Background(), api.ConfigGetInput{
		Name:    "SN07Hello.yaml",
		Default: true,
	})
	if err != nil {
		t.Fatalf("Get(default=true): %v", err)
	}
	if got.Name != "SN07Hello.yaml" {
		t.Errorf("Name=%q want SN07Hello.yaml", got.Name)
	}
	if got.Path != "" {
		t.Errorf("Path=%q expected empty when reading default", got.Path)
	}
	if !strings.Contains(got.Content, "greeting: hi") {
		t.Errorf("Content=%q does not contain the default body", got.Content)
	}
}

// L1 — Get with Default=true on a name that's NOT in the embed FS
// returns an error. Validates the "no fallback to disk" rule.
func TestDefaultConfigService_Get_DefaultMissingInEmbedReturnsError(t *testing.T) {
	efs := fstest.MapFS{} // empty
	svc := NewDefaultConfigService(ConfigDeps{EmbeddedFS: efs})

	_, err := svc.Get(context.Background(), api.ConfigGetInput{
		Name:    "SN07Missing.yaml",
		Default: true,
	})
	if err == nil {
		t.Error("expected error for missing embedded default, got nil")
	}
}

// L1 — Set with Key set returns the "not yet implemented" error. Pins
// the RFU contract so we don't accidentally ship a half-working
// per-key writer.
func TestDefaultConfigService_Set_PerKeyNotImplemented(t *testing.T) {
	svc := NewDefaultConfigService(ConfigDeps{})

	_, err := svc.Set(context.Background(), api.ConfigSetInput{
		Key:   "auth.client_id",
		Value: "abc",
	})
	if err == nil {
		t.Fatal("expected error for per-key Set, got nil")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error=%v want 'not yet implemented'", err)
	}
}

// Validate rejects what the raw-write path must never persist: empty
// content, YAML that does not parse, and a top level that is not a mapping.
func TestDefaultConfigService_Validate_YAMLSyntax(t *testing.T) {
	svc := NewDefaultConfigService(ConfigDeps{})
	cases := map[string]struct {
		content string
		valid   bool
	}{
		"mapping":      {"a: 1\nb:\n  c: x\n", true},
		"empty":        {"   \n", false},
		"tab-indented": {"a:\n\t- x\n", false},
		"unclosed":     {"a: [1, 2\n", false},
		"scalar-top":   {"just text\n", false},
	}
	for name, c := range cases {
		out, err := svc.Validate(context.Background(), api.ConfigValidateInput{Name: "X.yaml", Content: c.content})
		if err != nil {
			t.Fatalf("%s: Validate: %v", name, err)
		}
		if out.Valid != c.valid {
			t.Errorf("%s: valid=%v want %v (errors=%v)", name, out.Valid, c.valid, out.Errors)
		}
	}
}

// setRaw writes the whole file under the config dir (creating parents) and
// reports the resolved path; a re-write replaces the content.
func TestDefaultConfigService_SetRaw_WritesWholeFile(t *testing.T) {
	svc := NewDefaultConfigService(ConfigDeps{})
	configDir := filepath.Join(t.TempDir(), "config")

	out, err := svc.setRaw(context.Background(), api.ConfigSetOutput{}, configDir,
		api.ConfigSetInput{Name: "Notify.yaml", Content: "voice: alloy\n"})
	if err != nil {
		t.Fatalf("setRaw: %v", err)
	}
	want := filepath.Join(configDir, "Notify.yaml")
	if len(out.Files) != 1 || out.Files[0].Status != "written" || out.Files[0].Path != want {
		t.Fatalf("files=%+v want one 'written' entry at %s", out.Files, want)
	}
	if got, _ := os.ReadFile(want); string(got) != "voice: alloy\n" {
		t.Errorf("content=%q", got)
	}

	if _, err := svc.setRaw(context.Background(), api.ConfigSetOutput{}, configDir,
		api.ConfigSetInput{Name: "Notify.yaml", Content: "voice: nova\n"}); err != nil {
		t.Fatalf("re-write: %v", err)
	}
	if got, _ := os.ReadFile(want); string(got) != "voice: nova\n" {
		t.Errorf("after re-write content=%q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(configDir, ".*.tmp")); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}

// Invalid content is refused BEFORE anything touches the disk.
func TestDefaultConfigService_SetRaw_RejectsInvalidWithoutWriting(t *testing.T) {
	svc := NewDefaultConfigService(ConfigDeps{})
	configDir := filepath.Join(t.TempDir(), "config")
	_ = os.MkdirAll(configDir, 0o755)
	path := filepath.Join(configDir, "X.yaml")
	_ = os.WriteFile(path, []byte("keep: me\n"), 0o644)

	_, err := svc.setRaw(context.Background(), api.ConfigSetOutput{}, configDir,
		api.ConfigSetInput{Name: "X.yaml", Content: "a: [1,\n"})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("err=%v want a 'rejected' error", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "keep: me\n" {
		t.Errorf("file was modified: %q", got)
	}
}

// The name is confined to the config dir.
func TestDefaultConfigService_SetRaw_RefusesEscapingNames(t *testing.T) {
	svc := NewDefaultConfigService(ConfigDeps{})
	configDir := filepath.Join(t.TempDir(), "config")
	for _, name := range []string{"../X.yaml", "..", filepath.Join(t.TempDir(), "abs.yaml")} {
		_, err := svc.setRaw(context.Background(), api.ConfigSetOutput{}, configDir,
			api.ConfigSetInput{Name: name, Content: "a: 1\n"})
		if err == nil {
			t.Errorf("%q: expected refusal", name)
		}
	}
}
