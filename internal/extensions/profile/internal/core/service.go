// SPDX-License-Identifier: Apache-2.0

// Package core implements the ProfileService: the profile kept in
// `<GlobalHomeDir>/config/Profile.yaml`, written only by the hub that serves it.
//
// The file holds the profile and feature sections (`guide`). Every write loads
// the file first and changes only the section it owns, so no writer drops
// another's section — including a section this build does not know.
package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// fileName is the config name: `Profile.yaml`.
const fileName = "Profile"

// fileVersion is the on-disk format version.
const fileVersion = 1

// file is the on-disk shape: a format version, the profile, the feature
// sections, and any section a newer writer added (kept as read).
type file struct {
	Version     int `yaml:"version"`
	api.Profile `yaml:",inline"`
	Guide       *api.GuideState `yaml:"guide,omitempty"`
	Relay       *api.RelayState `yaml:"relay,omitempty"`
	Other       map[string]any  `yaml:",inline"`
}

// Options configure a Service.
type Options struct {
	// Dir holds Profile.yaml — the global config layer, `<GlobalHomeDir>/config`.
	Dir string
	// OnChange runs after every successful write (Update, GuideComplete,
	// GuideReset) with the profile as saved. Best-effort: it must not fail the
	// save that already happened.
	OnChange func(ctx context.Context, p api.Profile)
	// Suggest reads the OS identity; nil suggests nothing.
	Suggest func(ctx context.Context) api.Suggestion
	// Now stamps completions; nil is time.Now.
	Now func() time.Time
}

// Service is the ProfileService.
type Service struct {
	opts Options
	mu   sync.Mutex
}

var _ api.ProfileService = (*Service)(nil)

// New returns a Service over opts.Dir.
func New(opts Options) *Service {
	return &Service{opts: opts}
}

// Get returns the saved profile, or the placeholder when there is none.
func (s *Service) Get(_ context.Context) (api.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return api.Profile{}, err
	}
	return profileOf(f), nil
}

// Update validates in and saves it as the local profile, keeping every other
// section of the file.
func (s *Service) Update(ctx context.Context, in api.UpdateInput) (api.Profile, error) {
	p := in.Profile()
	if err := p.Validate(); err != nil {
		return api.Profile{}, err
	}
	if _, err := s.write(ctx, func(f *file) bool { f.Profile = p; return true }); err != nil {
		return api.Profile{}, err
	}
	return p, nil
}

// Suggest returns what the OS knows about the user.
func (s *Service) Suggest(ctx context.Context) (api.Suggestion, error) {
	if s.opts.Suggest == nil {
		return api.Suggestion{}, nil
	}
	return s.opts.Suggest(ctx), nil
}

// GuideGet returns the recipes completed so far.
func (s *Service) GuideGet(_ context.Context) (api.GuideState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return api.GuideState{}, err
	}
	return guideOf(f), nil
}

// GuideComplete records a completion, replacing an earlier entry for the same
// recipe, stamped now.
func (s *Service) GuideComplete(ctx context.Context, in api.GuideCompleteInput) (api.GuideState, error) {
	if err := in.Validate(); err != nil {
		return api.GuideState{}, err
	}
	now := time.Now
	if s.opts.Now != nil {
		now = s.opts.Now
	}
	c := api.GuideCompletion{
		Recipe: strings.TrimSpace(in.Recipe),
		At:     now().UTC().Format(time.RFC3339),
		How:    strings.TrimSpace(in.How),
	}
	return s.write(ctx, func(f *file) bool {
		g := guideOf(*f)
		g.Completed = slices.DeleteFunc(g.Completed, func(e api.GuideCompletion) bool { return e.Recipe == c.Recipe })
		g.Completed = append(g.Completed, c)
		f.Guide = &g
		return true
	})
}

// GuideReset forgets one recipe's completion, or all of them when in.Recipe is
// empty. Nothing is written (nor announced) when nothing is forgotten.
func (s *Service) GuideReset(ctx context.Context, in api.GuideResetInput) (api.GuideState, error) {
	recipe := strings.TrimSpace(in.Recipe)
	return s.write(ctx, func(f *file) bool {
		g := guideOf(*f)
		before := len(g.Completed)
		if recipe == "" {
			g.Completed = g.Completed[:0]
		} else {
			g.Completed = slices.DeleteFunc(g.Completed, func(e api.GuideCompletion) bool { return e.Recipe == recipe })
		}
		if len(g.Completed) == before {
			return false
		}
		if len(g.Completed) == 0 {
			f.Guide = nil
		} else {
			f.Guide = &g
		}
		return true
	})
}

