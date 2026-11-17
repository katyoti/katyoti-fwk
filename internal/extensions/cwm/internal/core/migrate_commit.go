// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	stdctx "context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A PROJECT'S CONVERSION AS ONE COMMIT (: migration is
// one committed act by one person). The migrate dry run says where git stands
// on the files the conversion writes; MigrateCommit converts and commits those
// files alone, so a person converts a project from the GUI without a terminal.

// touchedPaths are the files a conversion writes, project-relative: the
// objects its steps change or remove and the project object it stamps.
func touchedPaths(out api.MigrateOutput) []string {
	seen := map[string]bool{api.ProjectFileName: true}
	for _, p := range groupMergeTouched(out.GroupMerge) {
		seen[p] = true
	}
	for _, st := range out.Steps {
		for _, p := range append(slices.Clone(st.Changed), st.Removed...) {
			seen[p] = true
		}
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	return paths
}

// migrateTree is where git stands on paths (project-relative): each one that
// differs from the last commit - modified, staged, deleted or untracked - is
// named. A project in a sub-folder of its repository is read as it is: git
// runs in the project root and its answers are brought back to it.
func (s *Service) migrateTree(paths []string) api.MigrateTree {
	root := s.Config.ObjectsRoot()
	if !inGitWorkTree(root) {
		return api.MigrateTree{}
	}
	prefixOut, err := gitcli.Run(root, "rev-parse", "--show-prefix")
	if err != nil {
		return api.MigrateTree{}
	}
	prefix := strings.TrimSpace(prefixOut) // the project root, from the repository's top, "" or ending in /
	tree := api.MigrateTree{Repo: true}
	dirty := map[string]bool{}
	for start := 0; start < len(paths); start += dirtyChunk {
		chunk := paths[start:min(start+dirtyChunk, len(paths))]
		args := append([]string{"status", "--porcelain", "-z", "--untracked-files=all", "--"}, chunk...)
		out, err := gitcli.Run(root, args...)
		if err != nil { // an unreadable status is no proof of a commit
			for _, p := range chunk {
				dirty[p] = true
			}
			continue
		}
		entries := gitcli.Paths(out)
		for i := 0; i < len(entries); i++ {
			e := entries[i]
			if len(e) < 4 {
				continue
			}
			if e[0] == 'R' || e[0] == 'C' {
				i++ // a rename or copy is followed by the path it came from
			}
			p := strings.TrimPrefix(e[3:], prefix)
			dirty[p] = true
		}
	}
	for p := range dirty {
		tree.TouchedDirty = append(tree.TouchedDirty, p)
	}
	slices.Sort(tree.TouchedDirty)
	tree.Clean = len(tree.TouchedDirty) == 0
	return tree
}

// MigrateCommit converts the project's data and commits the files the
// conversion wrote as one commit. Refused, writing nothing, when nothing
// waits or a file it writes is not as the last commit has it. Each step, the
// index rebuilt and the commit are reported into the panel the run lends it
// (progress.Emit - a no-op without one). Only the conversion's paths are
// committed (`git commit --only`, the paths on stdin): another change staged
// in the index stays out of it. Outside a repository, or when a hook refuses
// the commit, the data is converted and the answer says why no commit was
// made.
func (s *Service) MigrateCommit(ctx stdctx.Context, in api.MigrateCommitInput) (api.MigrateCommitOutput, error) {
	if s.refused != nil {
		return api.MigrateCommitOutput{}, s.refused
	}
	dry, err := s.Migrate(ctx, in.MigrateInput(true))
	if err != nil {
		return api.MigrateCommitOutput{MigrateOutput: dry}, err
	}
	if dry.From == dry.To && dry.GroupMerge == nil {
		return api.MigrateCommitOutput{MigrateOutput: dry}, fmt.Errorf("%w: MigrateCommit: the project's data is at version %d already - nothing waits for a conversion", api.ErrInvalidInput, dry.To)
	}
	if dry.Tree != nil && dry.Tree.Repo && !dry.Tree.Clean {
		return api.MigrateCommitOutput{MigrateOutput: dry}, fmt.Errorf("%w: MigrateCommit: files the conversion writes differ from the last commit - commit or discard them first: %s",
			api.ErrConflict, strings.Join(dry.Tree.TouchedDirty, ", "))
	}
	total := int64(dry.To-dry.From) + 2 // each step, the index, the commit
	var done int64
	report := func(label string) {
		done++
		progress.Emit(ctx, panel.Progress{Label: label, Current: done, Total: total})
	}
	out, err := s.migrate(ctx, in.MigrateInput(false), report)
	res := api.MigrateCommitOutput{MigrateOutput: out}
	if err != nil {
		return res, err
	}
	res.Commit = s.commitConversion(out)
	if res.Commit.Sha != "" {
		report("committed " + shortSha(res.Commit.Sha))
	} else {
		report("not committed: " + res.Commit.Reason)
	}
	return res, nil
}

// commitConversion commits the files out wrote, and only them.
func (s *Service) commitConversion(out api.MigrateOutput) api.MigrateCommit {
	root := s.Config.ObjectsRoot()
	if !inGitWorkTree(root) {
		return api.MigrateCommit{Reason: "the project is in no git repository: converted, nothing committed"}
	}
	var paths bytes.Buffer
	for _, p := range touchedPaths(out) {
		paths.WriteString(p)
		paths.WriteByte(0)
	}
	msg := fmt.Sprintf("Convert project data from version %d to %d", out.From, out.To)
	if out.GroupMerge != nil {
		// The modules a join moved are new paths: staged, with what left
		// them, so the one commit holds the move.
		if _, err := gitcli.RunInput(root, paths.Bytes(), "add", "-A", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
			return api.MigrateCommit{Reason: commitRefusal(err)}
		}
		msg = fmt.Sprintf("Join the project group %s into one project; convert its data from version %d to %d", out.GroupMerge.Group, out.From, out.To)
	}
	if _, err := gitcli.RunInput(root, paths.Bytes(), "commit", "--only", "-m", msg, "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
		return api.MigrateCommit{Reason: commitRefusal(err)}
	}
	head, err := gitcli.Run(root, "rev-parse", "HEAD")
	if err != nil {
		return api.MigrateCommit{Reason: err.Error()}
	}
	return api.MigrateCommit{Sha: strings.TrimSpace(head)}
}

// lineEndingWarning is git's note on a file whose line endings it will
// change (core.autocrlf): noise beside why a commit was refused.
var lineEndingWarning = regexp.MustCompile(`warning: (in the working copy of '[^']*', )?(CR)?LF will be replaced by (CR)?LF[^\n]*|The file will have its original line endings in your working directory\.?`)

// commitRefusal is why git refused the commit - a hook's words, say - without
// git's line-ending warnings.
func commitRefusal(err error) string {
	var kept []string
	for _, line := range strings.Split(lineEndingWarning.ReplaceAllString(err.Error(), ""), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func shortSha(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}
