// SPDX-License-Identifier: Apache-2.0

package application

// HookFn is a function that runs during a specific phase.
// It receives the fully wired Application for access to EM, SM, and Config.
type HookFn func(app *Application) error

// HookEntry is a named phase hook with its origin.
type HookEntry struct {
	Pkg  string // package identifier (e.g. "extensions.docs")
	Name string // hook name within the package (e.g. ".container")
	Fn   HookFn
}

// Extension is a function that registers phase hooks into an ApplicationInit.
// Write a raw func directly, or use a factory function that returns Extension.
type Extension func(*ApplicationInit)

// ApplicationInit accumulates phase hooks contributed by an Extension.
// Passed to Extension functions during Application.New().
// Only exposes hook registration — no access to SM or EM.
type ApplicationInit struct {
	hooks map[Phase][]HookEntry
}

func newApplicationInit() *ApplicationInit {
	return &ApplicationInit{hooks: make(map[Phase][]HookEntry)}
}

// Add appends a hook for the given phase.
func (i *ApplicationInit) Add(phase Phase, pkg, name string, fn HookFn) *ApplicationInit {
	i.hooks[phase] = append(i.hooks[phase], HookEntry{Pkg: pkg, Name: name, Fn: fn})
	return i
}

// AddRegisterPoints appends a hook for PhaseRegisterPoints.
func (i *ApplicationInit) AddRegisterPoints(pkg, name string, fn HookFn) *ApplicationInit {
	return i.Add(PhaseRegisterPoints, pkg, name, fn)
}

// AddContribute appends a hook for PhaseContribute.
func (i *ApplicationInit) AddContribute(pkg, name string, fn HookFn) *ApplicationInit {
	return i.Add(PhaseContribute, pkg, name, fn)
}

// AddAppInit appends a hook for PhaseAppInit.
func (i *ApplicationInit) AddAppInit(pkg, name string, fn HookFn) *ApplicationInit {
	return i.Add(PhaseAppInit, pkg, name, fn)
}

// AddBootstrap appends a hook for PhaseBootstrap.
func (i *ApplicationInit) AddBootstrap(pkg, name string, fn HookFn) *ApplicationInit {
	return i.Add(PhaseBootstrap, pkg, name, fn)
}

// AddExtension folds all hooks from another Extension into this one.
func (i *ApplicationInit) AddExtension(ext Extension) *ApplicationInit {
	tmp := newApplicationInit()
	ext(tmp)
	for phase, entries := range tmp.hooks {
		i.hooks[phase] = append(i.hooks[phase], entries...)
	}
	return i
}
