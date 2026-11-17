// SPDX-License-Identifier: Apache-2.0

// Package api is the profile contract: who the person using the platform says
// they are, before (and later bound to) an authenticated identity.
//
// A local profile is a CLAIM, not an identity — nothing authorizes against it.
// Its ID is a stable handle, never the email: it is recorded as the person actor
// on committed CWM history, so it must not change when the email does and must
// not carry personal data. Tenant membership is not a profile field.
package api

import (
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"
)

// Profile sources.
const (
	// SourcePlaceholder: no profile has been set — the platform's John Doe.
	SourcePlaceholder = "placeholder"
	// SourceLocal: typed (or confirmed) by the user on this machine; unverified.
	SourceLocal = "local"
)

// Profile is the person using the platform.
type Profile struct {
	ID        string `json:"id"                  yaml:"id,omitempty"         jsonschema:"description=Stable handle - the person actor id; never the email"`
	FirstName string `json:"firstName,omitempty" yaml:"first_name,omitempty"`
	LastName  string `json:"lastName,omitempty"  yaml:"last_name,omitempty"`
	Email     string `json:"email"               yaml:"email,omitempty"`
	Source    string `json:"source"              yaml:"source,omitempty"     jsonschema:"description=Where the profile comes from,enum=placeholder|local"`
	Language  string `json:"language,omitempty"  yaml:"language,omitempty"   jsonschema:"description=The language the person reads; empty follows the operating system,enum=en|fr"`
}

// DisplayName is "First Last", or the handle when no name is set.
func (p Profile) DisplayName() string {
	if n := strings.TrimSpace(p.FirstName + " " + p.LastName); n != "" {
		return n
	}
	return p.ID
}

// Handle is the person this profile names: its ID, or "" for the placeholder
// (John Doe is nobody - a reader that needs a person refuses it).
func (p Profile) Handle() string {
	if p.Source == SourcePlaceholder {
		return ""
	}
	return p.ID
}

// Placeholder is the profile of a user who has not set one: John Doe, as the
// GUI has always shown.
func Placeholder() Profile {
	return Profile{
		ID:        "john-doe",
		FirstName: "John",
		LastName:  "Doe",
		Email:     "john.doe@example.com",
		Source:    SourcePlaceholder,
	}
}

