// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE PROJECT KEY — one store serves every project of the universe
// : a to-do, its claim and its module locks are keyed by
// (repository, module), the repository the uuid of the project its module
// belongs to (Todo.Repository). Two projects each holding a module of one name
// never share a lock, a queue or a module's mode.
//
// A module is named as the CWM names it: bare, the one the CWM
// resolves (refused once several projects hold it); qualified,
// <key>:<module>, that project's. The store keys a row by the repository the
// reference resolves to and keeps the bare module beside it; a to-do id stays
// <module>.TD-<time>-<token>: the one store mints every id and its primary key
// refuses a second row of one id, so two projects' ids never meet in it.

// splitModule reads a module reference: its project key ("" when bare) and
// its bare module.
func splitModule(ref string) (key, module string, err error) {
	key, module, err = cwmapi.SplitReference(ref)
	if err != nil {
		return "", "", fmt.Errorf("module %q: %w", ref, err)
	}
	return key, module, nil
}

// bareModule is a module reference's module, the reference itself when it
// does not parse (its resolution says why).
func bareModule(ref string) string {
	if _, m, err := splitModule(ref); err == nil {
		return m
	}
	return ref
}

// oneModule refuses a module filter that names no one module (
// rule): a bare name several served projects hold is refused, naming the
// qualified references <key>:<module> to choose from; a qualified one must
// resolve to its project's module. A bare name no project holds is no refusal:
// it names nothing, and the universe's own requests module resolves to no
// object (projectless).
func oneModule(ctx context.Context, module string) error {
	if module == "" {
		return nil
	}
	key, bare, err := splitModule(module)
	if err != nil {
		return err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	_, err = cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: module})
	switch {
	case err == nil:
		return nil
	case key != "":
		return fmt.Errorf("module %s: %w", module, err)
	case !errors.Is(err, cwmapi.ErrInvalidInput):
		return nil
	}
	refs, lerr := modules(ctx)
	if lerr != nil {
		return fmt.Errorf("module %s: %w", module, err)
	}
	var choices []string
	for _, r := range refs {
		if bareModule(r) == bare && r != bare {
			choices = append(choices, r)
		}
	}
	if len(choices) < 2 {
		return fmt.Errorf("module %s: %w", module, err)
	}
	return invalid("module %s is in several projects: name one of %s", module, strings.Join(choices, ", "))
}

// sameModule reports a module reference a filter names, the filter passed by
// oneModule: the reference itself, or the one of its bare module - a bare
// filter names the one project holding it, a qualified one the module of a
// single served project, whose references are bare.
func sameModule(ref, filter string) bool {
	if ref == filter {
		return true
	}
	rk, rm, rerr := cwmapi.SplitReference(ref)
	fk, fm, ferr := cwmapi.SplitReference(filter)
	return rerr == nil && ferr == nil && rm == fm && (rk == "" || fk == "")
}

// modeKey is the key of a module's mode in a queue's read: its repository
// and its module.
func modeKey(repository, module string) string { return repository + "\x00" + module }

// modeOf is the mode of the module a to-do belongs to, in its project.
func modeOf(modes map[string]string, t api.Todo) string {
	return modes[modeKey(t.Repository, t.Module)]
}

// projectUUID is the uuid of a project named by its key or its uuid: the key
// resolves through the CWM to its project object.
func projectUUID(ctx context.Context, project string) (string, error) {
	if looksLikeUUID(project) {
		return project, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return "", err
	}
	got, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: cwmapi.QualifiedID(project, project)})
	if err != nil {
		return "", fmt.Errorf("project %q: %w", project, err)
	}
	if got.ProjectID == "" {
		return "", fmt.Errorf("%w: project %q has no uuid, the key of its to-dos", cwmapi.ErrInvalidInput, project)
	}
	return got.ProjectID, nil
}

// scopeOf is a filter as the queue reads it: its module bare, its project the
// uuid its to-dos carry. A qualified module names its project - resolved as
// the store keys its rows (repositoryOf) - and one that contradicts the
// project named is refused.
func (s *Service) scopeOf(ctx context.Context, f api.Filter) (api.Filter, error) {
	if f.Module != "" {
		key, module, err := splitModule(f.Module)
		if err != nil {
			return f, err
		}
		if key != "" {
			repoOf := s.repoOf
			if repoOf == nil {
				repoOf = repositoryOf
			}
			repo, err := repoOf(ctx, f.Module)
			if err != nil {
				return f, err
			}
			if f.Project != "" {
				named, err := projectUUID(ctx, f.Project)
				if err != nil {
					return f, err
				}
				if named != repo {
					return f, invalid("module %s is not of project %s", f.Module, f.Project)
				}
			}
			f.Module, f.Project = module, repo
			return f, nil
		}
		f.Module = module
	}
	if f.Project != "" {
		uuid, err := projectUUID(ctx, f.Project)
		if err != nil {
			return f, err
		}
		f.Project = uuid
	}
	return f, nil
}

// looksLikeUUID reports a project uuid: the CWM's one parser of a full name
// reads it (cwmapi.ParseFullName), never a copy of its check.
func looksLikeUUID(s string) bool {
	return len(s) == 36 && cwmapi.IsFullName(cwmapi.FullName(s, "m"))
}
