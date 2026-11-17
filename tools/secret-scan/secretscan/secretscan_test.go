// SPDX-License-Identifier: Apache-2.0

package secretscan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The key is assembled at run time, so this file holds no secret shape.
func fakeKey() string { return "AK" + "IA" + strings.Repeat("ABCD", 4) }

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanTreeNamesEachFileHoldingASecretOrADenyListTerm(t *testing.T) {
	root := t.TempDir()
	// A binary: the key sits between NUL and high bytes.
	bin := append([]byte{0x7f, 'E', 'L', 'F', 0, 0, 0xff, 0x01}, []byte(fakeKey())...)
	bin = append(bin, 0, 0x02, 0x03)
	write(t, filepath.Join(root, "linux-amd64", "ktw-x"), bin)
	write(t, filepath.Join(root, "conf", ".env"), []byte("NAME=value\n"))
	write(t, filepath.Join(root, "packs", "readme.md"), []byte("line one\nmade for Zorblax Corp\n"))
	write(t, filepath.Join(root, "packs", "clean.txt"), []byte("nothing to see\n"))
	denyFile := filepath.Join(t.TempDir(), "deny.txt")
	write(t, denyFile, []byte("# a comment\n\n\\bzorblax\\b\n"))
	deny, err := LoadDenyList(denyFile)
	if err != nil {
		t.Fatal(err)
	}

	got, err := ScanTree(root, NewShapes(), deny)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, f := range got {
		lines = append(lines, f.String())
	}
	want := []string{
		"conf/.env: secret path",
		"linux-amd64/ktw-x:1: aws-key",
		`packs/readme.md:2: deny-list term \bzorblax\b`,
	}
	slices.Sort(lines)
	if !slices.Equal(lines, want) {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestScanTreePassesACleanFolderAndScansOneFile(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "b.txt"), []byte("plain words\x00\x01more words\n"))
	write(t, filepath.Join(root, "notes.md"), []byte("the key "+fakeKey()+" // "+FakeMarker+"\n"))
	got, err := ScanTree(root, NewShapes(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("a clean folder: %v %v", got, err)
	}
	one := filepath.Join(t.TempDir(), "notes.txt")
	write(t, one, []byte("x\n"+fakeKey()+"\n"))
	got, err = ScanTree(one, NewShapes(), nil)
	if err != nil || len(got) != 1 || got[0].String() != "notes.txt:2: aws-key" {
		t.Fatalf("one file: %v %v", got, err)
	}
}

func TestLoadDenyListRefusesABadExpression(t *testing.T) {
	f := filepath.Join(t.TempDir(), "deny.txt")
	write(t, f, []byte("ok\n(unclosed\n"))
	if _, err := LoadDenyList(f); err == nil || !strings.Contains(err.Error(), ":2:") {
		t.Fatalf("want the line named, got %v", err)
	}
}

func TestFilterLetsOnlyTheAllowedPathAndKindPass(t *testing.T) {
	f := filepath.Join(t.TempDir(), "allow.txt")
	write(t, f, []byte("# rclone's documentation sample\nktw-sync* url-credential\nlinux-*/ktw-hub bearer\n"))
	allow, err := LoadAllowList(f)
	if err != nil {
		t.Fatal(err)
	}
	in := []Finding{
		{Path: "windows-amd64/ktw-sync.exe", Line: 3, Kind: "url-credential"},
		{Path: "windows-amd64/ktw-sync.exe", Line: 4, Kind: "private-key"},
		{Path: "linux-amd64/ktw-hub", Line: 1, Kind: "bearer"},
		{Path: "windows-amd64/ktw-hub.exe", Line: 1, Kind: "bearer"},
	}
	var got []string
	for _, x := range Filter(in, allow) {
		got = append(got, x.String())
	}
	want := []string{"windows-amd64/ktw-sync.exe:4: private-key", "windows-amd64/ktw-hub.exe:1: bearer"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	write(t, f, []byte("no-kind\n"))
	if _, err := LoadAllowList(f); err == nil {
		t.Fatal("an entry without a kind is refused")
	}
}

func TestReleaseAllowReadsAndNamesOnlyKnownLibrarySamples(t *testing.T) {
	allow := ReleaseAllow()
	if len(allow) == 0 {
		t.Fatal("the release allow-list is empty")
	}
	in := []Finding{
		{Path: "ktw-sync.exe", Line: 9, Kind: "url-credential"},
		{Path: "ktw-hub", Line: 2, Kind: "bearer"},
		{Path: "ktw-hub.exe", Line: 2, Kind: "aws-key"},
		{Path: "katyoti.exe", Line: 1, Kind: "url-credential"},
	}
	if got := Filter(in, allow); len(got) != 2 || got[0].Kind != "aws-key" || got[1].Path != "katyoti.exe" {
		t.Fatalf("got %v", got)
	}
}

// NewShapes runs inside the long-running ktw-compose sidecar: building it
// must leave the process environment as it is, for every goroutine, the whole
// time - and a secret-named variable's value is not a shape.
func TestNewShapesLeavesTheEnvironmentUntouched(t *testing.T) {
	const probe = "SECRETSCAN_PROBE_TOKEN"
	value := strings.Repeat("plainvalue", 2)
	t.Setenv(probe, value)
	before := os.Environ()
	slices.Sort(before)

	stop := make(chan struct{})
	missed := make(chan bool, 1)
	go func() {
		seen := false
		for {
			select {
			case <-stop:
				missed <- seen
				return
			default:
				if os.Getenv(probe) != value {
					seen = true
				}
			}
		}
	}()
	var sh Shapes
	for range 300 {
		sh = NewShapes()
	}
	close(stop)
	if <-missed {
		t.Error("another goroutine saw the environment without " + probe + " while NewShapes was built")
	}
	after := os.Environ()
	slices.Sort(after)
	if !slices.Equal(before, after) {
		t.Errorf("os.Environ() changed:\nbefore %d entries\nafter  %d entries", len(before), len(after))
	}
	if k := sh("the value " + value); k != "" {
		t.Errorf("an environment value counted as a shape: %q", k)
	}
	if k := sh("key " + fakeKey()); k == "" {
		t.Error("the built-in shapes are gone")
	}
}

// A deny-list line is read as the publication gate reads it (ListLine, the one reading
// of the list format): a leading BOM stripped, a trailing « # reason » cut - not
// compiled into the expression, where it would make the term never match - and a
// finding printed as the term's line, never the term.
func TestLoadDenyListCutsATrailingReasonAndRefsTheLine(t *testing.T) {
	f := filepath.Join(t.TempDir(), "deny.txt")
	write(t, f, []byte(string(rune(0xfeff))+"# the private terms\n\\bzorblax\\b # a client name\n\tquux[#]9\t# a literal #\n"))
	deny, refs, err := LoadDenyListRefs(f)
	if err != nil || len(deny) != 2 {
		t.Fatalf("want two terms, got %d: %v", len(deny), err)
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "notes.md"), []byte("intro\nmade for Zorblax Corp\nquux#9\n"))
	got, err := ScanTree(root, NewShapes(), deny)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, f := range RefDenyTerms(got, refs) {
		lines = append(lines, f.String())
	}
	slices.Sort(lines)
	want := []string{
		"notes.md:2: deny-list term deny.txt line 2",
		"notes.md:3: deny-list term deny.txt line 3",
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestListLineReadsTheListFormat(t *testing.T) {
	for in, want := range map[string]string{
		"":                    "",
		"   ":                 "",
		"# a comment":         "",
		"  # an indented one": "",
		`\bterm\b`:            `\bterm\b`,
		"  term  ":            "term",
		"term # the reason":   "term",
		"term\t# the reason":  "term",
		"a#b":                 "a#b",
		"a[#]b # a literal #": "a[#]b",
		"term #":              "term",
	} {
		if got := ListLine(in); got != want {
			t.Errorf("ListLine(%q) = %q, want %q", in, got, want)
		}
	}
}
