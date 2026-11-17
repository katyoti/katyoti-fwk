// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// SYSTEM MODULES (od-system-state-homes): an operational space the platform
// keeps for itself — the inbox, the journal, the unrouted requests — is a
// module named `sys-<name>`, committed, beside the functional module that
// describes the concept. The name is reserved both ways (CWM checks it on
// every write and reports it on every object read): a module named `sys-*`
// carries the `system-module` capability, and a module carrying it is named
// `sys-*`.
//
// A reader tells a system module from a user module by the CAPABILITY —
// IsSystemModule, UserModules — never by its type's name or by the prefix.

const (
	// CapModule is the capability every module carries.
	CapModule = "module"
	// CapSystemModule is the capability that marks a system module.
	CapSystemModule = "system-module"
	// SystemModulePrefix is the name prefix the capability reserves.
	SystemModulePrefix = "sys-"
	// SystemModuleType is the object type EnsureSystemModule creates.
	SystemModuleType = "system-module"
)

// SystemModule is the `system-module` capability's field-group: the one
// source of its data-type, `cap-system-module`, generated from it
// (cwm/contrib). Every field is optional: a system module of no capability
// (sys-inbox, sys-journal) says nothing.
type SystemModule struct {
	Operates string `json:"operates,omitempty" yaml:"operates,omitempty" jsonschema:"title=Operates,description=The capability whose operational space this module is (sys-contributions operates contributions): a capability the universe declares; empty for a module that operates none"`
}

// SystemModuleOperates is the capability a system module operates: its
// `system-module` group's `operates`; "" when it names none.
func SystemModuleOperates(obj Object) string {
	s, _ := obj.Capabilities[CapSystemModule]["operates"].(string)
	return s
}

// IsSystemModule reports whether the object id carries the system-module
// capability (its type's, its aspects', its own or one attached to it).
func IsSystemModule(ctx context.Context, cwm CWMService, id string) (bool, error) {
	return HasCapability(ctx, cwm, id, CapSystemModule)
}

// UserModules lists the project's user modules: every object carrying the
// module capability, less those carrying system-module.
func UserModules(ctx context.Context, cwm CWMService) ([]Object, error) {
	all, err := cwm.ObjectList(ctx, ObjectListInput{Capability: CapModule})
	if err != nil {
		return nil, err
	}
	sys, err := cwm.ObjectList(ctx, ObjectListInput{Capability: CapSystemModule})
	if err != nil {
		return nil, err
	}
	skip := make(map[string]bool, len(sys.Objects))
	for _, o := range sys.Objects {
		skip[o.ID] = true
	}
	out := make([]Object, 0, len(all.Objects))
	for _, o := range all.Objects {
		if !skip[o.ID] {
			out = append(out, o)
		}
	}
	return out, nil
}

// EnsureSystemModule gives the project the system module id (named
// `sys-<name>`) when it has none, and reports whether it created it. A
// concurrent ensure that created it first is not an error. The inbox, the
// journal and planification each ensure theirs through it. With several
// projects served, id is written <key>:sys-<name> to ensure it in the project
// of key (an observer's is EventProject).
func EnsureSystemModule(ctx context.Context, cwm CWMService, id, description string) (bool, error) {
	module, name := id, id
	if i := strings.Index(id, ":"); i >= 0 {
		if err := checkProjectKey(id[:i]); err != nil {
			return false, fmt.Errorf("%w: system module %q: %v", ErrInvalidInput, id, err)
		}
		name = id[i+1:]
	}
	if !strings.HasPrefix(name, SystemModulePrefix) || strings.ContainsAny(name, ".:") {
		return false, fmt.Errorf("%w: a system module is named %s<name> or <key>:%s<name>, not %q", ErrInvalidInput, SystemModulePrefix, SystemModulePrefix, id)
	}
	if _, err := cwm.ObjectGet(ctx, ObjectGetInput{Module: module, ID: name}); err == nil {
		return false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return false, err
	}
	mod := Object{ID: name, Type: SystemModuleType}
	mod.Name = name
	mod.Description = description
	if _, err := cwm.ObjectCreate(ctx, ObjectCreateInput{Module: module, Object: mod}); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			return false, nil
		}
		return false, fmt.Errorf("create %s: %w", id, err)
	}
	return true, nil
}