// RelayGet returns the relay section: off when never set.
func (s *Service) RelayGet(_ context.Context) (api.RelayState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.read()
	if err != nil {
		return api.RelayState{}, err
	}
	return relayOf(f), nil
}

// RelaySet turns relaying on or off, stamped now. It is the person's own act:
// a call whose cause is an agent is refused, whatever it says.
func (s *Service) RelaySet(ctx context.Context, in api.RelaySetInput) (api.RelayState, error) {
	if c, ok := cause.CauseFrom(ctx); ok && c.Kind == cause.KindAgent {
		return api.RelayState{}, fmt.Errorf("profile.RelaySet: relaying is turned on or off by the person themselves (katyoti profile relay, or the profile in the GUI) - not from an agent's session (%s)", c.ID)
	}
	now := time.Now
	if s.opts.Now != nil {
		now = s.opts.Now
	}
	r := api.RelayState{Enabled: in.Enabled, At: now().UTC().Format(time.RFC3339)}
	if _, err := s.write(ctx, func(f *file) bool { f.Relay = &r; return true }); err != nil {
		return api.RelayState{}, err
	}
	return r, nil
}

// relayOf is the relay section a file holds: off when absent.
func relayOf(f file) api.RelayState {
	if f.Relay == nil {
		return api.RelayState{}
	}
	return *f.Relay
}

// ReadRelay is the relay section of dir's Profile.yaml — off when it is
// absent or the file cannot be read. A read for the guards of the person's
// acts, in whichever process serves them (person.AcceptRelayed).
func ReadRelay(dir string) api.RelayState {
	f, err := New(Options{Dir: dir}).read()
	if err != nil {
		return api.RelayState{}
	}
	return relayOf(f)
}

// write loads the file, lets change edit the section it owns, and saves the
// whole file back when change reports a change; then announces it.
func (s *Service) write(ctx context.Context, change func(f *file) bool) (api.GuideState, error) {
	s.mu.Lock()
	f, err := s.read()
	if err != nil {
		s.mu.Unlock()
		return api.GuideState{}, err
	}
	if !change(&f) {
		s.mu.Unlock()
		return guideOf(f), nil
	}
	f.Version = fileVersion
	err = config.SaveAtomic(s.opts.Dir, fileName, f)
	s.mu.Unlock()
	if err != nil {
		return api.GuideState{}, fmt.Errorf("save profile: %w", err)
	}
	if s.opts.OnChange != nil {
		s.opts.OnChange(ctx, profileOf(f))
	}
	return guideOf(f), nil
}

// read loads the file; a missing file is an empty one.
func (s *Service) read() (file, error) {
	var f file
	if err := config.LoadFromDir(s.opts.Dir, fileName, &f); err != nil {
		return file{}, fmt.Errorf("load profile: %w", err)
	}
	return f, nil
}

// ReadHandle is the handle of the profile set in dir's Profile.yaml — "" when
// none is set (the placeholder is nobody) or the file cannot be read. A read
// for the callers that must not wait on the service (a CLI start naming the
// person as the cause of its writes); every write goes through the service.
func ReadHandle(dir string) string {
	f, err := New(Options{Dir: dir}).read()
	if err != nil {
		return ""
	}
	return profileOf(f).Handle()
}

// profileOf is the profile a file holds: the placeholder when none has been
// set (no file, or a file holding only feature sections).
func profileOf(f file) api.Profile {
	if f.ID == "" {
		return api.Placeholder()
	}
	p := f.Profile
	if p.Source == "" {
		p.Source = api.SourceLocal // a hand-written file is still a local claim
	}
	return p
}

// guideOf is a copy of the file's guide section; Completed is never nil.
func guideOf(f file) api.GuideState {
	g := api.GuideState{Completed: []api.GuideCompletion{}}
	if f.Guide != nil {
		g.Completed = append(g.Completed, f.Guide.Completed...)
	}
	return g
}
