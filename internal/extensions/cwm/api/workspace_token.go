// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
)

// THE WORKSPACE SLOT — which project an object event comes from.
//
// Every CWM subject carries `workspace.<ws>` (subjects.go). Two repositories
// served in one universe can each hold a module `inbox`: their objects' tokens
// collide, so the slot is what tells their events apart. It carries the
// project's id - `project.uuid`, minted once by ProjectInit and committed with
// the repository, stable across a rename of the project - never its name.
//
// Publisher and subscriber each derive it from the project with the one
// function below and never exchange it (rule, as for the object
// token): the publisher is the store serving the project, the subscriber
// anything that wants that one project's events.

// UnboundWorkspace is the slot of a store whose project has no id yet (no
// project object, or one before ProjectInit minted its uuid).
const UnboundWorkspace = "unbootstrapped"

// WorkspaceToken is the workspace slot of a project's events: its uuid,
// lowercased; UnboundWorkspace when it has none or it is no valid subject
// token.
func WorkspaceToken(projectUUID string) string {
	tok := strings.ToLower(strings.TrimSpace(projectUUID))
	if !cwmTokenPattern.MatchString(tok) {
		return UnboundWorkspace
	}
	return tok
}

// ProjectWorkspaceToken is the workspace slot of the project at projectRoot,
// read from its project object (`project.cwm.yaml`); UnboundWorkspace when
// there is none or it names no id.
func ProjectWorkspaceToken(projectRoot string) string {
	if projectRoot == "" {
		return UnboundWorkspace
	}
	f, err := projectfile.Read(projectRoot)
	if err != nil {
		return UnboundWorkspace
	}
	return WorkspaceToken(f.UUID)
}

// ProjectEventScope is the scope of the object events of the project at
// projectRoot served in the universe at universeHome: tenant and universe as
// every publisher resolves them (cenv.ResolveTenantUniverse), the workspace
// slot from the project's id (ProjectWorkspaceToken). A subscriber of one
// project's events builds its pattern from it (ProjectEventsPattern).
func ProjectEventScope(universeHome, projectRoot string) (tenantID, universeID, workspaceID string) {
	tenantID, universeID = cenv.ResolveTenantUniverse(universeHome)
	return tenantID, universeID, ProjectWorkspaceToken(projectRoot)
}

// ProjectEventsPattern is `ktw.<t>.universe.<u>.workspace.<ws>.>`: every
// event of one project.
func ProjectEventsPattern(tenantID, universeID, workspaceID string) string {
	return workspaceSubjectPrefix(tenantID, universeID, workspaceID) + ".>"
}

// UniverseWorkspacesPattern is `ktw.<t>.universe.<u>.workspace.>`: every
// event of every project the universe serves.
func UniverseWorkspacesPattern(tenantID, universeID string) string {
	cwmMustToken("tenantID", tenantID)
	cwmMustToken("universeID", universeID)
	return "ktw." + tenantID + ".universe." + universeID + ".workspace.>"
}
