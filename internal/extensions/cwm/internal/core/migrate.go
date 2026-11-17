// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// MIGRATIONS: a project's data carries its data version
// (`project.data_version` on the project object); this binary's data version
// is the baseline plus the steps it ships. A step moves the data one version
// up; `cwm migrate` runs the steps from the project's version to the
// binary's, then stamps the new version on the project object.

// MigrationStep moves a project's data from one data version to the next.
// A step once shipped - even in a development release - is never rewritten:
// a later change is a new step. A step is idempotent: applied to an object it
// already changed, it changes nothing.
type MigrationStep struct {
	// Pack names the CWM pack whose shape the step follows - the sidecar
	// (`app/<pack>/embed/cwm`) or the extension
	// (`internal/extensions/<pack>/contrib/embed/cwm`, e.g. planification)
	// that declares the types and capabilities it changes. A step changing
	// two packs' shapes names the first. TestEveryMigrationStepNamesAKnownPack
	// refuses a step with no pack or a pack nothing ships.
	Pack string
	// Title says what the step changes.
	Title string
	// Object changes one object in place and answers whether it changed it.
	Object func(o *api.Object) (bool, error)
	// Corpus, when set, sees every object at once - a step merging objects
	// into one: it answers the indexes it changed and those it removed (their
	// files deleted). Object is not called then.
	Corpus func(objs []*api.Object) (changed, removed []int, err error)
	// Project, when set, changes the project's files beyond its objects (a
	// tree retiring into objects, migrate_project.go): it says what it would
	// do, and its Apply runs once the changed objects are written, before
	// the data version is stamped. Object and Corpus are not called then.
	Project func(ctx stdctx.Context, pm ProjectMigration) (ProjectChange, error)
	// slot, set on a private slot only, is the data version it moves to.
	slot int
}

