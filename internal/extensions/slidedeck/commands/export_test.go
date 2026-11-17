// SPDX-License-Identifier: Apache-2.0

package commands_test

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/commands"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// export runs `deck export <a two-slide deck> [flags]` and answers its output,
// its error and how long it took.
func export(t *testing.T, flags ...string) (string, error, time.Duration) {
	t.Helper()
	ctx, _ := cwmapp.New(t, contrib.ContributeLayouts("ktw-test"))
	deck := filepath.Join(t.TempDir(), "two.deck.md")
	if err := os.WriteFile(deck, []byte("---\ntitle: Two\n---\n\n# One\n\n---\n\n# Two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := commands.NewDeckCommand()
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"export", deck}, flags...))
	start := time.Now()
	err := cmd.Execute()
	return out.String(), err, time.Since(start)
}

// No display server anywhere: the command stops at once and says what is
// missing, every place it looked and how to provide one — never a timeout.
func TestExportNamesTheMissingDisplayServer(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "nowhere", "ktw-display.exe")
	t.Setenv("KTW_DISPLAY_BIN", missing)
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	_, err, took := export(t, "--format", "png")
	if err == nil {
		t.Fatal("export succeeded with no display server")
	}
	msg := err.Error()
	for _, want := range []string{
		"display server (ktw-display)", "found none", "Looked at",
		"KTW_DISPLAY_BIN=" + missing, filepath.Join(root, "sidecars.yaml"),
		filepath.Join("app", "ktw-display", "dist"),
		"make ktw-display", "--display",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if took > 5*time.Second {
		t.Errorf("took %s: a missing server must fail at once", took)
	}
}

// --display names a server that does not answer: the command says so at once,
// with the URL it tried and how to start one.
func TestExportNamesAnUnreachableDisplayServer(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	_ = l.Close() // nothing listens there now

	_, err, took := export(t, "--format", "pptx", "--display", "http://127.0.0.1:"+port+"/?token=x")
	if err == nil {
		t.Fatal("export succeeded with no display server")
	}
	msg := err.Error()
	for _, want := range []string{
		"display server (ktw-display)", "none answers at http://127.0.0.1:" + port + "/healthz",
		"KTW_DISPLAY_PORT=", "KTW_DISPLAY_TOKEN=", "leave out --display",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if took > 5*time.Second {
		t.Errorf("took %s: an unreachable server must fail at once", took)
	}
}

func TestExportRefusesAnUnknownFormat(t *testing.T) {
	_, err, _ := export(t, "--format", "gif")
	if err == nil || !strings.Contains(err.Error(), "pptx, pptx-pictures, html or png") {
		t.Fatalf("err = %v; want the formats named", err)
	}
}

// --display names a server that answers but refuses the token: the command
// says the token is wrong, not a bare 401.
func TestExportNamesARefusedToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()
	_, err, _ := export(t, "--format", "html", "--display", srv.URL+"/?token=wrong")
	if err == nil || !strings.Contains(err.Error(), "refused the token") {
		t.Fatalf("err = %v; want the refused token named", err)
	}
}
