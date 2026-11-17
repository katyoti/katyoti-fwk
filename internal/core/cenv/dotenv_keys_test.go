// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"testing"
)

// The keys a process took from a universe's .env and the global .env are
// listed, and a child that reads them itself is started without the copies:
// a key saved after the parent started reaches the child.
func TestScopedDotenvKeysAreListedAndLeftOutOfAChild(t *testing.T) {
	home, global, cwd := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".env"), []byte("CENV_F025_UNI=u\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(global, ".env"), []byte(GlobalKeysVar+"=CENV_F025_GLOBAL\nCENV_F025_GLOBAL=g\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_KATYOTI_ROOT", global)
	t.Setenv(DotenvKeysVar, "CENV_F025_INHERITED")
	t.Setenv("CENV_F025_INHERITED", "from-the-parent")
	t.Setenv("CENV_F025_REAL", "set-by-the-person")
	for _, k := range []string{"CENV_F025_UNI", "CENV_F025_GLOBAL", GlobalKeysVar} {
		t.Setenv(k, "") // registered for cleanup, then unset
		os.Unsetenv(k)
	}
	saved := envFileStatuses
	envFileStatuses = nil
	t.Cleanup(func() { envFileStatuses = saved })
	wd, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	LoadDotenv("katyoti-f025-test", home)

	if os.Getenv("CENV_F025_UNI") != "u" || os.Getenv("CENV_F025_GLOBAL") != "g" {
		t.Fatalf("the scoped .env files were not loaded")
	}
	child := map[string]bool{}
	for _, kv := range ChildEnviron(os.Environ()) {
		name, _, _ := cutName(kv)
		child[name] = true
	}
	for _, gone := range []string{"CENV_F025_UNI", "CENV_F025_GLOBAL", "CENV_F025_INHERITED", DotenvKeysVar} {
		if child[gone] {
			t.Errorf("%s reaches the child: it must read the file itself", gone)
		}
	}
	if !child["CENV_F025_REAL"] {
		t.Errorf("a variable the person set is kept for the child")
	}
}

func cutName(kv string) (string, string, bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' && i > 0 {
			return kv[:i], kv[i+1:], true
		}
	}
	return kv, "", false
}