// migrationSteps are the steps this binary ships, in order: the first moves
// the baseline (projectfile.BaselineDataVersion) to the next version. Append
// only.
var migrationSteps = []MigrationStep{
	// 1 → 2: a change entry may carry a label (: 1.001-9
	// a change listed again) and a capability is added
	// by a sidecar pack. Both only add an optional field or a new
	// capability: no stored object is rewritten.
	{Pack: "planification", Title: "change entries may carry a label; a pack capability is added (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 2 → 3: a source-document's section_path becomes optional - a whole
	// document a source's sync anchors has none. Every
	// stored source-document stays valid: nothing is rewritten.
	{Pack: "sources", Title: "a source-document's section path is optional (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 3 → 4: a check may declare its own time limit:
	// a checkable's and a deliverable's optional timeout, the verdict's
	// optional timeout it ran on. Only optional fields: nothing is rewritten.
	{Pack: "ktw-methods", Title: "a checkable, a deliverable and a verdict may carry a timeout (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 4 → 5: generated documents: the knowledge capability
	// gets its fields (include_generated, cap-knowledge), an information
	// source may override it, a source-document records whether it was
	// generated and by what. Only optional fields: nothing is rewritten.
	{Pack: "sources", Title: "the knowledge capability, an information source and a source-document may say whether generated documents come in or were (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 5 → 6: who reviews a build's work: a request, a
	// release and a stream may say review: agent or person. Only optional
	// fields - an agent reviews when none says: nothing is rewritten.
	{Pack: "planification", Title: "a request, a release and a stream may say who reviews the work - an agent or a person (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 6 → 7: a release's candidate records its build, its gates' results, its
	// alpha sign-off and whether it was promoted. Only
	// optional fields: nothing is rewritten.
	{Pack: "planification", Title: "a release's candidate may record its build, its gates, its sign-off and its promotion (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 7 → 8: a candidate keeps its letter, its « Quoi de neuf »
	// . An optional field: nothing is rewritten.
	{Pack: "planification", Title: "a release's candidate may keep its letter (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 8 → 9: the old plan-step model is removed: a
	// plan no longer carries its next step, when it was planned or its run -
	// a request's steps are its to-dos - and running is no longer a plan
	// state (read from the to-dos); a stored running plan is ready. The
	// unused `delivery` and `approval` workflows of ktw-methods are removed:
	// no type used them, no object is rewritten for them.
	{Pack: "planification", Title: "a plan no longer carries next, planned_at or run, and running is no longer a plan state; the delivery and approval workflows are removed",
		Object: dropPlanStep},
	// 9 → 10: an information source may hold a choice not yet read
	// . An optional field: nothing is rewritten.
	{Pack: "sources", Title: "an information source may hold a choice not yet read (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 10 → 11: an information source records where each item applied
	// stands. An optional field: nothing is rewritten.
	{Pack: "sources", Title: "an information source records where each item applied stands (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 11 → 12: an object holds one source per connector among its own sources
	// : the sources of one connector under one carrier merge
	// into the first (by id), its selection the union of theirs, the
	// documents anchored under the others moved under it; the others are
	// removed.
	{Pack: "sources", Title: "a carrier's sources of one connector merge into one, their selections kept",
		Corpus: mergeSourcesPerConnector},
	// 12 → 13: a generated document is read like any other, marked « IA »
	// : the include_generated setting of an information source
	// and of the knowledge capability is gone.
	{Pack: "sources", Title: "an information source and the knowledge capability no longer carry include_generated",
		Object: dropIncludeGenerated},
	// 13 → 14: one Confluence connector: a
	// source of confluence-page or confluence-space reads through confluence,
	// its location become its selection - the page, or space:<KEY> -, then a
	// carrier's Confluence sources merge into one as 11 → 12 does. A space
	// narrowed by a title query keeps its location, a source of its own.
	{Pack: "sources", Title: "the confluence-page and confluence-space sources read through the one confluence connector",
		Corpus: oneConfluenceConnector},
	// 14 → 15: an increment not done by its release's date moves on
	// : every stream carries the daily release-due timer
	// that moves it, and an increment's release fields may say why it moved.
	{Pack: "planification", Title: "every stream carries its daily release-due timer",
		Object: streamDueTimer},
	// 15 -> 16: code is a capability of its own: a module's kind
	// code packages, tiers and facets move from its module group to its code
	// group; a kind that is no code kind (platform, methodology) goes.
	{Pack: "ktw-code", Title: "a module's code - kind, packages, tiers, facets - moves to the code capability",
		Object: moveCodeFields},
	// 16 -> 17: every module takes its type (the object-levels
	// study's migration): a module with code packages, or a sidecar, is a
	// software module; any other a documentation module. A module of a domain
	// (an aspect of its own) is its domain's to convert; the project waits for
	// its type. Ids, folders, edges and data are kept.
	{Pack: "cwm", Title: "every module takes its type: software-module with code, documentation without",
		Object: typeModules},
	// 17 -> 18: a module carrying an aspect takes its type too:
	// version 17 left it to its domain, but an aspect such as the method's isa
	// is no domain standing in for a type. The aspects stay, so a domain's own
	// step still finds its modules by them.
	{Pack: "cwm", Title: "a module carrying an aspect takes its type too, its aspects kept",
		Object: typeAspectModules},
	// 18 -> 19: a project of the bare project type is a software project, the
	// default project type; a project of another type stays.
	{Pack: "cwm", Title: "a project of the bare project type is a software project",
		Object: typeSoftwareProject},
	// 19 -> 20: a step of a pack this build does not carry (migrate_private.go).
	privateSlot(20),
	// 20 -> 21: a project may be a project group: the
	// project-group type and capability, whose one optional field names the
	// folder of its embedded child projects. The version also takes the
	// project type of a sidecar pack, declared without a
	// step of its own. No project is a group yet: nothing is rewritten.
	{Pack: "cwm", Title: "a project may be a project group, holding child projects (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 21 -> 22: a project group may link children in their own repository
	// : an optional list of links, each a name and the repository
	// it is cloned from. Nothing is rewritten.
	{Pack: "cwm", Title: "a project group may link child projects in their own repository (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 22 -> 23: a demand's delta operation may carry a role - display-switch,
	// the feature that puts an increment's new screens behind its switch
	// . Optional: nothing is rewritten.
	{Pack: "planification", Title: "a delta operation may carry a role (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 23 -> 24: a source's selected item may say who kept it and when
	// , and a held item carries them too. Optional: nothing
	// is rewritten.
	{Pack: "sources", Title: "a source's item may say who kept it and when (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 24 -> 25: a step of a pack this build does not carry (migrate_private.go).
	privateSlot(25),
	// 25 -> 26: a step of a pack this build does not carry (migrate_private.go).
	privateSlot(26),
	// 26 -> 27: a step of a pack this build does not carry (migrate_private.go).
	privateSlot(27),
	// 27 -> 28: a step of a pack this build does not carry (migrate_private.go).
	privateSlot(28),
	// 28 -> 29: a documentation module carries no code: version
	// 17 typed a module without code packages documentation but kept its
	// empty code group, so the code capability - and module-health, the code
	// checks attached to it - stayed on modules that hold no code. An empty
	// code group (no package, no kind, no tier, no facet) goes; one holding
	// any of them is left for a person to look at.
	{Pack: "cwm", Title: "a documentation module drops its empty code group",
		Object: documentationDropsEmptyCode},
	// 29 -> 30: questions asked of a person: the
	// holding-list type (who may answer a question) and the answer type (an
	// answer kept on what it is about) are added. New types only: no stored
	// object is rewritten.
	{Pack: "inbox", Title: "the holding-list and answer types are added (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 30 -> 31: whose a decision is: the decision
	// capability may carry its owner (person or agent) and why an agent may
	// take it. Optional fields, an unset owner reads the person's: no stored
	// decision is rewritten.
	{Pack: "ktw-methods", Title: "a decision may say whose it is (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 31 -> 32: what a synced item brings: a source-document
	// may name its item's kind, its data file and the data-type shaping it,
	// and its text file. Optional fields, written by the next sync: no stored
	// anchor is rewritten. (The shape recorded with it also holds the module
	// object type ktw-methods retired, - no object of it.)
	{Pack: "sources", Title: "a source-document may name its item's kind, data, data-type and text (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 32 -> 33: the bare project type is retired: step 18 -> 19
	// typed every bare project software-project; older data still holding
	// one is read in its legacy shape (migrate_legacy.go).
	{Pack: "cwm", Title: "the bare project type is retired (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 33 -> 34: an Inbox notice: an approval-request's expects
	// may be notice - an information, read and dismissed. A value added to an
	// enum: no stored request is rewritten. (The shape recorded with it also
	// holds type-pane 2.0.0,: a pane declaration drops its live
	// source - declared by types, never stored on an object.)
	{Pack: "inbox", Title: "an approval-request may expect a notice (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 34 -> 35: an increment's delivery review: the
	// release field-group may carry delivery_review and test_delivery, both
	// optional. No stored increment is rewritten.
	{Pack: "planification", Title: "an increment may carry its delivery review and its test delivery (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 35 -> 36: an object records its minter: the number of an
	// object created in a git repository comes from its minter's number
	// block, and the object keeps who minted it. Optional: an object minted
	// before blocks carries none and stays valid - nothing is rewritten.
	{Pack: "cwm", Title: "an object may record its minter (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 36 -> 37: the reversed planning edge (F-107, rule
	// ): a module object - a goal, a feature, a use case
	// a decision, a rule - carries no edge to an increment or a release. Its
	// targets edge goes, and the increment or release it named takes it (a
	// takes edge on the holder, once). A request keeps its targets edge.
	{Pack: "planification", Title: "a module object's targets edge becomes a takes edge on the increment or release it named",
		Corpus: targetsBecomeTakes},
	// 37 -> 38: a source-document may record the key of the item picked a
	// sync brought it for: a source's detail matches its
	// items to their documents by it. Optional, written by the next sync: no
	// stored anchor is rewritten.
	{Pack: "sources", Title: "a source-document may record the key of the item it was brought for (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 38 -> 39: a plan's step record may say who added the step beyond its
	// request's recipe, why and its generation.
	// Optional, written as a step is collected: no stored record is rewritten.
	{Pack: "planification", Title: "a plan's step record may say who added it beyond the recipe, why and its generation (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 39 -> 40: a demand's increment-create operation names the stream the
	// increment is created in - an increment is always created in its stream
	// . Optional in the shape: no stored delta is
	// rewritten (an applied one is history; a pending one is checked again).
	{Pack: "planification", Title: "a delta operation may name the stream a created increment is in (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 40 -> 41: project groups are retired (:
	// a project never holds projects): no pack ships the project-group type,
	// its capability or its data-type. A repository whose root is still a
	// project group is joined into one project before the steps run
	// (project_group_merge.go) - its children's modules in the root's flat
	// modules folder, ids and edges kept; until then it opens through the
	// legacy reading (migrate_legacy.go). Nothing else is rewritten.
	{Pack: "cwm", Title: "project groups are retired: a project group is joined into one project (nothing else to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 41 -> 42: a blocked plan says why it is blocked by a kind
	// : its readers choose its actions by it, never by
	// its note's words. A plan blocked before has its kind read once from the
	// note its blocker wrote.
	{Pack: "planification", Title: "a blocked plan says why it is blocked: end-refused, move-refused, hand-test-failed, parked or manual",
		Object: planBlockKind},
	// 42 -> 43: a step of a pack this build does not carry (migrate_private.go).
	privateSlot(43),
	// 43 -> 44: a step of a pack this build does not carry (migrate_private.go).
	privateSlot(44),
	// 44 -> 45: a type's pane says whether it draws the standard object
	// header (pane.header) - optional, declared by types,
	// never stored on an object: nothing is rewritten.
	{Pack: "objectpane", Title: "a type's pane may say whether it draws the object header (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 45 -> 46: a step of a pack this build does not carry (migrate_private.go).
	privateSlot(46),
	// 46 -> 47: the publication becomes one version-neutral type of the
	// publications pack: it names what it publishes - a
	// management view or a document-tree source object - and its target - a
	// publication connector or a view's converter. A planification
	// publication's converter, stored as `target`, is kept as `converter`.
	{Pack: "publications", Title: "a publication names its view's converter as converter, beside an object and a connector",
		Object: publicationTargetToConverter},
	// 47 -> 48: any object may carry the contributes capability
	// : the contribution it authors - kind, id, layers
	// overrides - read into the project's contributions. A new capability: no
	// stored object is rewritten.
	{Pack: "projectcontributions", Title: "any object may carry the contributes capability (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 48 -> 49: a project's overwrites/<provider>/ trees retire
	// (decision superseding):
	// each becomes override objects in sys-contributions, the
	// layers it served their files, each widening reported; the tree is
	// backed up, the copy checked, then removed. A file of any tree mapping to
	// no layer stops the step, nothing written.
	{Pack: "projectcontributions", Title: "the project's overwrites trees become override objects and are removed",
		Project: retireOverwritesTrees},
	// 49 -> 50: a source's held item may name the item it sits under
	// : a tree picker nests the selection by it. An
	// optional field: no stored object is rewritten.
	{Pack: "sources", Title: "a source's held item may name the item it sits under (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 50 -> 51: an information source keeps its own « Réglages »
	// : its keywords, its proposal bound, when its connection
	// was tested and whether it answered, and the re-reading mode
	// daily-on-open. Optional fields and a new enum value: nothing is
	// rewritten.
	{Pack: "sources", Title: "an information source may keep its own keywords, proposal bound and connection test, and be read again daily and on open (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 51 -> 52: an information source reading from another source keeps
	// the values that source kept at its last save: its
	// take tab marks a value kept since. An optional field: nothing is
	// rewritten.
	{Pack: "sources", Title: "an information source may keep the values the source it reads from kept at its last save (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
	// 52 -> 53: an information source may choose, per item kind, the
	// attribute its « Contenu lu » table leads with. An
	// optional field: nothing is rewritten.
	{Pack: "sources", Title: "an information source may choose the id column of each kind of item it reads (nothing to rewrite)",
		Object: func(*api.Object) (bool, error) { return false, nil }},
}

// publicationTargetToConverter moves a publication's converter action URI
// from `target` to `converter`; a converter already
// named is kept, the stale `target` dropped.
func publicationTargetToConverter(o *api.Object) (bool, error) {
	if o.Type != "publication" {
		return false, nil
	}
	pub, ok := o.Capabilities["publication"]
	if !ok {
		return false, nil
	}
	target, has := pub["target"]
	if !has {
		return false, nil
	}
	if c, _ := pub["converter"].(string); c == "" {
		pub["converter"] = target
	}
	delete(pub, "target")
	return true, nil
}

// planBlockKind gives a blocked plan the kind of its block, read from the
// note its blocker wrote - the one read of those words, once: a refused end
// (« its to-dos are done, but … »; its own node unable to become done, a
// move refused), a failed hand test, a send-back parked past the review cap;
// any other note is a block by hand. A plan not blocked, or already
// carrying its kind, is left as it is.
func planBlockKind(o *api.Object) (bool, error) {
	plan, ok := o.Capabilities["plan"]
	if !ok {
		return false, nil
	}
	if st, _ := plan["state"].(string); st != "blocked" {
		return false, nil
	}
	if k, _ := plan["block"].(string); k != "" {
		return false, nil
	}
	note, _ := plan["note"].(string)
	kind := "manual"
	switch {
	case strings.HasPrefix(note, "its to-dos are done, but "+o.ID+" cannot become done"):
		kind = "move-refused"
	case strings.HasPrefix(note, "its to-dos are done"), strings.Contains(note, "is not verified"):
		kind = "end-refused"
	case strings.HasPrefix(note, "a hand test failed: "):
		kind = "hand-test-failed"
	case strings.Contains(note, " past the cap of "):
		kind = "parked"
	}
	plan["block"] = kind
	return true, nil
}

// targetsBecomeTakes turns every targets edge of an object that is not a
// request into a takes edge on the increment or release it names
// : the holder takes it once, the object's edge goes.
// Idempotent: a holder already taking it is not written again. A targets
// edge naming nothing stored is left in place - the planification guard
// names it for a person.
func targetsBecomeTakes(objs []*api.Object) (changed, removed []int, err error) {
	byID := map[string]int{}
	for i, o := range objs {
		if o.ID != "" {
			byID[o.ID] = i
		}
	}
	touched := map[int]bool{}
	for i, o := range objs {
		if o.Type == "request" {
			continue
		}
		kept := o.Edges[:0:0]
		moved := false
		for _, e := range o.Edges {
			h, ok := byID[e.To]
			if e.Kind != "targets" || !ok || !holdsWork(objs[h].Type) {
				kept = append(kept, e)
				continue
			}
			holder := objs[h]
			if !slices.ContainsFunc(holder.Edges, func(x api.ObjectEdge) bool { return x.Kind == "takes" && x.To == o.ID }) {
				holder.Edges = append(holder.Edges, api.ObjectEdge{Kind: "takes", To: o.ID})
				touched[h] = true
			}
			moved = true
		}
		if moved {
			o.Edges = kept
			touched[i] = true
		}
	}
	for i := range objs {
		if touched[i] {
			changed = append(changed, i)
		}
	}
	return changed, nil, nil
}

// holdsWork: an increment or a release takes module work.
func holdsWork(typ string) bool { return typ == "increment" || typ == "release" }

// documentationDropsEmptyCode drops an empty code group from a documentation
// module.
func documentationDropsEmptyCode(o *api.Object) (bool, error) {
	if o.Type != "documentation" {
		return false, nil
	}
	code, ok := o.Capabilities["code"]
	if !ok {
		return false, nil
	}
	for _, v := range code {
		if !emptyValue(v) {
			return false, nil
		}
	}
	delete(o.Capabilities, "code")
	return true, nil
}

// emptyValue: nil, an empty string, list or map.
func emptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

// typeSoftwareProject types a bare project as a software project.
func typeSoftwareProject(o *api.Object) (bool, error) {
	if o.Type != api.CapProject {
		return false, nil
	}
	o.Type = api.DefaultProjectType
	return true, nil
}

// typeAspectModules types the modules version 17 left for carrying an aspect.
func typeAspectModules(o *api.Object) (bool, error) {
	if len(o.Aspects) == 0 {
		return false, nil
	}
	return typeModuleByCode(o)
}

// typeModules gives a module of the bare module type (or ktw-code's former
// code-module) its type.
func typeModules(o *api.Object) (bool, error) {
	if o.Type != "module" && o.Type != "code-module" {
		return false, nil
	}
	if len(o.Aspects) > 0 {
		return false, nil // a domain's module: converted by its domain
	}
	return typeModuleByCode(o)
}

// typeModuleByCode gives a module of the bare type its type by its code: a
// software module with code packages or a sidecar kind, else documentation.
func typeModuleByCode(o *api.Object) (bool, error) {
	if o.Type != "module" && o.Type != "code-module" {
		return false, nil
	}
	code := o.Capabilities["code"]
	pkgs, _ := code["code_packages"].([]any)
	kind, _ := code["kind"].(string)
	if len(pkgs) > 0 || kind == "sidecar" {
		o.Type = "software-module"
	} else {
		o.Type = "documentation"
	}
	return true, nil
}

// codeFields are the module group's fields that are code's.
var codeFields = []string{"code_packages", "tier_current", "tier_target", "tier_state", "facets"}

// codeKinds are the kinds of code - the one list (cap-code).
var codeKinds = map[string]bool{"framework": true, "library": true, "sidecar": true}

// moveCodeFields moves a module's code fields to its code group.
func moveCodeFields(o *api.Object) (bool, error) {
	mod, ok := o.Capabilities["module"]
	if !ok || mod == nil {
		return false, nil
	}
	code := o.Capabilities["code"]
	changed := false
	for _, f := range codeFields {
		v, has := mod[f]
		if !has {
			continue
		}
		if code == nil {
			code = map[string]any{}
		}
		if _, set := code[f]; !set {
			code[f] = v
		}
		delete(mod, f)
		changed = true
	}
	if k, has := mod["kind"]; has {
		if s, _ := k.(string); codeKinds[s] {
			if code == nil {
				code = map[string]any{}
			}
			if _, set := code["kind"]; !set {
				code["kind"] = s
			}
		}
		delete(mod, "kind")
		changed = true
	}
	if !changed {
		return false, nil
	}
	if code != nil {
		o.Capabilities["code"] = code
	}
	if len(mod) == 0 {
		delete(o.Capabilities, "module")
	}
	return true, nil
}

// streamDueTimer gives a stream the daily release-due timer it now carries
func streamDueTimer(o *api.Object) (bool, error) {
	if o.Type != "stream" {
		return false, nil
	}
	timed := o.Capabilities["timed"]
	var timers []any
	if timed != nil {
		timers, _ = timed["timers"].([]any)
	}
	for _, t := range timers {
		if m, ok := t.(map[string]any); ok && m["name"] == "release-due" {
			return false, nil
		}
	}
	if o.Capabilities == nil {
		o.Capabilities = map[string]map[string]any{}
	}
	if timed == nil {
		timed = map[string]any{}
	}
	timed["timers"] = append(timers, map[string]any{"name": "release-due", "at": "06:00"})
	o.Capabilities["timed"] = timed
	return true, nil
}

// oneConfluenceConnector moves the sources of the two retired Confluence
// connectors onto confluence.
func oneConfluenceConnector(objs []*api.Object) (changed, removed []int, err error) {
	var narrowed []int // a space with a query: renamed after the merge
	mark := map[int]bool{}
	for i, o := range objs {
		if o.Type != "information-source" {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal(o.FieldsJSON(), &fields); err != nil || fields == nil {
			continue
		}
		conn, _ := fields["connector"].(string)
		if conn != "confluence-page" && conn != "confluence-space" {
			continue
		}
		loc, _ := fields["location"].(map[string]any)
		page, _ := loc["page"].(string)
		space, _ := loc["space"].(string)
		query, _ := loc["query"].(string)
		if len(selectionItems(fields)) == 0 && strings.TrimSpace(query) != "" && strings.TrimSpace(space) != "" {
			narrowed = append(narrowed, i)
			continue
		}
		if len(selectionItems(fields)) == 0 {
			key := strings.TrimSpace(page)
			if key == "" && strings.TrimSpace(space) != "" {
				key = "space:" + strings.TrimSpace(space)
			}
			if key != "" {
				sel, _ := fields["selection"].(map[string]any)
				if sel == nil {
					sel = map[string]any{}
				}
				sel["items"] = []any{map[string]any{"key": key, "title": o.Name}}
				fields["selection"] = sel
			}
		}
		delete(fields, "location")
		fields["connector"] = "confluence"
		raw, err := json.Marshal(fields)
		if err != nil {
			return nil, nil, err
		}
		o.SetFieldsJSON(raw)
		changed, mark[i] = append(changed, i), true
	}
	merged, removed, err := mergeSourcesPerConnector(objs)
	if err != nil {
		return nil, nil, err
	}
	for _, i := range merged {
		if !mark[i] {
			changed, mark[i] = append(changed, i), true
		}
	}
	for _, i := range narrowed {
		var fields map[string]any
		_ = json.Unmarshal(objs[i].FieldsJSON(), &fields)
		fields["connector"] = "confluence"
		raw, err := json.Marshal(fields)
		if err != nil {
			return nil, nil, err
		}
		objs[i].SetFieldsJSON(raw)
		if !mark[i] {
			changed, mark[i] = append(changed, i), true
		}
	}
	return changed, removed, nil
}

// dropIncludeGenerated removes the include_generated setting of an
// information source and of a carrier's knowledge capability.
func dropIncludeGenerated(o *api.Object) (bool, error) {
	changed := false
	if o.Type == "information-source" {
		var f map[string]any
		if raw := o.FieldsJSON(); len(raw) > 0 && json.Unmarshal(raw, &f) == nil {
			if _, ok := f["include_generated"]; ok {
				delete(f, "include_generated")
				raw, err := json.Marshal(f)
				if err != nil {
					return false, err
				}
				o.SetFieldsJSON(raw)
				changed = true
			}
		}
	}
	if g, ok := o.Capability("knowledge"); ok {
		if _, has := g["include_generated"]; has {
			next := map[string]any{}
			for k, v := range g {
				if k != "include_generated" {
					next[k] = v
				}
			}
			if err := o.SetCapability("knowledge", next); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

// mergeSourcesPerConnector merges a carrier's information sources of one
// connector into the first, by id.
func mergeSourcesPerConnector(objs []*api.Object) (changed, removed []int, err error) {
	type group struct{ idx []int }
	groups := map[string]*group{}
	var order []string
	for i, o := range objs {
		if o.Type != "information-source" {
			continue
		}
		var f struct {
			Connector string `json:"connector"`
		}
		_ = json.Unmarshal(o.FieldsJSON(), &f)
		if f.Connector == "" {
			continue
		}
		k := o.Parent + "|" + f.Connector
		if groups[k] == nil {
			groups[k] = &group{}
			order = append(order, k)
		}
		groups[k].idx = append(groups[k].idx, i)
	}
	moved := map[string]string{} // a removed source's id -> the kept one
	for _, k := range order {
		g := groups[k]
		if len(g.idx) < 2 {
			continue
		}
		sort.Slice(g.idx, func(a, b int) bool { return objs[g.idx[a]].ID < objs[g.idx[b]].ID })
		keep := objs[g.idx[0]]
		var fields map[string]any
		if err := json.Unmarshal(keep.FieldsJSON(), &fields); err != nil || fields == nil {
			fields = map[string]any{}
		}
		items := selectionItems(fields)
		held := listOf(fields["held"])
		seen := map[string]bool{}
		for _, it := range items {
			seen[keyOf(it)] = true
		}
		heldSeen := map[string]bool{}
		for _, it := range held {
			heldSeen[keyOf(it)] = true
		}
		for _, i := range g.idx[1:] {
			var other map[string]any
			_ = json.Unmarshal(objs[i].FieldsJSON(), &other)
			for _, it := range selectionItems(other) {
				if k := keyOf(it); k != "" && !seen[k] {
					seen[k] = true
					items = append(items, it)
				}
			}
			for _, it := range listOf(other["held"]) {
				if k := keyOf(it); k != "" && !heldSeen[k] {
					heldSeen[k] = true
					held = append(held, it)
				}
			}
			moved[objs[i].ID] = keep.ID
			removed = append(removed, i)
		}
		if len(items) > 0 {
			sel, _ := fields["selection"].(map[string]any)
			if sel == nil {
				sel = map[string]any{}
			}
			sel["items"] = items
			fields["selection"] = sel
		}
		if len(held) > 0 {
			fields["held"] = held
		}
		raw, err := json.Marshal(fields)
		if err != nil {
			return nil, nil, err
		}
		keep.SetFieldsJSON(raw)
		changed = append(changed, g.idx[0])
	}
	if len(moved) == 0 {
		return nil, nil, nil
	}
	for i, o := range objs {
		if to, ok := moved[o.Parent]; ok {
			o.Parent = to
			changed = append(changed, i)
		}
	}
	return changed, removed, nil
}

// selectionItems reads a source's selection items, as stored.
func selectionItems(fields map[string]any) []any {
	sel, _ := fields["selection"].(map[string]any)
	return listOf(sel["items"])
}

func listOf(v any) []any {
	l, _ := v.([]any)
	return append([]any(nil), l...)
}

// keyOf is an item's key, as stored.
func keyOf(it any) string {
	m, _ := it.(map[string]any)
	k, _ := m["key"].(string)
	return k
}

// dropPlanStep removes what the old plan-step model wrote on a plan: next,
// planned_at, run; a plan stored running becomes ready.
func dropPlanStep(o *api.Object) (bool, error) {
	plan, ok := o.Capabilities["plan"]
	if !ok {
		return false, nil
	}
	changed := false
	for _, k := range []string{"next", "planned_at", "run"} {
		if _, has := plan[k]; has {
			delete(plan, k)
			changed = true
		}
	}
	if plan["state"] == "running" {
		plan["state"] = "ready"
		changed = true
	}
	return changed, nil
}

// privateSlotTitle is the title of a slot no step of this build fills.
const privateSlotTitle = "a step of a pack this build does not carry (nothing to rewrite)"

// privateSlot keeps the place of the step that moves the data to version to,
// a step of a pack the public build does not carry: the slot rewrites
// nothing, and every later step keeps its data version. A build carrying the
// pack fills the slot with its step (fillPrivateSlots).
func privateSlot(to int) MigrationStep {
	return MigrationStep{Pack: "cwm", Title: privateSlotTitle,
		Object: func(*api.Object) (bool, error) { return false, nil }, slot: to}
}

// fillPrivateSlots puts each step in the slot of the data version it moves
// to; a step with no slot, or a slot left empty, is a programming error.
func fillPrivateSlots(steps map[int]MigrationStep) {
	for to, st := range steps {
		i := to - projectfile.BaselineDataVersion - 1
		if i < 0 || i >= len(migrationSteps) || migrationSteps[i].slot != to {
			panic(fmt.Sprintf("cwm migrate: no private slot for data version %d", to))
		}
		migrationSteps[i] = st
	}
	for _, st := range migrationSteps {
		if st.slot != 0 {
			panic(fmt.Sprintf("cwm migrate: the private slot of data version %d is not filled", st.slot))
		}
	}
}

// dataVersionOf is the data version a binary shipping steps writes.
func dataVersionOf(steps []MigrationStep) int {
	return projectfile.BaselineDataVersion + len(steps)
}

// refuseNewerData refuses a project whose data is newer than the steps know,
// naming both versions and the release to install. The refusal wraps
// api.ErrProjectRefused and reads `project refused: <reason>`; New keeps it on
// the service, which starts and answers it to every read and write of the
// project (Service.refused). A project object that cannot be read is left to
// the index, which reports it on its file.
func refuseNewerData(projectRoot string, steps []MigrationStep) error {
	if projectRoot == "" {
		return nil
	}
	f, err := projectfile.Read(projectRoot)
	if err != nil {
		return nil
	}
	if known := dataVersionOf(steps); f.DataVersion > known {
		return fmt.Errorf("%w: the project's data is at version %d and this binary knows up to version %d - install a newer KaTyoti release, one that knows data version %d, to open it",
			api.ErrProjectRefused, f.DataVersion, known, f.DataVersion)
	}
	return nil
}

// Migrate brings the project's data to this binary's data version: every
// object is read once, each step from the project's version runs over all of
// them in order, and the changed ones are written; the project object is
// stamped with the new version last, so a run cut short runs again. The index
// is rebuilt after. DryRun reports the same and writes nothing.
func (s *Service) Migrate(ctx stdctx.Context, in api.MigrateInput) (api.MigrateOutput, error) {
	if s.refused != nil {
		return api.MigrateOutput{}, s.refused
	}
	out, err := s.migrate(ctx, in, nil)
	if err != nil || !in.DryRun || (out.From == out.To && out.GroupMerge == nil) {
		return out, err
	}
	tree := s.migrateTree(touchedPaths(out))
	out.Tree = &tree
	return out, nil
}

// migrate is Migrate, report - when set - told each step once it ran and the
// index once it is rebuilt.
func (s *Service) migrate(ctx stdctx.Context, in api.MigrateInput, report func(label string)) (api.MigrateOutput, error) {
	if report == nil {
		report = func(string) {}
	}
	root := s.Config.ObjectsRoot()
	if root == "" {
		return api.MigrateOutput{}, fmt.Errorf("%w: project root unset", api.ErrInvalidInput)
	}
	// A project group is joined into one project first (project_group_merge.go):
	// the steps then run over every module it brings, from the lowest data
	// version of the root and its children.
	merge, err := planGroupMerge(root, in)
	if err != nil {
		return api.MigrateOutput{}, err
	}
	if merge != nil && !in.DryRun {
		if err := s.holdsStore(); err != nil {
			return api.MigrateOutput{}, err
		}
		if err := merge.apply(); err != nil {
			return api.MigrateOutput{GroupMerge: &merge.report}, err
		}
		report("project group joined into one project")
	}
	f, err := projectfile.Read(root)
	if err != nil {
		return api.MigrateOutput{}, err
	}
	out := api.MigrateOutput{From: f.DataVersion, To: dataVersionOf(s.steps), DryRun: in.DryRun}
	if merge != nil {
		out.From, out.GroupMerge = merge.report.DataVersion, &merge.report
	}
	switch {
	case out.From > out.To:
		return out, fmt.Errorf("%w: the project's data is at version %d and this binary knows up to %d: install a newer release",
			api.ErrInvalidInput, out.From, out.To)
	case out.From == out.To:
		if merge != nil && !in.DryRun {
			if err := s.rescanOwnWrites(); err != nil {
				return out, err
			}
		}
		return out, nil
	}
	projectPath := filepath.Join(root, api.ProjectFileName)
	if _, err := os.Stat(projectPath); err != nil {
		return out, fmt.Errorf("%w: no %s to record the data version on: run cwm project-init", api.ErrInvalidInput, api.ProjectFileName)
	}

	files, _, err := s.index().objectsGlob()
	if err != nil {
		return out, err
	}
	type loaded struct {
		path    string
		obj     api.Object
		changed bool
		removed bool
	}
	var objs []*loaded
	for _, fl := range files {
		o, err := ReadObjectFile(fl.Path)
		if err != nil {
			out.Unreadable = append(out.Unreadable, s.relPath(fl.Path))
			continue
		}
		objs = append(objs, &loaded{path: fl.Path, obj: o})
	}
	var applies []func(stdctx.Context, api.CWMService) error
	for v := out.From; v < out.To; v++ {
		step := s.steps[v-projectfile.BaselineDataVersion]
		rep := api.MigrationStepReport{From: v, To: v + 1, Title: step.Title}
		if step.Project != nil {
			change, err := step.Project(ctx, ProjectMigration{Root: root, BackupDir: s.Config.BackupDir, Kinds: s.contributionKinds})
			if err != nil {
				return out, fmt.Errorf("migration %d to %d (%s): %w", v, v+1, step.Title, err)
			}
			rep.Changed, rep.Removed, rep.Lines = change.Changed, change.Removed, change.Lines
			if change.Apply != nil {
				applies = append(applies, change.Apply)
			}
			out.Steps = append(out.Steps, rep)
			report(stepLabel(rep))
			continue
		}
		if step.Corpus != nil {
			ptrs := make([]*api.Object, 0, len(objs))
			for _, l := range objs {
				ptrs = append(ptrs, &l.obj)
			}
			changed, removed, err := step.Corpus(ptrs)
			if err != nil {
				return out, fmt.Errorf("migration %d to %d (%s): %w", v, v+1, step.Title, err)
			}
			for _, i := range changed {
				if !objs[i].changed {
					rep.Changed = append(rep.Changed, s.relPath(objs[i].path))
				}
				objs[i].changed = true
			}
			for _, i := range removed {
				objs[i].removed = true
				rep.Removed = append(rep.Removed, s.relPath(objs[i].path))
			}
			out.Steps = append(out.Steps, rep)
			report(stepLabel(rep))
			continue
		}
		for _, l := range objs {
			changed, err := step.Object(&l.obj)
			if err != nil {
				return out, fmt.Errorf("migration %d to %d (%s) on %s: %w", v, v+1, step.Title, s.relPath(l.path), err)
			}
			if changed {
				l.changed = true
				rep.Changed = append(rep.Changed, s.relPath(l.path))
			}
		}
		out.Steps = append(out.Steps, rep)
		report(stepLabel(rep))
	}
	if in.DryRun {
		return out, nil
	}
	if err := s.holdsStore(); err != nil {
		return out, err
	}

	var project *loaded
	for _, l := range objs {
		if l.path == projectPath {
			project = l
			continue
		}
		if l.removed {
			if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
				return out, err
			}
			continue
		}
		if l.changed {
			if _, err := writeObjectFileRev(l.path, l.obj); err != nil {
				return out, err
			}
		}
	}
	if project == nil {
		return out, fmt.Errorf("%w: %s cannot be read: the data version is not recorded", api.ErrInvalidInput, api.ProjectFileName)
	}
	if len(applies) > 0 {
		// The project-level steps write through the service: its index first
		// sees what the object steps wrote.
		if err := s.rescanOwnWrites(); err != nil {
			return out, err
		}
		for _, apply := range applies {
			if err := apply(ctx, s); err != nil {
				return out, err
			}
		}
	}
	group, _ := project.obj.Capability(api.CapProject)
	next := map[string]any{}
	for k, v := range group {
		next[k] = v
	}
	next["data_version"] = out.To
	if err := project.obj.SetCapability(api.CapProject, next); err != nil {
		return out, err
	}
	if _, err := writeObjectFileRev(projectPath, project.obj); err != nil {
		return out, err
	}
	s.dataVersion.Store(0) // the legacy reads end with the conversion (migrate_legacy.go)
	if err := s.rescanOwnWrites(); err != nil {
		return out, err
	}
	report("index rebuilt")
	return out, nil
}

// stepLabel is a migration step as its line in a panel.
func stepLabel(r api.MigrationStepReport) string {
	return fmt.Sprintf("%d to %d: %s (%d changed, %d removed)", r.From, r.To, r.Title, len(r.Changed), len(r.Removed))
}

// holdsObjects says whether the project already holds an object file (the
// project object aside): data written before this binary minted its project.
func (s *Service) holdsObjects() bool {
	files, _, err := s.index().objectsGlob()
	if err != nil {
		return false
	}
	for _, f := range files {
		if f.Module != "" {
			return true
		}
	}
	return false
}

// MigrateFolder brings the project at projectRoot - a folder no hub serves,
// such as a copy being imported - to this binary's data
// version: the project is opened on the scratch workspace, migrated and
// closed. Nothing is written outside projectRoot and scratch.
func MigrateFolder(ctx stdctx.Context, projectRoot, scratch string) (api.MigrateOutput, error) {
	if err := storage.WriteWorkspaceRaw(scratch, []byte("version: \"1\"\n")); err != nil {
		return api.MigrateOutput{}, err
	}
	svc, err := New(ctx, &CWMServiceConfig{WorkspaceRoot: scratch, ProjectRoot: projectRoot, BackupDir: filepath.Join(scratch, "migration-backups")})
	if err != nil {
		return api.MigrateOutput{}, err
	}
	defer svc.Close()
	return svc.Migrate(ctx, api.MigrateInput{})
}
