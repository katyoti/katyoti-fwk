// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	personasapi "github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

//go:embed recipes/*.yaml
var recipeFS embed.FS

var (
	recipesOnce sync.Once
	recipesList []api.Recipe
	recipesErr  error
)

// loadRecipes reads and checks the recipes: a known kind, unique step names,
// a step waiting only on the steps before it.
func loadRecipes() ([]api.Recipe, error) {
	recipesOnce.Do(func() {
		recipesList, recipesErr = readRecipes(recipeFS, "recipes")
	})
	return recipesList, recipesErr
}

func readRecipes(fsys fs.FS, dir string) ([]api.Recipe, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []api.Recipe
	for _, e := range entries {
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var r api.Recipe
		if err := config.DecodeBytes(raw, marshal.YAML, &r); err != nil {
			return nil, fmt.Errorf("recipe %s: %w", e.Name(), err)
		}
		if err := checkRecipe(r); err != nil {
			return nil, fmt.Errorf("recipe %s: %w", e.Name(), err)
		}
		out = append(out, r)
	}
	sortRecipes(out)
	return out, nil
}

// sortRecipes puts the most specific first: types and states, types, states,
// the kind alone; then by name.
func sortRecipes(out []api.Recipe) {
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := specificity(out[i]), specificity(out[j]); a != b {
			return a > b
		}
		return out[i].Name < out[j].Name
	})
}

func specificity(r api.Recipe) int {
	n := 0
	if len(r.Types) > 0 {
		n += 2
	}
	if len(r.States) > 0 {
		n++
	}
	if len(r.Resolves) > 0 {
		n += 2
	}
	return n
}

func checkRecipe(r api.Recipe) error {
	if r.Name == "" {
		return fmt.Errorf("a recipe needs a name")
	}
	if len(r.Types) == 0 && !slices.Contains(api.Kinds, r.Kind) {
		return fmt.Errorf("a recipe names object types or a kind among %v (got %q)", api.Kinds, r.Kind)
	}
	if r.Kind != "" && !slices.Contains(api.Kinds, r.Kind) {
		return fmt.Errorf("kind %q is not among %v", r.Kind, api.Kinds)
	}
	if len(r.Steps) == 0 {
		return fmt.Errorf("no steps")
	}
	if r.Depth != "" && !slices.Contains(api.Depths, r.Depth) {
		return fmt.Errorf("depth %q is not one of %v", r.Depth, api.Depths)
	}
	for _, st := range r.Steps {
		if st.Isolation != "" && !slices.Contains(api.Isolations, st.Isolation) {
			return fmt.Errorf("step %s: isolation %q does not exist yet (only %v)", st.Name, st.Isolation, api.Isolations)
		}
		if st.Name == api.StepIntake {
			return fmt.Errorf("step %q is reserved: the planning loop gives every new request its intake job", api.StepIntake)
		}
	}
	seen := map[string]bool{}
	share := 0.0
	for _, st := range r.Steps {
		if st.Name == "" || seen[st.Name] {
			return fmt.Errorf("step %q: a step has a name, unique in its recipe", st.Name)
		}
		if st.Class != "" && st.Class != api.ClassRefine && st.Class != api.ClassDeliver {
			return fmt.Errorf("step %s: class %q is not refine or deliver", st.Name, st.Class)
		}
		switch st.Actor {
		case "", api.ActorAgent:
			if st.PersonWhenAsked {
				if st.Interaction != api.InteractionValidate && st.Interaction != api.InteractionChallenge {
					return fmt.Errorf("step %s: a review a person does when asked asks validate or challenge (got %q)", st.Name, st.Interaction)
				}
			} else if st.Interaction != "" && st.Interaction != api.InteractionValidate && st.Interaction != api.InteractionChallenge {
				// An agent's review answers a verdict (validate or challenge);
				// deciding and providing are a person's.
				return fmt.Errorf("step %s: an agent's step asks at most a verdict, validate or challenge (got %q)", st.Name, st.Interaction)
			}
			if st.Role == "" {
				return fmt.Errorf("step %s: an agent's step names its role (a task or a persona)", st.Name)
			}
		case api.ActorPerson:
			if st.PersonWhenAsked {
				return fmt.Errorf("step %s: a person's step is always a person's: person_when_asked is an agent's review", st.Name)
			}
			if !slices.Contains([]string{api.InteractionValidate, api.InteractionChallenge, api.InteractionDecide, api.InteractionProvide}, st.Interaction) {
				return fmt.Errorf("step %s: a person's step asks validate or challenge or decide or provide (got %q)", st.Name, st.Interaction)
			}
		default:
			return fmt.Errorf("step %s: actor %q is not agent or person", st.Name, st.Actor)
		}
		if st.Skill != "" {
			if u, err := contribapi.ParseResourceURI(st.Skill); err != nil || u.Type != "skill" {
				return fmt.Errorf("step %s: skill %q is not a skill URI (<provider>:/skill/<name>)", st.Name, st.Skill)
			}
		}
		if st.Phase != "" && api.PhaseIndex(st.Phase) < 0 {
			return fmt.Errorf("step %s: phase %q is not one of %v", st.Name, st.Phase, api.Phases)
		}
		for _, a := range st.After {
			if !seen[a] {
				return fmt.Errorf("step %s waits on %q, which is not a step before it", st.Name, a)
			}
		}
		seen[st.Name] = true
		share += st.Share
	}
	if share > 1+1e-9 {
		return fmt.Errorf("its shares add up to %g, more than 1", share)
	}
	return nil
}

