// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE TO-DOS LIVE IN THE SCHEDULER'S STORE: since the
// migration a workspace's to-dos and waits are rows of the
// SQLite store, never committed files. No rule, guide, skill or module page
// names `.ktw/todo`, `.ktw/done` or `.ktw/waiting` as the store, nor asks to
// commit to-do files. The scheduler's own page still documents its file-store
// adapter: a paragraph that says « file store » may name its folders.
// Records are out of the sweep: research, dated audits, decisions and the
// planning spaces (`.ktw/`, change-log entries included).

var (
	todoFolderRE      = regexp.MustCompile(`\.ktw/(todo|done|waiting)\b`)
	commitTodoFilesRE = regexp.MustCompile(`(?i)\bto-do (and request )?files?\b`)
	commitRE          = regexp.MustCompile(`(?i)\bcommit`)
)

// todoStoreWordingProblems is every paragraph under root's guides, skills and
// module pages that names the to-do folders as the store or asks to commit
// to-do files, as "<path>: <first words>".
func todoStoreWordingProblems(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, f := range []string{"CLAUDE.md", "DEV_GUIDE.md"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			files = append(files, filepath.Join(root, f))
		}
	}
	walk := func(dir string, keep func(rel string) bool) {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			r := filepath.ToSlash(rel(root, p))
			if d.IsDir() {
				switch d.Name() {
				case ".ktw", "audits", "decisions", "node_modules":
					return filepath.SkipDir
				}
				if r == "modules/research" || strings.HasPrefix(r, "modules/research-") {
					return filepath.SkipDir
				}
				return nil
			}
			if keep(r) {
				files = append(files, p)
			}
			return nil
		})
	}
	md := func(r string) bool { return strings.HasSuffix(r, ".md") }
	walk(filepath.Join(root, ".claude", "skills"), func(r string) bool { return md(r) || strings.HasSuffix(r, ".sh") })
	walk(filepath.Join(root, "modules"), md)
	apps, _ := filepath.Glob(filepath.Join(root, "app", "*", "embed", "skills"))
	for _, a := range apps {
		walk(a, md)
	}

	var problems []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		problems = append(problems, todoStoreWordingIn(filepath.ToSlash(rel(root, f)), string(b))...)
	}
	sort.Strings(problems)
	return problems
}

// todoStoreWordingIn is the offending paragraphs of one text.
func todoStoreWordingIn(name, text string) []string {
	var out []string
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, para := range strings.Split(text, "\n\n") {
		bad := todoFolderRE.MatchString(para) && !strings.Contains(para, "file store")
		bad = bad || (commitRE.MatchString(para) && commitTodoFilesRE.MatchString(para))
		if !bad {
			continue
		}
		first := strings.TrimSpace(para)
		if len(first) > 100 {
			first = first[:100]
		}
		out = append(out, name+": "+strings.ReplaceAll(first, "\n", " "))
	}
	return out
}

func TestNoPageNamesTheToDoFoldersAsTheStore(t *testing.T) {
	root := repoRoot(t)
	if problems := todoStoreWordingProblems(t, root); len(problems) > 0 {
		t.Fatalf("to-dos are rows of the scheduler's store, never committed files - these paragraphs name .ktw/todo|done|waiting as the store or ask to commit to-do files; say what the store does (todos list/get/search/history, the export ref):\n  %s",
			strings.Join(problems, "\n  "))
	}
}

// TestTodoStoreWordingGuardCatchesASentence falsifies the guard on a text
// putting the old rule back.
func TestTodoStoreWordingGuardCatchesASentence(t *testing.T) {
	for _, bad := range []string{
		"- Commit the to-do and request files you change (`modules/<m>/.ktw/todo/`, `.ktw/done/`) by pathspec.",
		"A to-do is a file in `modules/<m>/.ktw/todo/`.",
		"Commit what a step changed with its request and to-do files.",
	} {
		if len(todoStoreWordingIn("x.md", bad)) == 0 {
			t.Errorf("guard missed: %s", bad)
		}
	}
	for _, good := range []string{
		"A to-do is a row of the store; on the file store, a file `modules/<m>/.ktw/todo/<id>.yaml`.",
		"Commit the request files (`modules/<m>/.ktw/requests/`); to-dos are the scheduler's rows.",
	} {
		if got := todoStoreWordingIn("x.md", good); len(got) > 0 {
			t.Errorf("guard refused a sound text: %v", got)
		}
	}
}

// todoHelpRE is a help line naming a to-do or a wait a file: the store's rows
// are no files (the migration's own help - moving the planning files in - and
// a to-do's history - the commits of its file in the git archive - say what
// was, and are kept).
var todoHelpRE = regexp.MustCompile(`(?i)\bas files\b|\ba file per\b|\bto-dos? (is|are) (a )?files?\b`)

// todoHelpProblems is every help line of the given services (the service's
// own, then each method's) that names to-dos or waits as files.
func todoHelpProblems(defs ...meta.ServiceDef) []string {
	var out []string
	for _, d := range defs {
		if todoHelpRE.MatchString(d.Short) {
			out = append(out, d.Name+": "+d.Short)
		}
		for name, m := range d.Methods {
			if todoHelpRE.MatchString(m.Short) {
				out = append(out, d.Name+" "+name+": "+m.Short)
			}
		}
	}
	sort.Strings(out)
	return out
}

// TestNoTodosHelpNamesTheToDosFiles: the todos and waits
// commands' help - a CLI line, out of the page sweep above - says to-dos and
// waits are the store's, never files.
func TestNoTodosHelpNamesTheToDosFiles(t *testing.T) {
	if problems := todoHelpProblems(schedapi.TodoServiceDef, schedapi.WaitServiceDef); len(problems) > 0 {
		t.Fatalf("to-dos and waits are rows of the scheduler's store - these help lines call them files:\n  %s", strings.Join(problems, "\n  "))
	}
	// The guard catches the old line.
	old := meta.ServiceDef{Name: "todos", Short: "The scheduler: each module's to-dos (work tickets, as files) handed out to workers"}
	if len(todoHelpProblems(old)) == 0 {
		t.Error("guard missed the old todos help line")
	}
}
