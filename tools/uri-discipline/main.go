// SPDX-License-Identifier: Apache-2.0

// uri-discipline forbids literals that must not be hand-written.
//
// The table is the point: each rule names a literal, the places allowed to hold
// it and why, and what to use instead. Adding a rule is adding a row.
//
//   - `slot://` — the superseded tool-URI form. Tools use
//     `<provider>:<slot>[(<instance>)]/<method>`
//     (rule). Per
//     `feedback_no_legacy` there are no compat shims; the old form must be gone.
//   - `ktw-hub:/CatalogService/` — the aggregator's address. Hand-building it
//     means hand-building the paging, the decode and the wire shape too;
//     `internal/extensions/manifest/remote` is the one place that holds it, and
//     every consumer reads through the query surface instead.
//   - `yaml.Unmarshal` **in a file that reaches the config directory** — a
//     config file decoded by hand skips variable expansion and the
//     envkey/default/required tags, so its values differ from every other read
//     of the same file and nothing reports it.
//   - `LoadInto(` — a CLI affordance, never how a service obtains its settings.
//   - `:/data-type/task/` — a task's published schema address. The producer
//     names the contribution and the GUI looks it up; spelled out on both
//     sides, the two drift and the lookup 404s with no way to say why.
//     `mbapi.TaskSchemaID` / `TaskSchemaURI` are the one writer.
//
// This is an import-graph-free check: a forbidden STRING, which is why it stays
// separate from `import-discipline` rather than becoming a rule there.
//
// The rule this table holds is
// (modules/framework/rules/R-006-five-addresses-and-calls-are-never-hand-written.cwm.md).
//
// Run via: go run ./tools/uri-discipline
// Exits non-zero on violation; CI uses this in the `check` Make target.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// rule is one forbidden literal, with the single place allowed to hold it.
//
// A table rather than a constant, because the second rule arrived and a second
// TOOL would have been the same algorithm with different constants — which is
// exactly the duplication `import-discipline` was built to end (c02). One
// checker, one list.
type rule struct {
	// literal is the string no source file may contain.
	literal string
	// exempt are the paths that legitimately hold the literal. Each entry
	// needs a reason at its declaration: an exemption without one becomes a
	// place to hide, and a guard with hiding places is a guard people learn
	// to route around.
	exempt []string
	// withAny narrows the rule to files that ALSO contain one of these.
	//
	// Some literals are only wrong in company. `yaml.Unmarshal` is correct
	// everywhere content is parsed — dossiers, catalogs, kind declarations —
	// and wrong only when the same file is reaching into the config
	// directory, because that is a config file being read raw. Without the
	// conjunction the rule would fire on ~70 correct uses, and a guard that
	// is mostly noise is one people learn to ignore.
	//
	// Empty means the literal is forbidden on its own.
	withAny []string

	// hint says what to do instead. A guard that reports a violation without
	// naming the alternative is a guard people route around.
	hint string
}