// recipeFor is the recipe for a request: the most specific whose types
// name its type and whose states (when it names some) its state — the stage to
// reach — and otherwise the one for its kind (build when unset).
func recipeFor(o cwmapi.Object) (api.Recipe, error) {
	recipes, err := loadRecipes()
	if err != nil {
		return api.Recipe{}, err
	}
	return selectRecipe(recipes, o)
}

// ListRecipes answers the recipes.
func (p *Planning) ListRecipes(context.Context, api.ListAllInput) (api.ListRecipesOutput, error) {
	recipes, err := loadRecipes()
	return api.ListRecipesOutput{Recipes: recipes}, err
}

// recipeAt is the recipe a step names (<recipe>/<step>) and the step's index
// in it — or, for no step, o's recipe and -1.
func recipeAt(o cwmapi.Object, at string) (api.Recipe, int, error) {
	name, step, ok := strings.Cut(at, "/")
	if !ok {
		r, err := recipeFor(o)
		return r, -1, err
	}
	recipes, err := loadRecipes()
	if err != nil {
		return api.Recipe{}, 0, err
	}
	for _, r := range recipes {
		if r.Name != name {
			continue
		}
		for i, st := range r.Steps {
			if st.Name == step {
				return r, i, nil
			}
		}
	}
	return api.Recipe{}, 0, invalid("no recipe step %q", at)
}

// resolveStep makes a recipe step a plan step on o: a person's step asks its
// interaction; an agent's role is a persona when the catalog has one, else a
// task. Refine work reads o's module, deliver work writes it.
func resolveStep(ctx context.Context, o cwmapi.Object, r api.Recipe, st api.RecipeStep) (api.PlanStep, error) {
	module, err := cwmapi.ModuleOfObjectID(o.ID)
	if err != nil {
		return api.PlanStep{}, err
	}
	s := api.PlanStep{Step: r.Name + "/" + st.Name, Phase: st.Phase, Group: api.PlanGroup, Gate: st.Gate}
	switch {
	case st.Actor == api.ActorPerson, st.PersonWhenAsked && personReviewAsked(ctx, o):
		s.Doer, s.Interaction = api.DoerPerson, st.Interaction
	case personaFor(ctx, st.Role):
		s.Doer, s.Task = api.DoerPersona, st.Role
	default:
		s.Doer, s.Task = api.DoerTask, st.Role
	}
	if s.Doer != api.DoerPerson {
		// An agent's review - one a person does when asked, or always an
		// agent's (the decide recipe's challenge) - answers the interaction's
		// verdict, as a person's.
		s.Interaction = st.Interaction
	}
	if s.Phase == "" {
		switch {
		case st.Class == api.ClassRefine:
			s.Phase = api.PhasePreparation
		case s.Doer == api.DoerPerson:
			s.Phase = api.PhaseVerify
		default:
			s.Phase = api.PhaseBuild
		}
	}
	switch {
	case s.Doer == api.DoerPerson:
	case st.Class == api.ClassRefine, st.PersonWhenAsked:
		s.Reads = []string{module}
	default:
		s.Writes = []string{module}
	}
	return s, nil
}

// personReviewAsked is whether a person reviews o's work: a deliverable only a
// person can test (by_hand) asks it; else the request's own review setting,
// else its release's, else its release's stream's - an agent when none says
func personReviewAsked(ctx context.Context, o cwmapi.Object) bool {
	for _, d := range fieldsOf(o).Deliverables {
		if d.ByHand != "" && !d.Verified {
			return true
		}
	}
	if r := fieldsOf(o).Review; r != "" {
		return r == api.ReviewPerson
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return false
	}
	_, rel := releaseOfWork(ctx, cwm, o)
	if rel == "" {
		return false
	}
	ro, _, err := loadAny(ctx, cwm, rel)
	if err != nil {
		return false
	}
	if r := releaseFields(ro).Review; r != "" {
		return r == api.ReviewPerson
	}
	if sm := edgeTo(ro, api.EdgeInStream); sm != "" {
		if so, err := loadStream(ctx, cwm, sm); err == nil {
			return streamFields(so).Review == api.ReviewPerson
		}
	}
	return false
}

// reresolveReviews gives a request's reviews not started yet the doer its
// review setting now asks: a review a person does when asked becomes a
// person's, or an agent's again.
func reresolveReviews(ctx context.Context, o cwmapi.Object) error {
	todos, err := todosOf(ctx, o.ID)
	if err != nil {
		return err
	}
	for _, x := range todos {
		if !notStarted(x) || x.Packet.Template == "" {
			continue
		}
		r, i, err := recipeAt(o, x.Packet.Template+"/"+strings.TrimSuffix(x.Packet.Step, reworkSuffix))
		if err != nil || i < 0 || !r.Steps[i].PersonWhenAsked {
			continue
		}
		s, err := resolveStep(ctx, o, r, r.Steps[i])
		if err != nil {
			return err
		}
		if s.Doer == x.Packet.Doer {
			continue
		}
		x.Packet.Doer, x.Packet.Task, x.Packet.Interaction = s.Doer, s.Task, s.Interaction
		x.For, x.Start, x.Reads, x.Writes = forOf(s.Doer), startOf(s.Doer), s.Reads, s.Writes
		if _, err := put(ctx, x); err != nil {
			return err
		}
	}
	return nil
}

// personaFor is whether a role names a catalog persona (a role is a persona's
// name); false when there is none, or no persona library.
func personaFor(ctx context.Context, role string) bool {
	if role == "" {
		return false
	}
	lib, err := application.UseService[personasapi.PersonaService](ctx)
	if err != nil || lib == nil {
		return false
	}
	_, err = lib.Resolve(ctx, personasapi.ResolveInput{Name: role})
	return err == nil
}
