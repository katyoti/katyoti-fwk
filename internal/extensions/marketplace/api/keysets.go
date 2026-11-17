// SPDX-License-Identifier: Apache-2.0

package api

// The key sets a solution provides (modules/katyoti/solutions.md §6): its
// vaults — sealed keys files published in its marketplace — opened together
// with the one team password, then the keys still missing listed for one form.

// KeySetsInput names a solution and the universe it was installed into (its
// main universe: each other universe of the solution is found beside it).
type KeySetsInput struct {
	// Solution is the solution (name[@version]); none: the universe alone —
	// no key set, only the keys its config check still asks for.
	Solution ItemRef `json:"solution,omitempty" desc:"The solution (name[@version]); empty: the universe alone"`
	// UniverseHome is the solution's main universe: a registered name or a folder.
	UniverseHome string `json:"universeHome" cli:"universe" desc:"The solution's main universe: a registered name or a folder" required:"true"`
	// Password, when given, is tried on every key set — nothing is written:
	// it says whether the sets share it and which keys each provides.
	Password string `json:"password,omitempty" desc:"The team password to try on every key set (nothing is written)"`
}

// KeySetsOutput is what the Ready to start screen shows for the keys: the
// key sets (one password field when there is one), then the form.
type KeySetsOutput struct {
	Solution string `json:"solution,omitempty" desc:"The solution (name@version)"`
	// KeySets are the solution's sealed key sets, in its order; each opened
	// only when a password was given and opens it.
	KeySets []VaultState `json:"keySets,omitempty" desc:"The solution's sealed key sets (its vaults); opened once a password opens them"`
	// PasswordNeeded: the solution provides key sets, so the screen asks the
	// team password once.
	PasswordNeeded bool `json:"passwordNeeded" desc:"The solution provides key sets: ask the team password once"`
	// SharedPassword: the password given opened every key set.
	SharedPassword bool `json:"sharedPassword,omitempty" desc:"The password given opened every key set"`
	// Universes are the solution's universes found on this machine, the main
	// one first: where the keys go.
	Universes []string `json:"universes,omitempty" desc:"The universes the keys go to (homes), the main one first"`
	// Remaining are the required keys still without a value that no opened
	// key set provides: the one form.
	Remaining []RemainingKey `json:"remaining,omitempty" desc:"The required keys still without a value that no opened key set provides: the one form"`
}

// RemainingKey is one key of the form: a required key with no value yet, as
// the universe's config check (its config-sets' report, each wired sidecar's
// own check) reports it.
type RemainingKey struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	// Scope is where its value is written: universe (that universe's .env) or
	// global (every universe's).
	Scope string `json:"scope"`
	// Source: the config-set manifest, or "sidecar:<name>" (its own check).
	Source string `json:"source,omitempty"`
	// UniverseHome is the universe that asks for it (a global key: the first).
	UniverseHome string `json:"universeHome"`
	// ConfigSet (name@version) is the config-set whose report lists it — the
	// form saves its value through ApplyConfigSet with it; empty when the
	// universe has no config-set (a sidecar's check alone asks for it): the
	// form saves it through ApplyConfigSet with an empty name, the universe's
	// own key set.
	ConfigSet string `json:"configSet,omitempty"`
}

// ImportKeySetsInput imports every key set of a solution with one password.
type ImportKeySetsInput struct {
	Solution     ItemRef `json:"solution" desc:"The solution (name[@version])" required:"true"`
	UniverseHome string  `json:"universeHome" cli:"universe" desc:"The solution's main universe: a registered name or a folder" required:"true"`
	Password     string  `json:"password" desc:"The team password: opens every key set" required:"true"`
	// Overwrite replaces a key already set to another value (default: kept).
	Overwrite bool `json:"overwrite,omitempty" cli:"overwrite" desc:"Replace keys already set to another value"`
}

// ImportKeySetsOutput says what the import wrote, and what is left for the form.
type ImportKeySetsOutput struct {
	Solution  string       `json:"solution" desc:"The solution (name@version)"`
	KeySets   []VaultState `json:"keySets,omitempty" desc:"Every key set, opened"`
	Universes []string     `json:"universes,omitempty" desc:"The universes the keys went to (homes), the main one first"`
	// Keys is each key of the sets, with what the import did in the main
	// universe (new, same, differs, written).
	Keys    []KeyEntry `json:"keys,omitempty" desc:"Each key of the sets and what the import did (main universe)"`
	Written []string   `json:"written,omitempty" desc:"The keys whose value was written (any universe)"`
	// Remaining are the required keys still without a value: the one form.
	Remaining []RemainingKey `json:"remaining,omitempty" desc:"The required keys still without a value: the one form"`
	// Restarts are the running members restarted because they read a key
	// written, in any universe.
	Restarts []MemberRestart `json:"restarts,omitempty" desc:"The running members restarted because they read a key written"`
}
