// SPDX-License-Identifier: Apache-2.0

package secretscan

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// The secret shapes are assembled at run time, so this file holds none and
// the scanner passes it without the fake marker.
func shapes() map[string]string {
	rep := func(s string, n int) string { return strings.Repeat(s, n) }
	return map[string]string{
		"api-key":        "key := \"" + "sk" + "-" + rep("a1B2", 6) + "\"",
		"aws-key":        "id = " + "AK" + "IA" + rep("ABCD", 4),
		"github-token":   "token: " + "gh" + "p_" + rep("x9Y8", 9),
		"slack-token":    "hook " + "xo" + "xb-" + rep("12ab", 4),
		"bearer":         "Authorization: " + "Bearer " + rep("tok3n", 5),
		"jwt":            "jwt " + "ey" + "J" + rep("a", 10) + "." + rep("b", 10) + "." + rep("c", 10),
		"private-key":    "-----BEGIN RSA " + "PRIVATE KEY-----",
		"url-credential": "url: " + "https://alice:" + "s3cretpass" + "@example.com/x",
	}
}

func TestScanRefusesEverySecretPath(t *testing.T) {
	samples := []string{
		".env", "app/.env.local", ".secret/x", "a/.secrets/y.txt", "data/hub/keys/seed.nk",
		"a.pem", "b.key", "c.p12", "d.pfx", "e.jks", "f.keystore", "api.token",
		"id_rsa", "home/id_dsa", "id_ecdsa", "id_ed25519", ".netrc", ".pgpass", ".git-credentials",
	}
	got := Scan(samples, nil, NewShapes())
	if len(got) != len(samples) {
		t.Fatalf("refused %d of %d secret paths: %v", len(got), len(samples), got)
	}
	for i, f := range got {
		if f.Path != samples[i] || f.Line != 0 || f.Kind != "secret path" {
			t.Errorf("finding %d = %+v, want %s as a secret path", i, f, samples[i])
		}
	}
}

func TestScanPassesAllowedPaths(t *testing.T) {
	ok := []string{".env.example", ".env.build.example", "id_rsa.pub", "main.go", "docs/token.md", "keys/readme.md"}
	if got := Scan(ok, nil, NewShapes()); len(got) != 0 {
		t.Fatalf("clean paths refused: %v", got)
	}
}

func TestScanRefusesEveryCredentialShape(t *testing.T) {
	sh := NewShapes()
	for kind, line := range shapes() {
		got := Scan(nil, []AddedLine{{Path: "x.go", Line: 7, Text: line}}, sh)
		if len(got) != 1 || got[0].Kind != kind || got[0].Line != 7 || got[0].Path != "x.go" {
			t.Errorf("%s: %q gave %v, want one %s finding at x.go:7", kind, line, got, kind)
		}
	}
}

func TestScanPassesAFakeFixtureAndACleanLine(t *testing.T) {
	sh := NewShapes()
	added := []AddedLine{
		{Path: "x_test.go", Line: 1, Text: shapes()["aws-key"] + " // " + FakeMarker},
		{Path: "x.go", Line: 2, Text: "func main() { fmt.Println(\"hello\") }"},
		{Path: "x.go", Line: 3, Text: "already scrubbed: [REDACTED:api-key]"},
	}
	if got := Scan(nil, added, sh); len(got) != 0 {
		t.Fatalf("clean or fake lines refused: %v", got)
	}
}

// The environment's values are not compared: a secret-named variable whose
// value is an ordinary word must not refuse every line holding it.
func TestScanIgnoresTheEnvironmentValues(t *testing.T) {
	t.Setenv("SOME_AUTH_SETTING", "ordinary-words-here")
	got := Scan(nil, []AddedLine{{Path: "a.md", Line: 1, Text: "see ordinary-words-here"}}, NewShapes())
	if len(got) != 0 {
		t.Fatalf("an environment value was compared: %v", got)
	}
	if os.Getenv("SOME_AUTH_SETTING") != "ordinary-words-here" {
		t.Fatal("NewShapes did not restore the environment")
	}
}

