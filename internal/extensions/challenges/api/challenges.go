// SPDX-License-Identifier: Apache-2.0

// Package api is the challenges service contract: the doubts aimed at an
// object, kept alive until they are answered. A challenge is an edge of kind
// `challenges` from the challenger to what it disputes, wherever a pack
// declares that kind. The service reads through the CWM Object API and keeps
// nothing of its own.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// EdgeChallenges is the edge kind from a challenger to what it challenges.
const EdgeChallenges = "challenges"

// ChallengesServiceDef describes the service for KTSP/CLI auto-generation.
var ChallengesServiceDef = meta.ServiceDef{
	Name:    "challenges",
	Short:   "The doubts aimed at an object: what challenges it, kept in view until answered",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Challengers": {Short: "What challenges an object (its incoming challenges edges) as the page its Challenged by tab shows: each challenger with its type, its state and where it sits", ReadOnly: true},
	},
}

// ChallengesService answers the challenges of an object.
type ChallengesService interface {
	// Challengers is the Challenged by panel's source.
	Challengers(ctx context.Context, in ObjectInput) (render.Def, error)
}

// ObjectInput names the object a panel is shown on.
type ObjectInput struct {
	ID       string `json:"id"                 jsonschema:"Object id; required"`
	Language string `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
}
