// SPDX-License-Identifier: Apache-2.0

package api

// ProjectsFolderOutput is the installation's projects folder: where every
// project of the person lives, chosen at first start, never inside the
// installation.
type ProjectsFolderOutput struct {
	Folder       string `json:"folder" desc:"The projects folder: the one chosen, else the one proposed"`
	Chosen       bool   `json:"chosen" desc:"The person chose it; false: it is only proposed"`
	Proposed     string `json:"proposed" desc:"The folder proposed at first start: <Documents>/Katyoti"`
	Installation string `json:"installation,omitempty" desc:"The installation's folder; empty for a development build (no installation: nothing can be set)"`
}

// SetProjectsFolderInput chooses the installation's projects folder.
type SetProjectsFolderInput struct {
	Folder string `json:"folder" desc:"A full path outside the installation's folder and its roots; created when missing" required:"true"`
}
