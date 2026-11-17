// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// hangingRunner is the fake runner, except that the programs it names never
// end their install: they wait until they are stopped.
type hangingRunner struct {
	*fakeRunner
	hang map[string]bool
}

func (h *hangingRunner) Install(ctx context.Context, s Step) error {
	if h.hang[s.Program] {
		<-ctx.Done()
		return ctx.Err()
	}
	return h.fakeRunner.Install(ctx, s)
}

func (h *hangingRunner) InstallElevated(ctx context.Context, steps []Step) error {
	for _, s := range steps {
		if h.hang[s.Program] {
			<-ctx.Done()
			return ctx.Err()
		}
	}
	return h.fakeRunner.InstallElevated(ctx, steps)
}

// installWithin installs needs and fails the test when the install has not
// ended within 5 s: a hanging step that is never stopped.
func installWithin(t *testing.T, e *Engine, needs []Need) mktapi.PrereqsInstallOutput {
	t.Helper()
	done := make(chan mktapi.PrereqsInstallOutput, 1)
	go func() {
		out, _ := e.Install(context.Background(), e.Plan(needs), nil, false, nil)
		done <- out
	}()
	select {
	case out := <-done:
		return out
	case <-time.After(5 * time.Second):
		t.Fatal("the install did not end: a hanging step is not stopped")
		return mktapi.PrereqsInstallOutput{}
	}
}

func step0(out mktapi.PrereqsInstallOutput, name string) mktapi.PrereqStep {
	for _, s := range out.Steps {
		if s.Program == name {
			return s
		}
	}
	return mktapi.PrereqStep{}
}

// A program whose install never ends is stopped after the step timeout and
// fails naming itself; the programs after it still install, and each step is
// said as it starts - so a hang names its program while it hangs.
func TestInstall_AStepThatHangsFailsNamingItsProgram(t *testing.T) {
	needs := []Need{
		{Sidecar: "a", Name: "pandoc", Required: true, Checked: true, Recipes: []publish.InstallRecipe{winget("JohnMacFarlane.Pandoc")}},
		{Sidecar: "a", Name: "typst", Required: true, Checked: true, Recipes: []publish.InstallRecipe{winget("Typst.Typst")}},
	}
	h := &hangingRunner{fakeRunner: newFake(), hang: map[string]bool{"pandoc": true}}
	var said bytes.Buffer
	e := &Engine{Runner: h, Platform: "windows", Progress: &said, StepTimeout: 50 * time.Millisecond}
	out := installWithin(t, e, needs)
	if st := step0(out, "pandoc"); st.Status != mktapi.StepFailed || !strings.Contains(st.Detail, "pandoc: no end after 50ms") {
		t.Errorf("pandoc step %+v, want failed naming pandoc and the timeout", st)
	}
	if st := step0(out, "typst"); st.Status != mktapi.StepInstalled {
		t.Errorf("typst step %+v, want installed after pandoc's hang", st)
	}
	if !strings.Contains(said.String(), "installing pandoc (") || !strings.Contains(said.String(), "installing typst (") {
		t.Errorf("progress %q, want each program said as it starts", said.String())
	}
}

// The administrator steps run behind one prompt: when it never answers (a
// prompt nobody sees), they are stopped and fail naming their programs.
func TestInstall_AnAdministratorPromptNobodyAnswersFailsNamingItsPrograms(t *testing.T) {
	needs := []Need{
		{Sidecar: "a", Name: "graphviz", Required: true, Checked: true, Recipes: []publish.InstallRecipe{apt("graphviz")}},
		{Sidecar: "a", Name: "pandoc", Required: true, Checked: true, Recipes: []publish.InstallRecipe{apt("pandoc")}},
	}
	h := &hangingRunner{fakeRunner: newFake(), hang: map[string]bool{"graphviz": true}}
	var said bytes.Buffer
	e := &Engine{Runner: h, Platform: "linux", Progress: &said, StepTimeout: 50 * time.Millisecond}
	out := installWithin(t, e, needs)
	for _, n := range []string{"graphviz", "pandoc"} {
		if st := step0(out, n); st.Status != mktapi.StepFailed || !strings.Contains(st.Detail, "the administrator steps (graphviz, pandoc): no end after 50ms") {
			t.Errorf("%s step %+v, want failed naming the administrator steps and the timeout", n, st)
		}
	}
	if !strings.Contains(said.String(), "installing as administrator, behind one prompt: graphviz, pandoc") {
		t.Errorf("progress %q, want the administrator steps said", said.String())
	}
}

// A stopped command returns even when a program it started still holds its
// output open (an installer winget launched): Run is not left waiting on it.
func TestAStoppedCommandReturnsThoughItsChildHoldsItsOutput(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the installers this guards run on Windows")
	}
	old := waitDelay
	waitDelay = 500 * time.Millisecond
	defer func() { waitDelay = old }()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	// cmd is stopped; the powershell it started keeps the output open for 20 s.
	err := runCommand(ctx, []string{"cmd", "/c", "powershell", "-NoProfile", "-Command", "Start-Sleep -Seconds 20"})
	if took := time.Since(start); took > 8*time.Second {
		t.Fatalf("runCommand took %s after its stop: it waited on the child's output", took)
	}
	if err == nil {
		t.Fatal("a stopped command reported no error")
	}
}
