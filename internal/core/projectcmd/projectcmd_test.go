// SPDX-License-Identifier: Apache-2.0

package projectcmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// requireBash skips where the machine has no bash (Run then falls back to the
// platform's shell, tested apart).
func requireBash(t *testing.T) {
	t.Helper()
	if _, err := FindBash(); err != nil {
		t.Skipf("no bash on this machine: %v", err)
	}
}

// A command is POSIX shell on every platform: && chains, a failed first
// command stops the chain and fails the run, $(...) and pipes work, the
// project is the working folder (a Windows path quoted in the command still
// works) and the environment is kept.
func TestRunIsPOSIXShellEverywhere(t *testing.T) {
	requireBash(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECTCMD_PROBE", "kept")
	ctx := context.Background()

	out, err := run(ctx, dir, "true && echo ok", time.Minute)
	if err != nil || strings.TrimSpace(out) != "ok" {
		t.Fatalf("true && echo ok: out %q err %v, want ok and no error", out, err)
	}
	out, err = run(ctx, dir, "false && echo x", time.Minute)
	if err == nil || strings.Contains(out, "x") {
		t.Fatalf("false && echo x: out %q err %v, want a failure that printed nothing", out, err)
	}
	for _, cmd := range []string{
		`n=$(printf 'a\nb\nc\n' | grep -c .); test "$n" -eq 3 && echo three`,
		`test -f marker.txt && echo here`,
		`cd "` + dir + `" && test -f marker.txt && echo here`,
		`test "$PROJECTCMD_PROBE" = kept && echo here`,
	} {
		if out, err := run(ctx, dir, cmd, time.Minute); err != nil || strings.TrimSpace(out) == "" {
			t.Fatalf("%s: out %q err %v, want it to pass", cmd, out, err)
		}
	}
}

// A command that overruns its bound is stopped — with the processes it
// started, even one holding the output open — and answers ErrTimedOut with the
// bound.
func TestRunStopsAnOverrunningCommand(t *testing.T) {
	requireBash(t)
	start := time.Now()
	// bash forks for sleep (it is not the last command): the child, not bash,
	// holds the output open.
	_, err := run(context.Background(), t.TempDir(), "sleep 60 && echo late", 2*time.Second)
	if !errors.Is(err, ErrTimedOut) || !strings.Contains(err.Error(), "timed out after 2s") {
		t.Fatalf("err %v, want timed out after 2s", err)
	}
	if took := time.Since(start); took >= 2*time.Second+waitDelay {
		t.Fatalf("Run took %s: the command's child was not stopped with it and held the output until WaitDelay", took)
	}
}

// /: a command runs in bash, its POSIX syntax
// with it; with no bash it does not run at all - no other shell - and the
// error says how to get one.
func TestACommandRunsOnlyInBash(t *testing.T) {
	dir := t.TempDir()
	out, err := RunIn(context.Background(), dir, `[[ 2 -gt 1 ]] && ! grep -q x /dev/null && echo "in bash"`, time.Minute)
	if err != nil || !strings.Contains(out, "in bash") {
		t.Fatalf("a bash command: %q %v", out, err)
	}

	kept := bashFor
	t.Cleanup(func() { bashFor = kept })
	bashFor = func() (string, error) { return "", ErrNoBash }
	marker := filepath.Join(dir, "ran")
	_, err = RunIn(context.Background(), dir, "echo > ran", time.Minute)
	if !errors.Is(err, ErrNoBash) || !strings.Contains(err.Error(), "no bash found") {
		t.Fatalf("no bash: err %v, want ErrNoBash saying how to get one", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("with no bash the command ran through another shell")
	}
}

// Windows' own bash.exe starts WSL: it is never the bash a command runs through.
func TestFindBashIsNeverTheWSLLauncher(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the WSL launcher is a Windows file")
	}
	for p, want := range map[string]bool{
		`C:\Windows\System32\bash.exe`:                             true,
		`C:\Users\me\AppData\Local\Microsoft\WindowsApps\bash.exe`: true,
		`C:\Program Files\Git\bin\bash.exe`:                        false,
		`C:\Users\me\AppData\Local\Programs\Git\usr\bin\bash.exe`:  false,
	} {
		if got := isWSLLauncher(p); got != want {
			t.Errorf("isWSLLauncher(%q) = %v, want %v", p, got, want)
		}
	}
	bash, err := FindBash()
	if err != nil {
		t.Skipf("no bash on this machine: %v", err)
	}
	if isWSLLauncher(bash) {
		t.Fatalf("FindBash = %s, the WSL launcher", bash)
	}
	t.Logf("bash: %s", bash)
}

// A declared bound is read once: none keeps the caller's default; a duration
// above zero and within the ceiling is the bound; anything else is refused,
// naming why.
func TestParseTimeout(t *testing.T) {
	for _, c := range []struct {
		in   string
		want time.Duration
		ok   bool
		err  string
	}{
		{in: "", ok: false},
		{in: "  ", ok: false},
		{in: "30m", want: 30 * time.Minute, ok: true},
		{in: "1h", want: time.Hour, ok: true},
		{in: "90s", want: 90 * time.Second, ok: true},
		{in: "61m", err: "above the ceiling of 1h0m0s"},
		{in: "0s", err: "not above zero"},
		{in: "-5m", err: "not above zero"},
		{in: "30", err: "not a duration"},
		{in: "half an hour", err: "not a duration"},
	} {
		d, ok, err := ParseTimeout(c.in)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) || ok {
				t.Errorf("ParseTimeout(%q) = %s, %v, %v; want an error saying %q", c.in, d, ok, err, c.err)
			}
			continue
		}
		if err != nil || ok != c.ok || d != c.want {
			t.Errorf("ParseTimeout(%q) = %s, %v, %v; want %s, %v, no error", c.in, d, ok, err, c.want, c.ok)
		}
	}
}

// A probe the hub runs does not inherit the hub's identity:
// its installation, instance, root, bus, workspace, cache and .env keys are
// dropped, so a live GUI check it starts resolves its scenario case, never the
// hub's installation; every other variable is kept. A scenario command run in
// its case (RunIn) keeps the environment its sandbox gave it.
func TestAProbeDoesNotInheritTheHubsIdentity(t *testing.T) {
	requireBash(t)
	for _, name := range hubIdentity {
		t.Setenv(name, "the-hub")
	}
	// The project root a probe runs in is read from the workspace: a real one.
	t.Setenv("KTW_WORKSPACE", t.TempDir())
	t.Setenv("PROJECTCMD_OTHER", "kept")
	show := `echo "${KTW_INSTALLATION:-none} ${KTW_KATYOTI_ROOT:-none} ${KTW_NATS_URL:-none} ${KTW_DOTENV_KEYS:-none} $PROJECTCMD_OTHER"`
	out, err := Run(context.Background(), show, time.Minute)
	if err != nil || strings.TrimSpace(out) != "none none none none kept" {
		t.Fatalf("a probe run by the hub sees %q (%v); want none of the hub's identity and the rest kept", strings.TrimSpace(out), err)
	}
	out, err = RunIn(context.Background(), t.TempDir(), show, time.Minute)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(out), "the-hub") {
		t.Fatalf("a command run in its case sees %q (%v); want its environment kept", strings.TrimSpace(out), err)
	}
}
