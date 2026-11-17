// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// STREAMS — the level above the release: a stream numbers its releases by one
// version scheme and makes them by one release mode (RQ-141 acts on the mode;
// here it is stored and checked). A release is in exactly one stream, its
// version following the stream's scheme — an ended release is exempt, its
// version is history; an increment may name the stream it aims at. A module
// in a stream is a lint finding: a module serves several streams.

// Streams implements api.StreamService.
type Streams struct{}

var _ api.StreamService = (*Streams)(nil)

// NewStreams returns the stream service.
func NewStreams() *Streams { return &Streams{} }

// streamFields reads a stream's own fields.
func streamFields(o cwmapi.Object) api.StreamFields {
	var f api.StreamFields
	if len(o.Fields) > 0 {
		if b, err := json.Marshal(o.Fields); err == nil {
			_ = json.Unmarshal(b, &f)
		}
	}
	return f
}

// withStreamFields writes a stream's own fields, keeping any other.
func withStreamFields(o cwmapi.Object, f api.StreamFields) (cwmapi.Object, error) {
	b, err := json.Marshal(f)
	if err != nil {
		return o, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return o, err
	}
	fields := map[string]any{}
	for k, v := range o.Fields {
		fields[k] = v
	}
	for _, k := range []string{"color", "order", "role", "version_scheme", "release_mode", "min_interval", "every", "last_cut"} {
		delete(fields, k)
	}
	for k, v := range m {
		fields[k] = v
	}
	o.Fields = fields
	return o, nil
}

func toStream(o cwmapi.Object) api.Stream {
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	f := streamFields(o)
	return api.Stream{ID: o.ID, Module: module, Name: o.Name, StreamFields: f, Cut: api.NextCut(f, time.Now())}
}

// loadStream reads a stream.
func loadStream(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, error) {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	if o.Type != api.StreamType {
		return cwmapi.Object{}, invalid("%s is a %q, not a stream", id, o.Type)
	}
	return o, nil
}

var (
	semverRe   = regexp.MustCompile(`^\d+\.\d+(\.\d+)?(-rc\d+)?$`)
	calendarRe = regexp.MustCompile(`^\d{4}\.(0[1-9]|1[0-2])(-rc\d+)?$`)
	intervalRe = regexp.MustCompile(`^[1-9]\d*d$`)
)

// checkScheme refuses an unknown scheme.
func checkScheme(scheme string) error {
	switch scheme {
	case "", api.SchemeSemver, api.SchemeCalendar, api.SchemeDate:
		return nil
	}
	return invalid("version scheme %q: semver or calendar or date", scheme)
}

// offScheme says why a version does not follow a scheme, or "".
func offScheme(scheme, version string) string {
	ok := true
	switch scheme {
	case api.SchemeSemver:
		ok = semverRe.MatchString(version)
	case api.SchemeCalendar:
		ok = calendarRe.MatchString(version)
	case api.SchemeDate:
		_, err := time.Parse("2006-01-02", version)
		ok = err == nil
	}
	if ok {
		return ""
	}
	return fmt.Sprintf("version %q does not follow the %s scheme (%s)", version, scheme, schemeExample(scheme))
}

func schemeExample(scheme string) string {
	switch scheme {
	case api.SchemeSemver:
		return "1.4.0 or 1.005 - -rcN allowed"
	case api.SchemeCalendar:
		return "2026.10 - -rcN allowed"
	}
	return "2026-10-10"
}

// checkMode refuses an unknown mode, or one missing its interval.
func checkMode(f api.StreamFields) error {
	switch f.ReleaseMode {
	case "", api.ModeManual:
	case api.ModeOnChange:
		if f.MinInterval != "" && !intervalRe.MatchString(f.MinInterval) {
			return invalid("min_interval %q: a number of days (1d or 2d)", f.MinInterval)
		}
	case api.ModePeriodic:
		if !intervalRe.MatchString(f.Every) {
			return invalid("a periodic stream says how often: every (14d), not %q", f.Every)
		}
	default:
		return invalid("release mode %q: manual or on-change or periodic", f.ReleaseMode)
	}
	return nil
}

// releaseVersion is what a release is numbered by: its version, else its name.
func releaseVersion(o cwmapi.Object) string {
	if v := releaseFields(o).Version; v != "" {
		return v
	}
	return o.Name
}