func TestParseAddedReadsOnlyAddedLinesWithTheirNumbers(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/a.go b/a.go",
		"index 1..2 100644",
		"--- a/a.go",
		"+++ b/a.go",
		"@@ -3,0 +4,2 @@ func x() {",
		"+one",
		"+two",
		"@@ -10 +12 @@",
		"-gone",
		"+three",
		"diff --git a/old.txt b/old.txt",
		"deleted file mode 100644",
		"--- a/old.txt",
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-removed",
		"diff --git a/dir with space/n.md b/dir with space/n.md",
		"new file mode 100644",
		"--- /dev/null",
		"+++ b/dir with space/n.md",
		"@@ -0,0 +1 @@",
		"+++ plus line",
	}, "\n")
	got := ParseAdded(diff)
	want := []AddedLine{
		{"a.go", 4, "one"}, {"a.go", 5, "two"}, {"a.go", 12, "three"},
		{"dir with space/n.md", 1, "++ plus line"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// stage makes a fresh repository in a temporary folder, writes the files and
// stages them (git add -f, past any .gitignore), and answers its folder. The
// caller's GIT_* variables (a hook's GIT_INDEX_FILE) are kept away from it.
func stage(t *testing.T, files map[string][]byte) string {
	t.Helper()
	for _, v := range []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_WORK_TREE", "GIT_OBJECT_DIRECTORY"} {
		if old, ok := os.LookupEnv(v); ok {
			_ = os.Unsetenv(v)
			t.Cleanup(func() { _ = os.Setenv(v, old) })
		}
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", "-f", "--", name)
	}
	return dir
}

func scanStaged(t *testing.T, dir string) []Finding {
	t.Helper()
	paths, diff, err := Change(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	return Scan(paths, ParseAdded(diff), NewShapes())
}

// A *.env file force-added past .gitignore is refused as a secret path.
func TestAStagedForceAddedEnvFileIsRefused(t *testing.T) {
	dir := stage(t, map[string][]byte{"prod.env": []byte("MODE=prod\n"), "readme.md": []byte("hello\n")})
	got := scanStaged(t, dir)
	if len(got) != 1 || got[0].Path != "prod.env" || got[0].Kind != "secret path" {
		t.Fatalf("staged prod.env gave %v, want one secret-path finding on prod.env", got)
	}
}

// A key behind a NUL byte - a file git takes for binary - is still read
// (--text) and refused, naming the path. The key is assembled at run time
// (shapes), so this file holds none.
func TestAKeyInAStagedBinaryFileIsRefused(t *testing.T) {
	body := append([]byte("head\x00\x01\x02\n"), []byte(shapes()["aws-key"]+"\n")...)
	dir := stage(t, map[string][]byte{"blob.bin": body})
	got := scanStaged(t, dir)
	if len(got) != 1 || got[0].Path != "blob.bin" || got[0].Kind != "aws-key" {
		t.Fatalf("staged binary with a key gave %v, want one aws-key finding on blob.bin", got)
	}
}

// A textconv driver named in .gitattributes is never run: on Git for Windows
// diff=astextplain on *.pdf and *.docx made git diff fail and every commit
// staging one was refused. Here the driver fails everywhere ("false"), and the
// key in the stored bytes is still read - in the staged form and in -commit.
func TestAFailingTextconvDriverNeitherFailsNorHidesTheScan(t *testing.T) {
	key := shapes()["aws-key"]
	dir := stage(t, map[string][]byte{
		".gitattributes": []byte("*.pdf diff=boom\n"),
		"doc.pdf":        []byte("%PDF-1.4\n" + key + "\n"),
	})
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "core.hooksPath=" + t.TempDir()}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("config", "diff.boom.textconv", "false")
	want := func(form string, got []Finding) {
		t.Helper()
		if len(got) != 1 || got[0].Path != "doc.pdf" || got[0].Kind != "aws-key" {
			t.Fatalf("%s: a key in doc.pdf behind a failing textconv gave %v, want one aws-key finding on doc.pdf", form, got)
		}
	}
	want("staged", scanStaged(t, dir))

	// -commit compares a commit with its first parent: give doc.pdf a parent.
	run("rm", "-q", "--cached", "doc.pdf")
	run("commit", "-q", "-m", "base")
	run("add", "-f", "--", "doc.pdf")
	run("commit", "-q", "-m", "pdf")
	paths, diff, err := Change(dir, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	want("-commit", Scan(paths, ParseAdded(diff), NewShapes()))
}

// Drift guard, both ways: this repository's .gitignore holds the one
// secret-path list exactly in its Secrets block (from the "# Secrets" comment
// to the next blank line) - no entry missing, none extra - and no other line
// ignores a secret-looking path the list does not name (a force-added one
// would pass the hook).
func TestTheRepositoryIgnoresExactlyTheSecretList(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, p := range secrets.GitignorePatterns() {
		listed[p] = true
	}
	// Anchored at a name's start or after a separator: venv/ is not an env pattern.
	secretLooking := regexp.MustCompile(`(?i)(^|[^a-z0-9])(env|secret|key|token|cred|pass|id_[rde])`)
	block, inBlock, seenBlock := map[string]bool{}, false, false
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "# Secrets"):
			inBlock, seenBlock = true, true
			continue
		case l == "":
			inBlock = false
			continue
		case strings.HasPrefix(l, "#"):
			continue
		}
		if inBlock {
			block[l] = true
			if !listed[l] {
				t.Errorf(".gitignore's Secrets block has %q, which secrets.GitignorePatterns lacks", l)
			}
			continue
		}
		if listed[l] || secretLooking.MatchString(l) {
			t.Errorf(".gitignore ignores %q outside its Secrets block: add it to secrets.GitignorePatterns and the block", l)
		}
	}
	if !seenBlock {
		t.Fatal(`.gitignore has no "# Secrets" block`)
	}
	for p := range listed {
		if !block[p] {
			t.Errorf(".gitignore's Secrets block lacks %q (secrets.GitignorePatterns)", p)
		}
	}
}
