// SPDX-License-Identifier: Apache-2.0

// CWM subject builders. Every NATS subject CWM publishes goes
// through one of these constructors — never `fmt.Sprintf` inline.
// See modules/bus/nats_subjects_list.md § 8 for the
// canonical catalog.
//
// Subject scope (longer prefix = narrower scope) per
// modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md:
//
//	ktw.<tenant>.universe.<u>.workspace.<ws>.<entity>.<id>.<action>
//
// Token charset: [a-z0-9-]{1,63}. Builders panic on invalid input —
// fmt.Sprintf would silently produce broken subjects otherwise.
package api

import (
	"fmt"
	"regexp"
)

// cwmTokenPattern enforces the same charset as the task-engine
// subject builders. Mirrored here to keep CWM independent of the
// task package.
var cwmTokenPattern = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

func cwmMustToken(name, value string) {
	if !cwmTokenPattern.MatchString(value) {
		panic(fmt.Sprintf("cwm subject builder: invalid %s %q (must match [a-z0-9-]{1,63})", name, value))
	}
}

// workspaceSubjectPrefix is the common `ktw.<t>.universe.<u>.workspace.<ws>`
// portion every CWM event shares. Returned by the helper below
// so each individual builder stays a one-line format.
func workspaceSubjectPrefix(tenantID, universeID, workspaceID string) string {
	cwmMustToken("tenantID", tenantID)
	cwmMustToken("universeID", universeID)
	cwmMustToken("workspaceID", workspaceID)
	return fmt.Sprintf("ktw.%s.universe.%s.workspace.%s",
		tenantID, universeID, workspaceID)
}

// --- Object lifecycle subjects ---------------------------------------------

// ObjectCreatedSubject — `…workspace.<ws>.object.<token>.created`, one per
// object written for the first time; the token is cwmapi.SubjectToken(id).
func ObjectCreatedSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.created",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// ObjectUpdatedSubject — `…workspace.<ws>.object.<token>.updated`, one per
// object rewritten. The inbox handshake waits on it.
func ObjectUpdatedSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.updated",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// ObjectDeletedSubject — `…workspace.<ws>.object.<token>.deleted`.
func ObjectDeletedSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.deleted",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// ObjectMovedSubject — `…workspace.<ws>.object.<token>.moved`, one per
// ObjectMove; the token is the NEW id's (cwmapi.SubjectToken).
func ObjectMovedSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.moved",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// ObjectArchivedSubject — `…workspace.<ws>.object.<token>.archived`, one per
// ObjectArchive that moved a file.
func ObjectArchivedSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.archived",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// ObjectSplitSubject — `…workspace.<ws>.object.<token>.split`, one per
// ObjectSplit; the token is the SOURCE's.
func ObjectSplitSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.split",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// ObjectTransitionRefusedSubject —
// `…workspace.<ws>.object.<token>.transition-refused`, one per move its guard
// refused.
func ObjectTransitionRefusedSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.transition-refused",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// ObjectTransitionedSubject — `…workspace.<ws>.object.<token>.transitioned`,
// one per ObjectTransition.
func ObjectTransitionedSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.transitioned",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// ObjectEventRaisedSubject — `…workspace.<ws>.object.<token>.event`, one per
// EventRaise: an event that is not an object write (a code change), recorded
// on the object it concerns; the token is cwmapi.SubjectToken(id).
func ObjectEventRaisedSubject(tenantID, universeID, workspaceID, objectToken string) string {
	cwmMustToken("objectToken", objectToken)
	return fmt.Sprintf("%s.object.%s.event",
		workspaceSubjectPrefix(tenantID, universeID, workspaceID), objectToken)
}

// --- Project subjects ------------------------------------------------------

// ProjectCaughtUpSubject - `…workspace.<ws>.project.caught-up`, ONE per
// catch-up that found objects moved outside the CWM verbs (a git pull, an
// editor): the ids added, modified and removed, in one grouped event.
func ProjectCaughtUpSubject(tenantID, universeID, workspaceID string) string {
	return workspaceSubjectPrefix(tenantID, universeID, workspaceID) + ".project.caught-up"
}
