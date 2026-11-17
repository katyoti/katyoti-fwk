// SPDX-License-Identifier: Apache-2.0

package contribapi

import "strings"

// Severity values for contribution validation. The set is intentionally
// small: a contribution either fails (error), warns (logged but kept
// valid), or is skipped entirely (off).
const (
	// ValidateOff disables validation for a (global or per-type) scope.
	ValidateOff = "off"
	// ValidateWarn logs diagnostics but keeps the contribution valid —
	// matches the catalog's warn-don't-block ethos. The global default.
	ValidateWarn = "warn"
	// ValidateOn marks a failing contribution invalid (Valid=false) and
	// records the diagnostics. Accepted spelling alongside "error".
	ValidateOn = "on"
	// ValidateError is the per-type opt-in synonym for ValidateOn — a
	// failing contribution is marked invalid. Kept-but-flagged: it stays
	// in the catalog, visibly marked, never silently dropped.
	ValidateError = "error"
)

// ContributionsConfig is the central OVERRIDE for validation severity, loaded
// through the standard config pipeline as `Contributions.yaml` (ConfigName
// strips the `Config` suffix).
//
// The severity itself is declared by each KIND (`validate:` in its
// contribution-type declaration) and resolved through Kinds.SeverityFor. This
// config never invents a default: an absent or empty file means every kind is
// validated at the severity it declares. Precedence: PerType[kind] > Validate >
// the kind's declaration > warn.
//
// It used to carry a built-in per-kind map that duplicated the kind files and
// left the kind's own `validate:` unread — three copies of one fact, two of
// them consulted. Now one.
type ContributionsConfig struct {
	// Validate is the global severity: "on"/"error", "warn", or "off".
	// Empty resolves to the default ("warn") in SeverityFor.
	Validate string `json:"validate,omitempty" yaml:"validate,omitempty"`

	// PerType overrides the global severity for specific contribution
	// types. Key is the `Type` discriminator; value is a severity. An
	// entry wins over Validate; absence falls back to Validate (then the
	// global default).
	PerType map[string]string `json:"perType,omitempty" yaml:"perType,omitempty"`
}

// defaultGlobalSeverity is the floor when neither the kind nor the config
// names a severity — visible, never blocking, never silent.
const defaultGlobalSeverity = ValidateWarn

// NormalizeSeverity folds "error" into "on" and lowercases / trims the
// input, defaulting unknown spellings to "warn" (fail safe: visible, not
// blocking).
func NormalizeSeverity(s string) string { return normalizeSeverity(s) }

func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ValidateOff:
		return ValidateOff
	case ValidateOn, ValidateError:
		return ValidateOn
	case ValidateWarn:
		return ValidateWarn
	default:
		return ValidateWarn
	}
}
