// SPDX-License-Identifier: Apache-2.0

package api

// MigrateInput brings the project's data to the binary's data version.
type MigrateInput struct {
	DryRun bool `json:"dry_run,omitempty" jsonschema:"List every object each step would change and write nothing"`
	// GroupType is the type a project group's root takes once its embedded
	// children are joined into it (GroupMerge): the children's one type when
	// they share it, else software-project, when empty.
	GroupType string `json:"group_type,omitempty" jsonschema:"The type a project group's root takes once its children's modules are joined into it - the children's one type when they share it or else software-project when empty"`
	// GroupCapabilities are field-groups the joined project takes - the
	// identity its type needs (a dossier's code) - set over its own.
	GroupCapabilities map[string]map[string]any `json:"group_capabilities,omitempty" jsonschema:"Field-groups by capability id the joined project takes - set over its own"`
	// GroupRenames keeps both modules of a collision: a child's module, by
	// its folder (project-relative), takes the new name as it joins - its
	// folder, its objects' ids and every reference its own project made to
	// them follow.
	GroupRenames map[string]string `json:"group_renames,omitempty" jsonschema:"A child's module renamed as the group is joined - by its folder (project-relative: projects/<child>/modules/<module>) to its new name; its folder and its objects' ids follow and so does every parent and edge its own project named them by"`
}

// GroupMerge is what joining a project group into one project did, or would
// do: every module of each embedded child moves
// into the root's modules folder as it is, the child project objects go, and
// the root drops its project-group capability. A module name two of them hold
// is a collision: nothing is written while one stands.
type GroupMerge struct {
	Group       string `json:"group"                  jsonschema:"The group's root project id - the one project left"`
	Type        string `json:"type"                   jsonschema:"The type the project takes"`
	DataVersion int    `json:"data_version"           jsonschema:"The lowest data version of the root and its children - the steps run from it"`
	// Before and After count the modules: the root's and every child's, then
	// the joined project's.
	Before     int               `json:"modules_before"         jsonschema:"Modules before: the root's and every child's"`
	After      int               `json:"modules_after"          jsonschema:"Modules of the joined project"`
	Children   []string          `json:"children"               jsonschema:"The children joined by folder"`
	Moved      []ModuleMove      `json:"moved,omitempty"        jsonschema:"The modules moved into the root's modules folder"`
	Joined     []ModuleMove      `json:"joined,omitempty"       jsonschema:"The system modules a child and the root both hold - joined file by file"`
	Collisions []ModuleCollision `json:"collisions,omitempty"   jsonschema:"What keeps the join from being made - nothing is written while one stands"`
	Retargeted []string          `json:"retargeted,omitempty"   jsonschema:"Objects whose parent or edge named a child project - now the joined project - or a renamed module's objects - and theirs whose id was renamed - by project-relative path"`
	Dropped    []DroppedProject  `json:"dropped,omitempty"      jsonschema:"The child project objects removed - with the field-groups they carried besides project"`
	Links      []ProjectLink     `json:"links,omitempty"        jsonschema:"Linked children the group named - repositories the person attaches on their own"`
	// KeptAside are a child's machine-local folders (.katyoti: its index, its
	// local queues) moved under the root's, never committed nor lost.
	KeptAside []ModuleMove `json:"kept_aside,omitempty" jsonschema:"A child's machine-local state folder moved under the root's own - never committed and never lost"`
}

// ProjectLink is a linked child an old project group named: its name and the
// repository it is cloned from - reported by the join for the person to link
// to the universe as a project of its own.
type ProjectLink struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
}

// ModuleMove is one module moved (or joined) from a child.
type ModuleMove struct {
	Module string `json:"module"`
	// RenamedFrom is the module's name in its child when GroupRenames gave
	// it another one.
	RenamedFrom string `json:"renamed_from,omitempty" jsonschema:"The module's name in its child - set when the join renamed it"`
	From        string `json:"from"            jsonschema:"Its folder before - project-relative"`
	To          string `json:"to"              jsonschema:"Its folder after - project-relative"`
	Files       int    `json:"files"           jsonschema:"The files it holds (moved)"`
	// Dropped are a joined system module's files the root holds already: the
	// same bytes, or the module's own object, the root's kept.
	Dropped []string `json:"dropped,omitempty" jsonschema:"Files of a joined system module the root holds already - the same bytes or the module's own object - by path in the module"`
}