// UpdateInput sets the profile. Every field is written as given.
type UpdateInput struct {
	ID        string `json:"id"                  jsonschema:"description=Stable handle: lowercase letters digits and hyphens"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	Email     string `json:"email"`
	Language  string `json:"language,omitempty" jsonschema:"description=The language the person reads; empty follows the operating system,enum=en|fr"`
}

// Profile returns the local profile this input describes, trimmed.
func (in UpdateInput) Profile() Profile {
	return Profile{
		ID:        strings.TrimSpace(in.ID),
		FirstName: strings.TrimSpace(in.FirstName),
		LastName:  strings.TrimSpace(in.LastName),
		Email:     strings.TrimSpace(in.Email),
		Source:    SourceLocal,
		Language:  strings.TrimSpace(in.Language),
	}
}

// EmailCandidate is one email the OS knows for the user, with where it came from.
type EmailCandidate struct {
	Email  string `json:"email"`
	Source string `json:"source" jsonschema:"description=Where the address was found e.g. office - upn - git"`
}

// Suggestion is what the machine already knows about the user, to pre-fill a
// profile. It is never saved on its own: the user confirms it through Update.
type Suggestion struct {
	ID        string           `json:"id,omitempty"`
	FirstName string           `json:"firstName,omitempty"`
	LastName  string           `json:"lastName,omitempty"`
	Emails    []EmailCandidate `json:"emails,omitempty" jsonschema:"description=Candidates best first; the user picks one"`
}

// ProfileService holds the profile. It is served by the hub in the admin
// universe (ktw-univers), its only writer; the GUI and the katyoti CLI reach it
// over KTSP.
type ProfileService interface {
	// Get returns the profile, or Placeholder() when none has been set.
	Get(ctx context.Context) (Profile, error)
	// Update validates and saves the profile (source: local).
	Update(ctx context.Context, in UpdateInput) (Profile, error)
	// Suggest reads what the hub's OS user is known as. Empty when the hub has
	// no desktop user to ask.
	Suggest(ctx context.Context) (Suggestion, error)

	// GuideGet returns the guide section: the recipes this person completed.
	GuideGet(ctx context.Context) (GuideState, error)
	// GuideComplete records that a recipe was completed. One entry per recipe:
	// a later completion replaces the earlier one.
	GuideComplete(ctx context.Context, in GuideCompleteInput) (GuideState, error)
	// GuideReset forgets one recipe's completion, or every completion when
	// Recipe is empty.
	GuideReset(ctx context.Context, in GuideResetInput) (GuideState, error)

	// RelayGet returns the relay section: off when never set.
	RelayGet(ctx context.Context) (RelayState, error)
	// RelaySet turns relaying the person's acts on or off; refused when its
	// cause is an agent.
	RelaySet(ctx context.Context, in RelaySetInput) (RelayState, error)
}

var handleRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// reservedPrefix is refused at the start of a handle: consumers that receive a
// bare actor id classify ids starting with it as agents (inbox Answer).
const reservedPrefix = "agent"

// ValidateHandle checks a profile ID: 1–63 lowercase letters, digits and single
// hyphens, not starting with the reserved "agent".
func ValidateHandle(id string) error {
	switch {
	case id == "":
		return fmt.Errorf("id is required")
	case len(id) > 63:
		return fmt.Errorf("id %q is longer than 63 characters", id)
	case !handleRE.MatchString(id):
		return fmt.Errorf("id %q must be lowercase letters, digits and single hyphens (e.g. jdoe, john-doe)", id)
	case strings.HasPrefix(id, reservedPrefix):
		return fmt.Errorf("id %q must not start with %q (reserved for agents)", id, reservedPrefix)
	}
	return nil
}

// ValidateEmail checks a bare address (no display name).
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email is required")
	}
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email {
		return fmt.Errorf("email %q is not a valid address", email)
	}
	return nil
}

// SupportedLanguages lists the accepted values of Profile.Language (besides
// empty, which follows the operating system's locale).
var SupportedLanguages = []string{"en", "fr"}

// ValidateLanguage checks a profile language: empty or one of SupportedLanguages.
func ValidateLanguage(lang string) error {
	if lang == "" || slices.Contains(SupportedLanguages, lang) {
		return nil
	}
	return fmt.Errorf("language %q must be empty or one of %s", lang, strings.Join(SupportedLanguages, ", "))
}

// Validate checks the profile an Update would save.
func (p Profile) Validate() error {
	if err := ValidateHandle(p.ID); err != nil {
		return err
	}
	if err := ValidateEmail(p.Email); err != nil {
		return err
	}
	return ValidateLanguage(p.Language)
}

// How a recipe was completed.
const (
	// GuideChecked: the recipe's check passed on the object it produced.
	GuideChecked = "checked"
	// GuideSeen: the person went through every step (a recipe without a check,
	// or a tour).
	GuideSeen = "seen"
	// GuideSkipped: the person said they know it (e.g. "I know my way").
	GuideSkipped = "skipped"
)

// GuideHows lists the accepted values of GuideCompletion.How.
var GuideHows = []string{GuideChecked, GuideSeen, GuideSkipped}

// GuideCompletion records that a person completed a recipe. Completions are
// the whole guide section: never per-step activity.
type GuideCompletion struct {
	// Recipe is the recipe's identity as the caller names it: the recipe's
	// file name (e.g. "add-a-capability") or its full contribution URI. It is
	// stored verbatim and matched by exact string.
	Recipe string `json:"recipe" yaml:"recipe" jsonschema:"description=Recipe identity: its file name or its contribution URI - stored verbatim"`
	At     string `json:"at"     yaml:"at"     jsonschema:"description=When it was completed - RFC3339"`
	How    string `json:"how"    yaml:"how"    jsonschema:"description=How it was completed,enum=checked|seen|skipped"`
}

// GuideState is the guide section of the profile.
type GuideState struct {
	Completed []GuideCompletion `json:"completed" yaml:"completed,omitempty" jsonschema:"description=One entry per recipe - oldest first"`
}

// GuideCompleteInput records one completion.
type GuideCompleteInput struct {
	Recipe string `json:"recipe" jsonschema:"description=Recipe identity: its file name or its contribution URI - stored verbatim"`
	How    string `json:"how"    jsonschema:"description=How it was completed,enum=checked|seen|skipped"`
}

// Validate checks a completion input (after trimming).
func (in GuideCompleteInput) Validate() error {
	if strings.TrimSpace(in.Recipe) == "" {
		return fmt.Errorf("recipe is required")
	}
	if !slices.Contains(GuideHows, strings.TrimSpace(in.How)) {
		return fmt.Errorf("how %q must be one of %s", in.How, strings.Join(GuideHows, ", "))
	}
	return nil
}

// GuideResetInput forgets completions.
type GuideResetInput struct {
	Recipe string `json:"recipe,omitempty" jsonschema:"description=The recipe to forget; empty forgets every completion"`
}

// RelayState is the relay section of the profile: whether the person lets an
// agent's session relay their own acts (decide, approve or send back, answer,
// test or verify by hand, hold, waive) once they confirmed each one. Off until
// the person turns it on.
type RelayState struct {
	Enabled bool   `json:"enabled"      yaml:"enabled"`
	At      string `json:"at,omitempty" yaml:"at,omitempty" jsonschema:"description=When it was last turned on or off (RFC 3339)"`
}

// RelaySetInput turns relaying on or off — the person's own act: refused from
// an agent's session.
type RelaySetInput struct {
	Enabled bool `json:"enabled"`
}
