// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// A FETCHED EXPORT WAITS ON THE PERSON - a clone's fetch
// brings another clone's export ref (refs/katyoti/scheduler/*, F-1306): when
// its tip holds commits this store never wrote nor imported, « À faire »
// shows one item, and its button runs `todos import` - only on the person's
// word, never by itself. What this store has seen is a local ref beside the
// export's, outside its refspec so never pushed nor fetched:
// refs/katyoti/scheduler-seen/<uuid>, moved by each export it writes and each
// import. An export that would write over a tip it has not seen (the
// automatic export after a fetch, a rejected push rewritten over the
// remote's tip) keeps that tip first in refs/katyoti/scheduler-fetched/<uuid>:
// the item stays, and the import reads it before the tip, so nothing fetched
// is buried. A repository with no seen mark whose rows the store holds took
// its ref from an export written before the mark existed: seen.

// The local refs keeping what this store has seen and what waits on import.
const (
	seenRefPrefix    = "refs/katyoti/scheduler-seen/"
	fetchedRefPrefix = "refs/katyoti/scheduler-fetched/"
)

func seenRef(repository string) string    { return seenRefPrefix + repository }
func fetchedRef(repository string) string { return fetchedRefPrefix + repository }

// isAncestor says commit a is b or one of b's ancestors.
func isAncestor(dir, a, b string) bool {
	if a == b {
		return true
	}
	_, err := gitcli.Run(dir, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// unseenExport is the export commit of the repository this store has not
// seen - a kept fetched tip first, else the ref's tip -, "" when there is
// none. hasRows says the store holds rows of the repository.
func unseenExport(dir, repository string, hasRows bool) (string, error) {
	kept, err := refTip(dir, fetchedRef(repository))
	if err != nil || kept != "" {
		return kept, err
	}
	tip, err := refTip(dir, ExportRef(repository))
	if err != nil || tip == "" {
		return "", err
	}
	seen, err := refTip(dir, seenRef(repository))
	if err != nil {
		return "", err
	}
	if seen == "" {
		if hasRows {
			return "", nil // an export written before the mark existed
		}
		return tip, nil
	}
	if isAncestor(dir, tip, seen) {
		return "", nil
	}
	return tip, nil
}

// keepUnseen keeps the ref's tip in the fetched ref when the store has not
// seen it, before an export writes over it.
func keepUnseen(dir, repository string) error {
	unseen, err := unseenExport(dir, repository, true)
	if err != nil || unseen == "" {
		return err
	}
	_, err = gitcli.Run(dir, "update-ref", "-m", "scheduler: a fetched export waits on import", fetchedRef(repository), unseen)
	return err
}

// markSeen records commit as seen by this store; done, the kept fetched tip
// is dropped too.
func markSeen(dir, repository, commit string, imported bool) error {
	if commit == "" {
		return nil
	}
	if _, err := gitcli.Run(dir, "update-ref", "-m", "scheduler: seen", seenRef(repository), commit); err != nil {
		return err
	}
	if imported {
		if _, err := gitcli.Run(dir, "update-ref", "-d", fetchedRef(repository)); err != nil {
			return err
		}
	}
	return nil
}

// FetchedWaitingSourceName is the fetched export's source in « À faire ».
const FetchedWaitingSourceName = "scheduler-export"

// importURI is the act the item's button runs.
const importURI = "ktw-agents:todos/import"

// FetchedImportButton is the element id of the item's button (the GUI walk
// finds it by id).
const FetchedImportButton = "scheduler-export-import"

// fetchedExports are the repositories whose fetched export the store has not
// seen, by uuid, with their directory.
func fetchedExports(ctx context.Context, s *Service) (map[string]string, error) {
	d, err := dumpOf(s)
	if err != nil {
		return nil, nil // the file store: its to-dos are their committed files
	}
	roots, err := repositoryRoots(ctx)
	if err != nil {
		return nil, err
	}
	held, err := d.Repositories(ctx)
	if err != nil {
		return nil, err
	}
	has := map[string]bool{}
	for _, r := range held {
		has[r] = true
	}
	out := map[string]string{}
	for repo, dir := range roots {
		if dir == "" {
			continue
		}
		unseen, err := unseenExport(dir, repo, has[repo])
		if err != nil {
			return nil, fmt.Errorf("the export ref of %s in %s: %w", repo, dir, err)
		}
		if unseen != "" {
			out[repo] = dir
		}
	}
	return out, nil
}

// FetchedWaitingSource is the fetched export's contribution to « À faire ».
func FetchedWaitingSource() inboxapi.WaitingSource {
	return fetchedWaitingSource(ServiceIn)
}

func fetchedWaitingSource(service func(context.Context) (*Service, error)) inboxapi.WaitingSource {
	return inboxapi.WaitingSource{
		Name: FetchedWaitingSourceName,
		Items: func(ctx context.Context) ([]inboxapi.WaitingItem, error) {
			s, err := service(ctx)
			if err != nil {
				return nil, nil // no scheduler in this process: nothing to import
			}
			got, err := fetchedExports(ctx, s)
			if err != nil || len(got) == 0 {
				return nil, err
			}
			return []inboxapi.WaitingItem{fetchedItem(got)}, nil
		},
		Controls: fetchedControls,
	}
}

// fetchedItem is the one item: whatever the number of projects, one import
// brings them all.
func fetchedItem(got map[string]string) inboxapi.WaitingItem {
	repos := make([]string, 0, len(got))
	for r := range got {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	dirs := make([]string, 0, len(repos))
	for _, r := range repos {
		dirs = append(dirs, got[r])
	}
	return inboxapi.WaitingItem{
		Key:   FetchedWaitingSourceName + ":fetched",
		Kind:  inboxapi.KindApprove,
		Title: "Import the fetched to-dos",
		Asks: fmt.Sprintf("A fetch brought a scheduler export newer than this store (%s). "+
			"Import it to bring its to-dos here; to-dos the store already holds stay as they are. Nothing is imported without you.",
			strings.Join(dirs, ", ")),
		Module: "scheduler", About: "scheduler", AnswerableBy: "person",
		Ref: strings.Join(repos, ","),
	}
}

// fetchedControls is the item's button: import, on the person's word.
func fetchedControls(_ context.Context, it inboxapi.WaitingItem, lang string) []render.Node {
	if it.Key != FetchedWaitingSourceName+":fetched" {
		return nil
	}
	label := "Import"
	if lang == "fr" {
		label = "Importer"
	}
	return []render.Node{{"kind": "button", "id": FetchedImportButton, "label": label, "icon": "download",
		"size": "xs", "variant": "light", "action": render.RunURI(label, importURI, map[string]any{})}}
}