// releaseEnded reports a release whose version is history.
func releaseEnded(o cwmapi.Object) bool {
	st := releaseState(o)
	return st == api.ReleaseReleased || st == api.ReleaseDropped
}

// Create creates a stream under a module.
func (s *Streams) Create(ctx context.Context, in api.CreateStreamInput) (api.StreamOutput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || in.Module == "" {
		return api.StreamOutput{}, invalid("Create: module and name required")
	}
	if err := checkScheme(in.VersionScheme); err != nil {
		return api.StreamOutput{}, err
	}
	if err := checkMode(in.StreamFields); err != nil {
		return api.StreamOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.StreamOutput{}, err
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: in.Module, ID: in.Module}); err != nil {
		return api.StreamOutput{}, fmt.Errorf("streams.Create: module %s: %w", in.Module, err)
	}
	o := cwmapi.Object{Type: api.StreamType}
	o.Parent, o.Name = in.Module, name
	if o, err = withStreamFields(o, in.StreamFields); err != nil {
		return api.StreamOutput{}, err
	}
	if o, err = withCutTimer(o, in.StreamFields); err != nil {
		return api.StreamOutput{}, err
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: in.Module, Object: o})
	if err != nil {
		return api.StreamOutput{}, err
	}
	return api.StreamOutput{Stream: toStream(out.Object), Path: out.Path}, nil
}

// Get reads one stream.
func (s *Streams) Get(ctx context.Context, in api.GetInput) (api.StreamOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.StreamOutput{}, err
	}
	o, err := loadStream(ctx, cwm, in.ID)
	if err != nil {
		return api.StreamOutput{}, err
	}
	return api.StreamOutput{Stream: toStream(o)}, nil
}

// List lists the streams of a module, or of every module.
func (s *Streams) List(ctx context.Context, in api.ListStreamsInput) (api.ListStreamsOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListStreamsOutput{}, err
	}
	all, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module, Type: api.StreamType})
	if err != nil {
		return api.ListStreamsOutput{}, err
	}
	out := api.ListStreamsOutput{Streams: []api.Stream{}}
	for _, o := range all.Objects {
		full, err := loadStream(ctx, cwm, o.ID)
		if err != nil {
			return api.ListStreamsOutput{}, err
		}
		out.Streams = append(out.Streams, toStream(full))
	}
	sort.Slice(out.Streams, func(a, b int) bool { return out.Streams[a].ID < out.Streams[b].ID })
	return out, nil
}

// Set changes a stream's scheme or mode; an empty field keeps its value.
func (s *Streams) Set(ctx context.Context, in api.SetStreamInput) (api.StreamOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.StreamOutput{}, err
	}
	o, err := loadStream(ctx, cwm, in.ID)
	if err != nil {
		return api.StreamOutput{}, err
	}
	f := streamFields(o)
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&f.Color, in.Color)
	set(&f.Role, in.Role)
	set(&f.VersionScheme, in.VersionScheme)
	set(&f.ReleaseMode, in.ReleaseMode)
	set(&f.MinInterval, in.MinInterval)
	set(&f.Every, in.Every)
	if in.Order != 0 {
		f.Order = in.Order
	}
	if err := checkScheme(f.VersionScheme); err != nil {
		return api.StreamOutput{}, err
	}
	if err := checkMode(f); err != nil {
		return api.StreamOutput{}, err
	}
	if o, err = withStreamFields(o, f); err != nil {
		return api.StreamOutput{}, err
	}
	if o, err = withCutTimer(o, f); err != nil {
		return api.StreamOutput{}, err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return api.StreamOutput{}, err
	}
	return api.StreamOutput{Stream: toStream(out.Object)}, nil
}

// checkInStream refuses a release whose version is off its stream's scheme
// (an ended release is exempt).
func checkInStream(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, stream string) error {
	so, err := loadStream(ctx, cwm, stream)
	if err != nil {
		return err
	}
	if releaseEnded(rel) {
		return nil
	}
	if why := offScheme(streamFields(so).VersionScheme, releaseVersion(rel)); why != "" {
		return invalid("%s in %s: %s", rel.ID, stream, why)
	}
	return nil
}