// ModuleCollision is a module name (or a file of a joined system module) two
// projects of the group hold.
type ModuleCollision struct {
	Module string   `json:"module"`
	Paths  []string `json:"paths" jsonschema:"Where each one is - project-relative"`
	Why    string   `json:"why"`
}

// DroppedProject is a child project object the join removes.
type DroppedProject struct {
	ID           string                    `json:"id"`
	Type         string                    `json:"type"`
	Dir          string                    `json:"dir"`
	Capabilities map[string]map[string]any `json:"capabilities,omitempty" jsonschema:"The field-groups it carried besides project - a dossier's identity - for the person to keep where they choose"`
}

// MigrationStepReport is what one step changed, or would change.
type MigrationStepReport struct {
	From    int      `json:"from"              jsonschema:"The data version the step starts from"`
	To      int      `json:"to"                jsonschema:"The data version it brings the data to"`
	Title   string   `json:"title"             jsonschema:"What the step changes"`
	Changed []string `json:"changed,omitempty" jsonschema:"The objects it changed (or would change) by project-relative path"`
	Removed []string `json:"removed,omitempty" jsonschema:"The objects it removed (or would remove) - merged into another - by project-relative path"`
	Lines   []string `json:"lines,omitempty"   jsonschema:"What a step on the project's files did (or would do) - one thing per line"`
}

// MigrateOutput says where the data was, where it is and what each step did.
type MigrateOutput struct {
	From   int                   `json:"from"             jsonschema:"The project's data version before"`
	To     int                   `json:"to"               jsonschema:"This binary's data version: the project's after"`
	DryRun bool                  `json:"dry_run,omitempty"`
	Steps  []MigrationStepReport `json:"steps,omitempty"  jsonschema:"Each step run in order"`
	// Unreadable are the files no step could read: left as they are.
	Unreadable []string `json:"unreadable,omitempty" jsonschema:"Object files that could not be read and were left unchanged"`
	// Tree is where git stands on the files the conversion writes: set by a
	// dry run that has something to do.
	Tree *MigrateTree `json:"tree,omitempty" jsonschema:"Where git stands on the files the conversion writes - set by a dry run with something to do"`
	// GroupMerge is set when the project was a project group: its children
	// joined into it before the steps ran.
	GroupMerge *GroupMerge `json:"group_merge,omitempty" jsonschema:"The project group's children joined into one project - set when the project was a group"`
}

// MigrateTree is where git stands on the files a conversion writes: the
// objects it changes or removes and the project object it stamps.
type MigrateTree struct {
	Repo         bool     `json:"repo"                    jsonschema:"The project is in a git repository"`
	Clean        bool     `json:"clean"                   jsonschema:"Every file the conversion writes is as the last commit has it - false outside a repository"`
	TouchedDirty []string `json:"touched_dirty,omitempty" jsonschema:"The files the conversion writes that differ from the last commit - modified or staged or deleted or untracked - by project-relative path"`
}

// MigrateCommitInput converts the project's data and commits the conversion.
type MigrateCommitInput struct {
	// The group options of MigrateInput, for a project group joined as it
	// is converted.
	GroupType         string                    `json:"group_type,omitempty" jsonschema:"As for migrate: the type a project group's root takes once joined"`
	GroupCapabilities map[string]map[string]any `json:"group_capabilities,omitempty" jsonschema:"As for migrate: field-groups by capability id the joined project takes"`
	GroupRenames      map[string]string         `json:"group_renames,omitempty" jsonschema:"As for migrate: a child's module renamed as the group is joined - by its folder to its new name"`
}

// MigrateInput is the migrate input carrying the commit's group options.
func (in MigrateCommitInput) MigrateInput(dryRun bool) MigrateInput {
	return MigrateInput{DryRun: dryRun, GroupType: in.GroupType, GroupCapabilities: in.GroupCapabilities, GroupRenames: in.GroupRenames}
}

// MigrateCommitOutput is the conversion and its commit.
type MigrateCommitOutput struct {
	MigrateOutput
	Commit MigrateCommit `json:"commit" jsonschema:"The conversion's commit - its sha or why there is none"`
}

// MigrateCommit is a conversion's commit: its sha, or why none was made (no
// repository, a hook refusing it) - the data is converted either way.
type MigrateCommit struct {
	Sha    string `json:"sha,omitempty"    jsonschema:"The commit holding the conversion"`
	Reason string `json:"reason,omitempty" jsonschema:"Why no commit was made - the data is converted all the same"`
}
