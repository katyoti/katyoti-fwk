// SPDX-License-Identifier: Apache-2.0

package api

// EarlierProject is a project an earlier installation of this machine serves:
// a folder one of its universes attaches.
type EarlierProject struct {
	Folder       string `json:"folder" desc:"The project's folder, in the earlier installation's world: read, never written"`
	Name         string `json:"name" desc:"The name it takes in the projects folder (its folder's name)"`
	Installation string `json:"installation" desc:"The earlier installation's name"`
	Universe     string `json:"universe" desc:"The universe of that installation that serves it"`
	DataVersion  int    `json:"data_version" desc:"The version of its data's shape: an import migrates the copy to this release's"`
	ImportedTo   string `json:"imported_to,omitempty" desc:"Where it was imported already: its name is taken in the projects folder"`
	LastUsed     string `json:"last_used,omitempty" desc:"When KaTyoti last changed it (RFC 3339): the newest change in its store (.katyoti), else its folder's"`
}

// EarlierProjectsOutput lists the projects of the earlier installations.
type EarlierProjectsOutput struct {
	Projects []EarlierProject `json:"projects"`
	// Skipped are the installations or universes that could not be read, each
	// with why: the others are still listed.
	Skipped []string `json:"skipped,omitempty" desc:"Installations or universes not read, each with why"`
}

// ImportProjectInput imports an earlier project as a copy.
type ImportProjectInput struct {
	Folder   string `json:"folder" desc:"The earlier project's folder (one EarlierProjects lists)" required:"true"`
	Name     string `json:"name,omitempty" desc:"Its name in the projects folder; default its folder's name"`
	Universe string `json:"universe,omitempty" desc:"A universe of this installation to attach the copy to; none: the copy is only placed"`
}

// ImportProjectOutput says where the copy is and how its data moved.
type ImportProjectOutput struct {
	Project  string `json:"project" desc:"The copy, in the projects folder"`
	From     int    `json:"from" desc:"The data version of the original (and of the copy before)"`
	To       int    `json:"to" desc:"The copy's data version after the migration"`
	Universe string `json:"universe,omitempty" desc:"The universe the copy is attached to"`
}

// EarlierInstallation is one installation of this machine (katyoti-instances.yaml):
// what it holds and whether it can be deleted.
type EarlierInstallation struct {
	Name     string `json:"name" desc:"The name it is registered under"`
	Title    string `json:"title" desc:"Its own name (its katyoti.yml), what its shortcuts and window show"`
	Folder   string `json:"folder" desc:"Its folder"`
	Version  string `json:"version,omitempty" desc:"Its control plane's version, when recorded"`
	Bytes    int64  `json:"bytes" desc:"What its folders hold, in bytes"`
	LastUsed string `json:"last_used,omitempty" desc:"When a file of it last changed (RFC 3339)"`
	Current  bool   `json:"current,omitempty" desc:"It is the installation answering: never deleted from here"`
	Reason   string `json:"reason,omitempty" desc:"Why it cannot be deleted, for a screen to say in its language: current, last, projects-inside or shared" enum:"current,last,projects-inside,shared"`
	Refused  string `json:"refused,omitempty" desc:"Why it cannot be deleted, the sentence"`
}

// EarlierInstallationsOutput lists the installations of this machine.
type EarlierInstallationsOutput struct {
	Installations []EarlierInstallation `json:"installations"`
}

// DeleteInstallationInput names the installation to delete and, once the
// person acknowledged what goes, the token of that plan.
type DeleteInstallationInput struct {
	Name       string `json:"name" desc:"The installation's registered name" required:"true"`
	Ack        string `json:"ack,omitempty" desc:"The token of the plan the person acknowledged; none: only the plan is answered and nothing is touched"`
	DeleteLast bool   `json:"delete_last,omitempty" desc:"Delete it even when it is the last installation"`
}

// InstallationFolder is one folder a deletion removes, and what it holds.
type InstallationFolder struct {
	Path     string `json:"path" desc:"The folder"`
	Bytes    int64  `json:"bytes" desc:"What it holds, in bytes"`
	LastUsed string `json:"last_used,omitempty" desc:"When a file in it last changed (RFC 3339)"`
}

// DeleteInstallationOutput is what deleting the installation takes away - the
// plan to acknowledge - or that it was deleted.
type DeleteInstallationOutput struct {
	Name      string               `json:"name" desc:"The installation"`
	Folders   []InstallationFolder `json:"folders" desc:"Every folder that goes, with its size and last use"`
	Shortcuts string               `json:"shortcuts" desc:"The installation name its shortcuts carry (« KaTyoti <name> »)"`
	Token     string               `json:"token" desc:"The token that acknowledges this plan"`
	Deleted   bool                 `json:"deleted,omitempty" desc:"It was deleted (an ack was given and matched)"`
}