var rules = []rule{
	{
		literal: "slot://",
		hint:    "use the `<provider>:<slot>[(<instance>)]/<method>` shape",
	},
	{
		// The aggregator's address. Hand-building it means hand-building the
		// paging, the decode and the wire shape too — which is how four
		// browsers in one binary ended up with four copies of a cursor loop,
		// in two different spellings of the same method name.
		literal: "ktw-hub:/CatalogService/",
		exempt: []string{
			// The one implementation. This is what the literal is forbidden
			// in favour of.
			"internal/extensions/manifest/remote",
			// The contract test for this exact service: naming the wire
			// address IS its subject. A test that asserted the contract
			// through the abstraction would not be testing the contract.
			"internal/test/catalog",
			// The harness that starts a real hub for those tests, for the
			// same reason.
			"internal/test/hubharness",
		},
		hint: "read the universe through the query surface: " +
			"manifestremote.New(sender) gives an mbapi.CatalogReader, and " +
			"mbapi.Query/QueryTyped/GetOne take it (c11, c25)",
	},
	{
		// A config file decoded by hand skips variable expansion and the
		// envkey / default / required tags — so its values differ from every
		// other read of the same file, and nothing reports the difference.
		//
		// Narrowed by `withAny` to files that reach the config directory:
		// parsing YAML is correct everywhere else.
		literal: "yaml.Unmarshal",
		withAny: []string{"ConfigDir()"},
		exempt: []string{
			// The config module is the implementation this rule points at.
			"internal/core/config",
			// The bootstrap layer BELOW the config service: it resolves the
			// paths the config service will later read from, so it cannot use
			// a service that does not exist yet. It is also where ConfigDir()
			// is defined, which is why the companion check matches here at
			// all.
			"internal/core/application",
			"internal/core/cenv",
			// The repair path: LoadManifest (the pipeline) has already
			// failed, and this re-reads raw ONLY to tell "missing name"
			// (healable) from "malformed YAML" (not). It is diagnosing a file
			// the pipeline rejected, not loading config.
			"internal/extensions/orchestrator/catalog/manifest.go",
		},
		hint: "load config through application.UseConfig[T] / AppUseConfig[T], " +
			"or let PublishService[Svc, Cfg] hand it to the factory — " +
			"config.DecodeBytes / DecodeMap are what apply expansion and the " +
			"envkey/default/required tags",
	},
	{
		// LoadInto reads a named config file directly. That is a CLI
		// affordance — a command loading a file the user named — and never
		// how a service or extension obtains its settings.
		literal: "LoadInto(",
		exempt: []string{
			// Where it is defined and used by the config module itself.
			"internal/core/config",
		},
		hint: "extension and service code resolves config through " +
			"application.UseConfig[T] (runtime) or AppUseConfig[T] (bootstrap); " +
			"LoadInto belongs in a CLI command",
	},
	{
		// The address a task's published schema answers on. It was built by
		// string concatenation in the producer AND, separately, in ktw-gui —
		// two writers of one fact with nothing to notice them disagreeing.
		// When they did, the GUI asked for a URI nobody published and
		// rendered no run form, reporting nothing.
		literal: ":/data-type/task/",
		exempt: []string{
			// The one writer. This is what the literal is forbidden in
			// favour of.
			"internal/extensions/manifest/api/task_schema_uri.go",
		},
		hint: "build it with mbapi.TaskSchemaURI(provider, task, role), or name " +
			"the contribution with mbapi.TaskSchemaID(task, role) (c27)",
	},
}

func main() {
	violations := 0
	checked := 0

	// Walk the project's Go sources.
	roots := []string{"app", "internal", "tools"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Skip vendor, _archive, generated, hidden.
				name := d.Name()
				if name == "vendor" || name == "_archive" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
					return filepath.SkipDir
				}
				return nil
			}
			// Lint only Go source files.
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			// Skip the lint tool itself.
			if strings.Contains(path, "tools/uri-discipline") || strings.Contains(path, "tools\\uri-discipline") {
				return nil
			}
			checked++
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			slashed := filepath.ToSlash(path)
			lines := strings.Split(string(data), "\n")
			for _, r := range rules {
				if exempted(slashed, r.exempt) {
					continue
				}
				if !companionPresent(lines, r.withAny) {
					continue
				}
				for i, line := range lines {
					// A comment naming the forbidden form is documentation,
					// not a call. Flagging it would make the guard fire on the
					// text that explains it.
					if t := strings.TrimSpace(line); strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") {
						continue
					}
					if strings.Contains(line, r.literal) {
						fmt.Fprintf(os.Stderr, "VIOLATION: %s:%d: contains forbidden literal %q\n    %s\n",
							path, i+1, r.literal, r.hint)
						violations++
					}
				}
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "walk %s: %v\n", root, err)
			os.Exit(2)
		}
	}

	if violations > 0 {
		// No trailing advice here: each violation already printed the hint for
		// its own rule. A generic footer would name the wrong fix for three of
		// the four rules.
		fmt.Fprintf(os.Stderr, "\nuri-discipline: %d violation(s) across %d files\n", violations, checked)
		os.Exit(1)
	}
	fmt.Printf("uri-discipline: OK (%d rules, %d files checked)\n", len(rules), checked)
}

// exempted reports whether a path is one of a rule's declared exemptions.
func exempted(path string, exempt []string) bool {
	for _, e := range exempt {
		if strings.Contains(path, e) {
			return true
		}
	}
	return false
}

// companionPresent reports whether the file contains any of `withAny`, ignoring
// comments. An empty `withAny` means the rule applies unconditionally.
func companionPresent(lines []string, withAny []string) bool {
	if len(withAny) == 0 {
		return true
	}
	for _, line := range lines {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") {
			continue
		}
		for _, w := range withAny {
			if strings.Contains(line, w) {
				return true
			}
		}
	}
	return false
}