// ReleaseIn puts a release in a stream, replacing the one it was in.
func (s *Streams) ReleaseIn(ctx context.Context, in api.StreamInInput) (api.ReleaseOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	if rel.Type != api.ReleaseType {
		return api.ReleaseOutput{}, invalid("ReleaseIn: %s is an increment - it names its stream with AimIncrement", in.ID)
	}
	if err := checkInStream(ctx, cwm, rel, in.Stream); err != nil {
		return api.ReleaseOutput{}, err
	}
	if openRelease(rel) && !personsException(rel) && edgeTo(rel, api.EdgeInStream) != in.Stream {
		if why, err := noRoomInStream(ctx, cwm, in.Stream); err != nil {
			return api.ReleaseOutput{}, err
		} else if why != "" {
			return api.ReleaseOutput{}, invalid("ReleaseIn: %s", why)
		}
	}
	rel = withEdge(rel, api.EdgeInStream, in.Stream)
	module, _ := cwmapi.ModuleOfObjectID(rel.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: rel})
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	// Its work's to-dos carry the stream's label.
	if err := settleAll(ctx, PlanningActor); err != nil {
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(out.Object)}, nil
}

// AimIncrement names the stream an increment aims at, replacing the one it named.
func (s *Streams) AimIncrement(ctx context.Context, in api.StreamInInput) (api.IncrementOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	inc, err := loadIncrement(ctx, cwm, in.ID)
	if err != nil {
		return api.IncrementOutput{}, err
	}
	if _, err := loadStream(ctx, cwm, in.Stream); err != nil {
		return api.IncrementOutput{}, err
	}
	inc = withEdge(inc, api.EdgeInStream, in.Stream)
	module, _ := cwmapi.ModuleOfObjectID(inc.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: inc})
	if err != nil {
		return api.IncrementOutput{}, err
	}
	return api.IncrementOutput{Increment: toIncrement(ctx, out.Object)}, nil
}

// Lint is the lint hook of a stream: a module in it.
func (s *Streams) Lint(ctx context.Context, in api.HookInput) (api.LintOutput, error) {
	out := api.LintOutput{Status: "ok", Findings: []api.LintFinding{}}
	if in.Object == nil || in.Object.Type != api.StreamType {
		return out, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.LintOutput{}, err
	}
	members, err := incoming(ctx, cwm, in.Object.ID, api.EdgeInStream)
	if err != nil {
		return api.LintOutput{}, err
	}
	for _, id := range members {
		if o, _, err := loadAny(ctx, cwm, id); err == nil && levelOf(ctx, o.Type) == cwmapi.CapModule {
			out.Findings = append(out.Findings, api.LintFinding{Rule: "module-in-stream", Severity: "warn",
				Message: fmt.Sprintf("module %s is in %s: a module serves several streams - put its releases in the stream instead (streams release-in)", id, in.Object.ID)})
		}
	}
	if len(out.Findings) > 0 {
		out.Status = "findings"
	}
	return out, nil
}

// streamLint is the release lint's finding on a release off its stream's
// scheme, or nil.
func streamLint(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) *api.LintFinding {
	sm := edgeTo(rel, api.EdgeInStream)
	if sm == "" || rel.Type != api.ReleaseType || releaseEnded(rel) {
		return nil
	}
	so, err := loadStream(ctx, cwm, sm)
	if err != nil {
		return nil
	}
	if why := offScheme(streamFields(so).VersionScheme, releaseVersion(rel)); why != "" {
		return &api.LintFinding{Rule: "version-off-scheme", Severity: "warn", Message: fmt.Sprintf("%s in %s: %s", rel.ID, sm, why)}
	}
	return nil
}

// noStreamLint is the lint's finding on an open increment in no stream, or
// nil: the views list increments through their stream - Team priorities shows
// it under « No stream » until it is put in one.
func noStreamLint(o cwmapi.Object) *api.LintFinding {
	if o.Type != api.IncrementType || releaseState(o) != api.IncrementOpen || edgeTo(o, api.EdgeInStream) != "" {
		return nil
	}
	return &api.LintFinding{Rule: "increment-no-stream", Severity: "warn",
		Message: fmt.Sprintf("%s is in no stream: put it in one (streams aim-increment), or plan it for a release (increments plan-for), which gives it the release's stream", o.ID)}
}
