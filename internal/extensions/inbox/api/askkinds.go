// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// THE KINDS OF QUESTIONS, READ - which kinds exist, their
// labels and defaults are the `ask-kind` contributions, never a list in code:
// the human task resolves its kind here, and so does an asker that puts a
// human task's to-do itself (planification's person steps).

// QueryAskKinds reads the contributed kinds of questions: the universe's,
// else this binary's own (a test, an offline command). A var: a test sets it.
var QueryAskKinds = func(ctx context.Context) ([]AskKind, error) {
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Kind: ContributionTypeAskKind}
	entries, err := mbapi.QueryTyped[AskKind](ctx, q)
	if err != nil {
		q.Reach = mbapi.ReachSelf
		entries, err = mbapi.QueryTyped[AskKind](ctx, q)
	}
	if err != nil {
		return nil, err
	}
	out := make([]AskKind, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Body)
	}
	return out, nil
}

// AskKinds are the kinds of questions contributed, in the order a list shows
// their counts, a name once.
func AskKinds(ctx context.Context) ([]AskKind, error) {
	all, err := QueryAskKinds(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: the kinds of questions: %w", HumanTaskStrategy, err)
	}
	seen := map[string]bool{}
	out := make([]AskKind, 0, len(all))
	for _, k := range all {
		if k.Name == "" || seen[k.Name] {
			continue
		}
		seen[k.Name] = true
		out = append(out, k)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ResolveAsk refuses a human task whose kind no pack contributes, and gives it
// its kind's label and, where it says nothing, its kind's defaults: who may
// answer, the permission code and what its answer settles.
func ResolveAsk(ctx context.Context, s HumanTask) (HumanTask, error) {
	kinds, err := AskKinds(ctx)
	if err != nil {
		return s, err
	}
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		if k.Name != s.Kind {
			names = append(names, k.Name)
			continue
		}
		s.Label = k.Label
		if s.AnswerableBy == "" {
			s.AnswerableBy = k.AnswerableBy
		}
		if s.Requires == "" {
			s.Requires = k.Requires
		}
		if len(s.Satisfies) == 0 {
			s.Satisfies = append([]string(nil), k.Satisfies...)
		}
		return s, nil
	}
	return s, fmt.Errorf("%w: %s: unknown kind %q (%s)", cwmapi.ErrInvalidInput, HumanTaskStrategy, s.Kind, strings.Join(names, ", "))
}
