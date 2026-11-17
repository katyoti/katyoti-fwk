// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/variables"
)

func TestEnvResolverRegistered(t *testing.T) {
	if !variables.HasResolver("env") {
		t.Fatal("env resolver should be registered via init()")
	}
}

func TestExpandVarsWithEnv(t *testing.T) {
	os.Setenv("CENV_TEST_VAR", "hello")
	defer os.Unsetenv("CENV_TEST_VAR")

	result := variables.ExpandVars("val=${{env:CENV_TEST_VAR}}")
	if result != "val=hello" {
		t.Fatalf("got %q, want %q", result, "val=hello")
	}
}

func TestGetString_Set(t *testing.T) {
	os.Setenv("CENV_STR", "abc")
	defer os.Unsetenv("CENV_STR")

	v, err := GetString("CENV_STR")
	if err != nil || v != "abc" {
		t.Fatalf("got %q, err=%v", v, err)
	}
}

func TestGetString_Missing_WithDefault(t *testing.T) {
	v, err := GetString("CENV_MISSING", "fallback")
	if err != nil || v != "fallback" {
		t.Fatalf("got %q, err=%v", v, err)
	}
}

func TestGetString_Missing_Required(t *testing.T) {
	_, err := GetString("CENV_MISSING_REQ")
	if err == nil {
		t.Fatal("expected error for missing required var")
	}
}

func TestGetBool(t *testing.T) {
	os.Setenv("CENV_BOOL", "true")
	defer os.Unsetenv("CENV_BOOL")

	v, err := GetBool("CENV_BOOL")
	if err != nil || v != true {
		t.Fatalf("got %v, err=%v", v, err)
	}
}

func TestGetBool_Default(t *testing.T) {
	v, err := GetBool("CENV_BOOL_MISS", false)
	if err != nil || v != false {
		t.Fatalf("got %v, err=%v", v, err)
	}
}

func TestGetInt(t *testing.T) {
	os.Setenv("CENV_INT", "42")
	defer os.Unsetenv("CENV_INT")

	v, err := GetInt("CENV_INT")
	if err != nil || v != 42 {
		t.Fatalf("got %d, err=%v", v, err)
	}
}

func TestGetFloat(t *testing.T) {
	os.Setenv("CENV_FLOAT", "3.14")
	defer os.Unsetenv("CENV_FLOAT")

	v, err := GetFloat("CENV_FLOAT")
	if err != nil || v != 3.14 {
		t.Fatalf("got %f, err=%v", v, err)
	}
}

func TestGetList(t *testing.T) {
	os.Setenv("CENV_LIST", "a, b, c")
	defer os.Unsetenv("CENV_LIST")

	v, err := GetList("CENV_LIST")
	if err != nil || len(v) != 3 || v[0] != "a" || v[1] != "b" || v[2] != "c" {
		t.Fatalf("got %v, err=%v", v, err)
	}
}

func TestGetPath_RelativeToCWD(t *testing.T) {
	os.Setenv("CENV_PATH", "relative/path")
	defer os.Unsetenv("CENV_PATH")

	v, err := GetPath("CENV_PATH", "")
	if err != nil || v == "" || v == "relative/path" {
		t.Fatalf("got %q, err=%v (expected absolute path)", v, err)
	}
}

func TestGetPath_RelativeToBaseDir(t *testing.T) {
	os.Setenv("CENV_PATH_BASE", "sub/dir")
	defer os.Unsetenv("CENV_PATH_BASE")

	v, err := GetPath("CENV_PATH_BASE", "/my/base")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/my/base", "sub/dir")
	if v != want {
		t.Fatalf("got %q, want %q", v, want)
	}
}

func TestGetPath_Absolute(t *testing.T) {
	abs := t.TempDir()
	os.Setenv("CENV_PATH_ABS", abs)
	defer os.Unsetenv("CENV_PATH_ABS")

	v, err := GetPath("CENV_PATH_ABS", "/ignored/base")
	if err != nil || v != filepath.Clean(abs) {
		t.Fatalf("got %q, err=%v (expected %q)", v, err, abs)
	}
}

func TestMustGetString_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustGetString("CENV_MUST_PANIC")
}

func TestResolveDir_Empty(t *testing.T) {
	got := ResolveDir("/base", "", "/fallback")
	if got != "/fallback" {
		t.Fatalf("got %q, want /fallback", got)
	}
}

func TestResolveDir_Absolute(t *testing.T) {
	// Use a real absolute path from the temp dir (works on all OSes).
	abs := t.TempDir()
	got := ResolveDir("/base", abs, "/fallback")
	if got != filepath.Clean(abs) {
		t.Fatalf("got %q, want %q", got, filepath.Clean(abs))
	}
}

func TestResolveDir_Relative(t *testing.T) {
	got := ResolveDir("/base", "sub/dir", "/fallback")
	want := filepath.Join("/base", "sub/dir")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveWorkspaceDir_Default(t *testing.T) {
	os.Unsetenv("KTW_WORKSPACE")
	got := ResolveWorkspaceDir("testapp")
	// Default: <XDG_CONFIG_HOME>/testapp (fallback root).
	if got == "" {
		t.Fatal("got empty WorkspaceDir")
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("got %q, want absolute path", got)
	}
	if filepath.Base(got) != "testapp" {
		t.Fatalf("got %q, want base dir %q", got, "testapp")
	}
}

func TestResolveWorkspaceDir_EnvOverride(t *testing.T) {
	os.Setenv("KTW_WORKSPACE", "/custom/workspace")
	defer os.Unsetenv("KTW_WORKSPACE")

	got := ResolveWorkspaceDir("testapp")
	want := filepath.Clean("/custom/workspace")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("CENV_LOAD_TEST=from_file\n"), 0644)
	os.Unsetenv("CENV_LOAD_TEST")
	defer os.Unsetenv("CENV_LOAD_TEST")

	err := loadEnvFile(envFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v := os.Getenv("CENV_LOAD_TEST"); v != "from_file" {
		t.Fatalf("got %q, want from_file", v)
	}
}

func TestLoadEnvFile_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("CENV_NOOVERWRITE=file_val\n"), 0644)

	os.Setenv("CENV_NOOVERWRITE", "already_set")
	defer os.Unsetenv("CENV_NOOVERWRITE")

	_ = loadEnvFile(envFile)
	if v := os.Getenv("CENV_NOOVERWRITE"); v != "already_set" {
		t.Fatalf("got %q, want already_set (should not overwrite)", v)
	}
}

func TestLoadEnvFile_MissingFile(t *testing.T) {
	err := loadEnvFile("/nonexistent/.env")
	if err != nil {
		t.Fatalf("missing file should return nil, got %v", err)
	}
}

func TestStripQuotes(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"hello"`, "hello"},
		{`'hello'`, "hello"},
		{`hello`, "hello"},
		{`""`, ""},
		{`"`, `"`},
	}
	for _, c := range cases {
		if got := stripQuotes(c.in); got != c.want {
			t.Errorf("stripQuotes(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
