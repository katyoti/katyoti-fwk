// SPDX-License-Identifier: Apache-2.0

package census

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// The commands a worker follows: every embedded task and skill of every binary
// (app/*/embed/tasks, app/*/embed/skills) names `ktw-agents …` verbs and
// `action run --uri <sidecar>:<slot>/<method>` methods to the letter, and one
// that does not exist stops it on « unknown command » or « no such method ».
// The extraction is shared here; each binary's census checks what it owns -
// ktw-agents its command tree, a sidecar the methods it publishes.

// EmbeddedDoc is one embedded task or skill: its path from the repository
// root and its text.
type EmbeddedDoc struct {
	Path string
	Text string
}

// EmbeddedDocs answers every embedded task and skill of the repository's
// binaries. It fails the test when none is found: a check over nothing
// proves nothing.
func EmbeddedDocs(t *testing.T) []EmbeddedDoc {
	t.Helper()
	root := repoRoot(t)
	var files []string
	for _, kind := range []string{"tasks", "skills"} {
		base, _ := filepath.Glob(filepath.Join(root, "app", "*", "embed", kind))
		for _, dir := range base {
			_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
					files = append(files, p)
				}
				return nil
			})
		}
	}
	if len(files) == 0 {
		t.Fatal("census: no embedded task or skill found: the check would prove nothing")
	}
	sort.Strings(files)
	out := make([]EmbeddedDoc, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, f)
		out = append(out, EmbeddedDoc{Path: filepath.ToSlash(rel), Text: string(raw)})
	}
	return out
}

var (
	codeSpan  = regexp.MustCompile("`ktw-agents ([^`]*)`")
	codeLine  = regexp.MustCompile(`^\s*(?:\$ )?ktw-agents (.*)$`)
	fence     = regexp.MustCompile("^[ \t]*(```|~~~)")
	verbWord  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	actionURI = regexp.MustCompile(`--uri\s+([a-z0-9-]+:[a-z0-9-]+/[a-z0-9-]+(?:/[a-z0-9-]+)?)(?:[\s'"` + "`" + `|),]|$)`)
)

// CommandWords answers the command words of every `ktw-agents …` a text
// names - in a code span, or a line of a fenced code block starting with it
// (a prose line starting with « ktw-agents has … » is no command): the words
// after ktw-agents up to the first that is not a verb (a flag, a placeholder,
// a quote).
func CommandWords(md string) [][]string {
	var found []string
	for _, m := range codeSpan.FindAllStringSubmatch(md, -1) {
		found = append(found, m[1])
	}
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if m := codeLine.FindStringSubmatch(strings.TrimSpace(line)); inFence && m != nil {
			found = append(found, m[1])
		}
	}
	var out [][]string
	for _, f := range found {
		var words []string
		for _, w := range strings.Fields(f) {
			if !verbWord.MatchString(w) {
				break
			}
			words = append(words, w)
		}
		if len(words) > 0 {
			out = append(out, words)
		}
	}
	return out
}

// ActionURIs answers the method URIs a text runs through
// `action run --uri <sidecar>:<slot>/<method>` - a literal URI only, never a
// placeholder (`<uri>`), the line break after --uri allowed.
func ActionURIs(md string) []string {
	var out []string
	for _, m := range actionURI.FindAllStringSubmatch(md, -1) {
		out = append(out, m[1])
	}
	return out
}

// AssertActionURIsResolve fails for every `<provider>:…` method URI an
// embedded task or skill runs that the provider's application - built and run
// by its census - does not publish: no service on that slot, or no such
// method on it. The actions' own slots (tasks, scripts, mutations) are not
// methods and are left out. It fails as well when the embedded documents name
// no URI of the provider while want says they should (want > 0 is the least
// number expected), so a broken extraction is seen.
func AssertActionURIsResolve(t *testing.T, provider string, app *application.Application, want int) {
	t.Helper()
	methods := publishedMethods(app)
	checked := 0
	for _, d := range EmbeddedDocs(t) {
		for _, raw := range ActionURIs(d.Text) {
			uri, err := mbapi.ParseToolURI(raw)
			if err != nil {
				t.Errorf("%s runs `%s`: not a method URI: %v", d.Path, raw, err)
				continue
			}
			if uri.Provider != provider {
				continue
			}
			switch uri.SlotType {
			case "tasks", "scripts", "mutations":
				continue
			}
			checked++
			slot, ok := methods[uri.SlotType]
			if !ok {
				t.Errorf("%s runs `%s`: %s publishes no service %s", d.Path, raw, provider, uri.SlotType)
				continue
			}
			if !slot[uri.Method] {
				t.Errorf("%s runs `%s`: %s's service %s has no method %s", d.Path, raw, provider, uri.SlotType, uri.Method)
			}
		}
	}
	if checked < want {
		t.Errorf("census[%s]: %d `action run --uri %s:…` method found in the embedded tasks and skills, at least %d expected: the extraction is broken", provider, checked, provider, want)
	}
}

// publishedMethods answers, per slot (a service's bus name), the kebab names
// of the methods the application publishes on the bus.
func publishedMethods(app *application.Application) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, ps := range application.UseAppExtensions[publish.PublishedService](app) {
		if ps.Local || ps.Definition == nil || !ps.HasCapability("ktsp-container") {
			continue
		}
		slot := ps.Definition.ResolvedBusName()
		if out[slot] == nil {
			out[slot] = map[string]bool{}
		}
		for name := range ps.Definition.Methods {
			out[slot][corereflect.ToKebab(name)] = true
		}
		if ps.ServiceType != nil {
			for i := 0; i < ps.ServiceType.NumMethod(); i++ {
				out[slot][corereflect.ToKebab(ps.ServiceType.Method(i).Name)] = true
			}
		}
	}
	return out
}
