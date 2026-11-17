// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
)

// THE ONE RULE OF THE COMMAND LINE AND THE HUB: every
// verb finds the running hub or spawns one; a spawned hub ends once no
// command uses it unless kept; the exclusions never spawn.
func TestHubModeFor(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  string
		mode hubclient.Mode
		keep bool
	}{
		{"a verb spawns if absent", []string{"requests", "file", "--module", "events"}, "", hubclient.ModeFindOrSpawn, false},
		{"a cwm verb spawns if absent", []string{"cwm", "object-get", "--id", "x"}, "", hubclient.ModeFindOrSpawn, false},
		{"action run spawns if absent", []string{"action", "run", "--uri", "ktw-methods:decisions/resolve"}, "", hubclient.ModeFindOrSpawn, false},
		{"run still spawns", []string{"run", "intake"}, "", hubclient.ModeFindOrSpawn, false},
		{"global value flags before the verb are skipped", []string{"--home", "x", "--universe", "dev", "todos", "pull"}, "", hubclient.ModeFindOrSpawn, false},
		{"--hub keep detaches", []string{"--hub", "keep", "todos", "pull"}, "", hubclient.ModeFindOrSpawn, true},
		{"--hub=keep detaches", []string{"todos", "pull", "--hub=keep"}, "", hubclient.ModeFindOrSpawn, true},
		{"KTW_HUB=keep detaches", []string{"todos", "pull"}, "keep", hubclient.ModeFindOrSpawn, true},
		{"--hub stop overrides the env", []string{"--hub", "stop", "todos", "pull"}, "keep", hubclient.ModeFindOrSpawn, false},
		{"sc never spawns: ktw-agents as a hub-managed sidecar", []string{"sc", "ktsp", "nats"}, "keep", hubclient.ModeReuse, false},
		{"llm never spawns", []string{"llm", "info"}, "", hubclient.ModeReuse, false},
		{"--help never spawns", []string{"requests", "file", "--help"}, "", hubclient.ModeReuse, false},
		{"--version never spawns", []string{"--version"}, "", hubclient.ModeReuse, false},
		{"version never spawns", []string{"version"}, "", hubclient.ModeReuse, false},
		{"no verb never spawns", []string{}, "", hubclient.ModeReuse, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mode, keep := hubModeFor(c.args, c.env)
			if mode != c.mode || keep != c.keep {
				t.Fatalf("hubModeFor(%v, %q) = (%v, %v), want (%v, %v)", c.args, c.env, mode, keep, c.mode, c.keep)
			}
		})
	}
}

// A CLIENT VERB WAITS AS LONG AS IT WOULD HAVE HERE: a method with no
// advertised timeout ran in-process without one, so over the hub it gets
// clientCallTimeout, never the bus's few seconds (a `requests check` running
// tests took 29 s and was cut at 5).
func TestClientCallTimeoutIsLong(t *testing.T) {
	if clientCallTimeout < 5*time.Minute {
		t.Fatalf("clientCallTimeout = %s: a long verb sent to the serving process would be cut short", clientCallTimeout)
	}
}

// WHO IS A CLIENT OF A RUNNING UNIVERSE: every verb — `--help`, `llm` and
// `version` too, which spawn no hub but reuse one that runs (opening the
// workspace here would meet the serving process's lock) — except the serving
// process itself (`sc …`) and the commands that work on the workspace here.
func TestWantsClient(t *testing.T) {
	cases := map[string]struct {
		args []string
		want bool
	}{
		"a verb":                      {[]string{"requests", "file"}, true},
		"--help reuses a hub":         {[]string{"work", "edit", "--help"}, true},
		"llm reuses a hub":            {[]string{"llm", "info"}, true},
		"the serving process":         {[]string{"sc", "ktsp", "nats"}, false},
		"run works here":              {[]string{"run", "intake"}, false},
		"a method command works here": {[]string{"method-act"}, false},
	}
	for name, c := range cases {
		if got := wantsClient(c.args); got != c.want {
			t.Errorf("%s: wantsClient(%v) = %v, want %v", name, c.args, got, c.want)
		}
	}
}

// A VERB TOLD ANOTHER WORKSPACE IS NOT A CLIENT: the
// serving process holds its universe's workspace only; a verb whose
// --workspace-dir (else KTW_WORKSPACE_DIR) names another folder works on it
// in its own process, never sent to the serving one.
func TestAVerbToldAnotherWorkspaceIsNotSentToTheServingProcess(t *testing.T) {
	served := filepath.Join(t.TempDir(), "dev", ".katyoti")
	other := filepath.Join(t.TempDir(), "scratch", ".katyoti")
	cases := map[string]struct {
		args []string
		env  string
		want bool
	}{
		"no workspace named":                 {[]string{"releases", "plan"}, "", true},
		"the universe's own, by the flag":    {[]string{"releases", "plan", "--workspace-dir", served}, "", true},
		"the universe's own, by the env":     {[]string{"releases", "plan"}, served, true},
		"another, by the flag":               {[]string{"releases", "plan", "--workspace-dir", other}, "", false},
		"another, by the flag's = form":      {[]string{"releases", "plan", "--workspace-dir=" + other}, "", false},
		"another, by the env":                {[]string{"releases", "plan"}, other, false},
		"the flag wins over the env":         {[]string{"releases", "plan", "--workspace-dir", other}, served, false},
		"the universe's own, written longer": {[]string{"releases", "plan", "--workspace-dir", served + string(filepath.Separator) + "."}, "", true},
	}
	for name, c := range cases {
		if got := servesWorkspace(served, namedWorkspace(append([]string{"ktw-agents"}, c.args...), c.env)); got != c.want {
			t.Errorf("%s: served by the universe's process = %v, want %v", name, got, c.want)
		}
	}
}
