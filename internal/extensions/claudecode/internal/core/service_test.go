// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/driver"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/process"
	cctest "github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/testing"
)

// fakeStarter starts fake Claudes that answer the handshake and read the
// prompt, recording each spec.
func fakeStarter(t *testing.T, specs chan<- process.Spec) Starter {
	return func(s process.Spec) (driver.Conn, error) {
		specs <- s
		f := cctest.New()
		go func() {
			m, err := f.Read()
			if err != nil {
				return
			}
			id, _ := cctest.Request(m)
			_ = f.Write(cctest.InitializeOK(id))
			if _, err := f.Read(); err != nil {
				return
			}
			_ = f.Write(cctest.SystemInit("sess", "none"))
			for {
				if _, err := f.Read(); err != nil {
					return
				}
			}
		}()
		return f, nil
	}
}

func fakeClaude(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "claude.exe")
	if err := os.WriteFile(p, []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestWorkerCapStartsWithTheDecidedDefaults: a worker starts in the decided
// permission mode with the person's settings, in its working directory, with
// the API keys removed; a worker over the cap is refused.
func TestWorkerCap(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "fake-api-key-should-not-pass")
	specs := make(chan process.Spec, 4)
	s := NewWith(ClaudeCodeConfig{ClaudePath: fakeClaude(t), MaxWorkers: 2}, fakeStarter(t, specs))
	dir := t.TempDir()
	w1, err := s.Start(context.Background(), api.StartInput{Prompt: "one", Cwd: dir, Name: "first"})
	if err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	spec := <-specs
	args := strings.Join(spec.Args, " ")
	for _, want := range []string{"--permission-mode auto", "--setting-sources=user,project,local", "--permission-prompt-tool stdio", "--name first"} {
		if !strings.Contains(args, want) {
			t.Errorf("args lack %q: %s", want, args)
		}
	}
	if spec.Dir != dir {
		t.Errorf("dir = %q, want %q", spec.Dir, dir)
	}
	if strings.Contains(strings.Join(spec.Env, "\n"), "fake-api-key-should-not-pass") {
		t.Errorf("the API key reached the worker's environment")
	}
	if w1.State != api.StateRunning {
		t.Errorf("first worker state = %s", w1.State)
	}
	if _, err := s.Start(context.Background(), api.StartInput{Prompt: "two"}); err != nil {
		t.Fatalf("Start 2: %v", err)
	}
	<-specs
	if _, err := s.Start(context.Background(), api.StartInput{Prompt: "three"}); err == nil || !strings.Contains(err.Error(), "max_workers") {
		t.Fatalf("Start 3 = %v, want the cap refusal", err)
	}
	list, _ := s.List(context.Background(), api.ListInput{})
	if len(list.Workers) != 2 {
		t.Errorf("list = %d workers, want 2", len(list.Workers))
	}
	if _, err := s.Stop(context.Background(), api.WorkerInput{ID: w1.ID}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if i, _ := s.Get(context.Background(), api.WorkerInput{ID: w1.ID}); i.State.Ended() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stopped worker did not end")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := s.Start(context.Background(), api.StartInput{Prompt: "three again"}); err != nil {
		t.Errorf("Start after a stop = %v, want room under the cap", err)
	}
	_ = s.Close()
}

// TestDriverRemoteControl: a worker started with Remote Control asks for it
// after the handshake and before its first prompt; its session URL and its
// bridge state are on the worker.
func TestDriverRemoteControl(t *testing.T) {
	order := make(chan string, 4)
	s := NewWith(ClaudeCodeConfig{ClaudePath: fakeClaude(t)}, func(process.Spec) (driver.Conn, error) {
		f := cctest.New()
		go func() {
			m, _ := f.Read()
			id, _ := cctest.Request(m)
			_ = f.Write(cctest.InitializeOK(id))
			m, _ = f.Read()
			id, req := cctest.Request(m)
			order <- fmt.Sprint(req["subtype"], " ", req["name"], " ", req["enabled"])
			_ = f.Write(cctest.BridgeState("ready"))
			_ = f.Write(cctest.RemoteControlOK(id, "https://claude.ai/code/session_test"))
			_ = f.Write(cctest.BridgeState("connected"))
			m, _ = f.Read()
			order <- fmt.Sprint(m["type"])
			for {
				if _, err := f.Read(); err != nil {
					return
				}
			}
		}()
		return f, nil
	})
	w, err := s.Start(context.Background(), api.StartInput{Prompt: "go", Name: "watched", RemoteControl: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := <-order; got != "remote_control watched true" {
		t.Errorf("first request after the handshake = %q, want remote_control watched true", got)
	}
	if got := <-order; got != "user" {
		t.Errorf("after remote control the host wrote %q, want the prompt", got)
	}
	if w.RemoteURL != "https://claude.ai/code/session_test" {
		t.Errorf("remote url = %q", w.RemoteURL)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		i, _ := s.Get(context.Background(), api.WorkerInput{ID: w.ID})
		if i.RemoteState == "connected" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("remote state = %q, want connected", i.RemoteState)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = s.Close()
}
