// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/driver"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/process"
	cctest "github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/testing"
)

// TestDriverRunInTheForeground: run prints the conversation, asks the
// permission on the terminal ("y" allows once), prints the result and ends
// the worker cleanly.
func TestDriverRunInTheForeground(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "claude.exe")
	_ = os.WriteFile(bin, []byte("MZ"), 0o755)
	allowed := make(chan map[string]any, 1)
	svc := core.NewWith(core.ClaudeCodeConfig{ClaudePath: bin}, func(process.Spec) (driver.Conn, error) {
		f := cctest.New()
		go func() {
			m, _ := f.Read()
			id, _ := cctest.Request(m)
			_ = f.Write(cctest.InitializeOK(id))
			_, _ = f.Read() // the prompt
			_ = f.Write(cctest.SystemInit("s1", "none"))
			_ = f.Write(cctest.AssistantText("I will run the tests."))
			_ = f.Write(cctest.CanUseTool("p1", "Bash", map[string]any{"command": "go test ./..."}))
			a, _ := f.Read()
			allowed <- a
			_ = f.Write(cctest.ToolResult("toolu_p1", "PASS", false))
			_ = f.Write(cctest.Result("s1", "All green.", false, 0.0123))
			for {
				if _, err := f.Read(); err != nil {
					return
				}
			}
		}()
		return f, nil
	})
	var out bytes.Buffer
	err := Run(context.Background(), svc, api.StartInput{Prompt: "run the tests"}, false, strings.NewReader("y\n"), &out)
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	a := <-allowed
	r, _ := a["response"].(map[string]any)
	body, _ := r["response"].(map[string]any)
	if body["behavior"] != "allow" {
		t.Errorf("the terminal's y did not allow: %v", a)
	}
	got := out.String()
	for _, want := range []string{"max subscription", "Claude: I will run the tests.", "Permission: Bash", "< ok PASS", "Result ($0.0123): All green.", "finished"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}
