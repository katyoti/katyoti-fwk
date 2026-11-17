// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// yamlOnlyConfigKeys are `config:` keys the JSON decode cannot map onto the
// request (their yaml name differs from the json tag) and that the agent
// factory reads explicitly after it: `body` → Prompt, `capability` →
// ProfileName; plus the ConfigOverride wire keys the runtime spawn path
// consumes (the parent's spawn policies, the strategy name).
var yamlOnlyConfigKeys = map[string]bool{
	presets.ConfigKeyPrompt: true, presets.ConfigKeyCapability: true, "body": true,
	presets.OverrideKeySpawnPolicies: true, presets.OverrideKeyStrategy: true,
}

// IsPassThroughConfigKey reports whether a top-level `config:` key is
// accepted without being a field of AgentRunRequest: a yaml-only key the
// factory maps by hand, or a substrate-reserved key (`_` prefix:
// `_target_*`, `_mode`).
func IsPassThroughConfigKey(k string) bool {
	return yamlOnlyConfigKeys[k] || strings.HasPrefix(k, "_")
}

// UnknownConfigKeys returns, sorted, the top-level `config:` keys of an
// agent task or persona that the agent strategy does not accept — the one
// rule the run applies at start (agent/local decodeAgentConfig) and the
// lints apply before it. A key is known when it is a json tag of
// AgentRunRequest (matched case-insensitively, as encoding/json does:
// `MaxTurns` and `maxTurns` are one key, `max_turns` is not) or a
// pass-through key (IsPassThroughConfigKey). Nested keys are not judged.
func UnknownConfigKeys(keys []string) []string {
	known := requestConfigKeys()
	var unknown []string
	for _, k := range keys {
		if IsPassThroughConfigKey(k) || known[strings.ToLower(k)] {
			continue
		}
		unknown = append(unknown, k)
	}
	sort.Strings(unknown)
	return unknown
}

// requestConfigKeys lists the json tags of AgentRunRequest, lower-cased.
func requestConfigKeys() map[string]bool {
	rt := reflect.TypeOf(AgentRunRequest{})
	keys := make(map[string]bool, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = rt.Field(i).Name
		}
		if name != "-" {
			keys[strings.ToLower(name)] = true
		}
	}
	return keys
}
