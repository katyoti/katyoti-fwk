// SPDX-License-Identifier: Apache-2.0

// Package cli generates Cobra commands from KTSP container definitions.
//
// GenerateCLI reads a container's slots and methods, then creates
// a Cobra command tree: one command per slot, one subcommand per method.
// CLIMeta on each MethodDef controls positional args and flags.
//
// Slots tagged with cli="false" or internal="true" are skipped.
// Methods tagged with cli="false" are skipped.
package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	render "github.com/katyoti/katyoti-fwk/internal/extensions/tui/render"
	kcontainer "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// GenerateCLI adds Cobra commands to rootCmd from the container's slot/method definitions.
// It uses container.Info() for discovery and container.Slot() for CLIMeta access.
func GenerateCLI(rootCmd *cobra.Command, container *kcontainer.Endpoint) {
	info := container.Info()
	for _, slotInfo := range info.Slots {
		if shouldSkipSlot(slotInfo) {
			continue
		}

		// CLI generation reads the slot TYPE contract, which lives on
		// the DefaultInstance binding by convention — Info() already
		// surfaces the type-level methods/tags via that instance.
		slot := container.Slot(slotInfo.Name, kcontainer.DefaultInstance)
		if slot == nil {
			continue
		}

		// Flat mode: methods are added directly to rootCmd instead of being
		// nested under a slot command. Use cli.flat="true" on the slot.
		// Multi-segment cli.name paths still form noun-verb groups under root.
		if slotInfo.Tags[kcontainer.TagSlotCLIFlat] == "true" {
			groups := parseCLIGroups(slotInfo.Tags[kcontainer.TagSlotCLIGroups])
			parents := map[string]*cobra.Command{}
			for _, methodInfo := range slotInfo.Methods {
				if shouldSkipMethod(methodInfo) {
					continue
				}
				def := slot.Def(methodInfo.Name)
				var cliMeta *kcontainer.CLIMeta
				var renderTmpl string
				var outputAST *schemaapi.Type
				var live bool
				if def != nil {
					cliMeta = def.GetCLIMeta()
					renderTmpl = def.Opts().RenderTemplate
					outputAST, _ = def.Opts().OutputAST.(*schemaapi.Type)
					live = def.Opts().Live
				}
				methodCmd := buildMethodCommand(container, slotInfo.Name, methodInfo, cliMeta, renderTmpl, outputAST, live)
				attachToTree(rootCmd, parents, groups, cliNameForMethod(methodInfo), methodCmd)
			}
			continue
		}

		slotCmd := buildSlotCommand(container, slotInfo, slot)
		rootCmd.AddCommand(slotCmd)
	}
}

// shouldSkipSlot returns true if the slot should not generate CLI commands.
func shouldSkipSlot(s kcontainer.SlotInfo) bool {
	if s.Tags[kcontainer.TagSlotCLI] == "false" || s.Tags[kcontainer.TagSlotInternal] == "true" {
		return true
	}
	return false
}

// shouldSkipMethod returns true if the method should not generate a CLI subcommand.
func shouldSkipMethod(m kcontainer.MethodInfo) bool {
	return m.Tags[kcontainer.TagMethodCLI] == "false"
}

// cliGroupMeta mirrors meta.CLIGroup for decoding the slot's cli.groups tag.
// Field names match meta.CLIGroup so the JSON keys line up.
type cliGroupMeta struct {
	Name    string
	Aliases []string
	Short   string
}

// parseCLIGroups decodes the cli.groups slot tag into a path→group map.
func parseCLIGroups(tag string) map[string]cliGroupMeta {
	out := map[string]cliGroupMeta{}
	if tag == "" {
		return out
	}
	var groups []cliGroupMeta
	if err := json.Unmarshal([]byte(tag), &groups); err == nil {
		for _, g := range groups {
			out[g.Name] = g
		}
	}
	return out
}

// cliNameForMethod returns a method's full CLI path (its cli.name override, or
// its wire name when unset).
func cliNameForMethod(m kcontainer.MethodInfo) string {
	if override, ok := m.Tags[kcontainer.TagMethodCLIName]; ok && override != "" {
		return override
	}
	return m.Name
}

// lastCLISegment returns the final space-separated segment of a CLI path
// ("page pull" → "pull"; "search" → "search").
func lastCLISegment(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return name
	}
	return fields[len(fields)-1]
}

// attachToTree adds leaf under the group path encoded in cliName (all segments
// but the last), creating or reusing parent group commands under root. Parents
// are cached by cumulative path so siblings share ancestors; group metadata
// (Short + Aliases) is applied from groups when a level is first created. A
// single-segment cliName attaches leaf directly to root (flat, unchanged).
func attachToTree(root *cobra.Command, parents map[string]*cobra.Command, groups map[string]cliGroupMeta, cliName string, leaf *cobra.Command) {
	segs := strings.Fields(cliName)
	if len(segs) <= 1 {
		root.AddCommand(leaf)
		return
	}
	parent := root
	path := ""
	for _, seg := range segs[:len(segs)-1] {
		if path == "" {
			path = seg
		} else {
			path += " " + seg
		}
		g, ok := parents[path]
		if !ok {
			g = &cobra.Command{Use: seg}
			if gm, ok := groups[path]; ok {
				g.Short = gm.Short
				g.Aliases = gm.Aliases
			}
			parent.AddCommand(g)
			parents[path] = g
		}
		parent = g
	}
	parent.AddCommand(leaf)
}

// buildSlotCommand creates a Cobra command for a slot with subcommands for each method.
func buildSlotCommand(container *kcontainer.Endpoint, slotInfo kcontainer.SlotInfo, slot *kcontainer.Slot) *cobra.Command {
	name := slotInfo.Name
	if override, ok := slotInfo.Tags[kcontainer.TagSlotCLIName]; ok {
		name = override
	}

	cmd := &cobra.Command{
		Use:   name,
		Short: slotInfo.Description,
	}

	if alias, ok := slotInfo.Tags[kcontainer.TagSlotCLIAlias]; ok {
		cmd.Aliases = strings.Split(alias, ",")
	}

	// Multi-segment cli.name paths nest further UNDER the slot command.
	groups := parseCLIGroups(slotInfo.Tags[kcontainer.TagSlotCLIGroups])
	parents := map[string]*cobra.Command{}
	for _, methodInfo := range slotInfo.Methods {
		if shouldSkipMethod(methodInfo) {
			continue
		}

		def := slot.Def(methodInfo.Name)
		var cliMeta *kcontainer.CLIMeta
		var renderTmpl string
		var outputAST *schemaapi.Type
		var live bool
		if def != nil {
			cliMeta = def.GetCLIMeta()
			renderTmpl = def.Opts().RenderTemplate
			outputAST, _ = def.Opts().OutputAST.(*schemaapi.Type)
			live = def.Opts().Live
		}

		methodCmd := buildMethodCommand(container, slotInfo.Name, methodInfo, cliMeta, renderTmpl, outputAST, live)
		attachToTree(cmd, parents, groups, cliNameForMethod(methodInfo), methodCmd)
	}

	return cmd
}

// buildMethodCommand creates a Cobra subcommand for a single method.
//
// Output rendering precedence at command-run time:
//  1. `renderTemplate != ""` — legacy Go text/template path (kept for
//     methods explicitly opting into hand-coded templates via MethodDef).
//  2. `outputAST` carries a `ui-cli` extension — dispatch to the
//     `internal/extensions/tui/render` renderer (the 3L data-driven
//     path).
//  3. Everything else — write pretty JSON. Matches the pre-3L default.
//
// The `--out json|text|table|tree` flag (added in console.AddOutputFlag)
// can override (2) by forcing the renderer's mode regardless of the
// ui-cli extension; the renderer falls back to JSON for unknown modes.
func buildMethodCommand(container *kcontainer.Endpoint, slotName string, method kcontainer.MethodInfo, cliMeta *kcontainer.CLIMeta, renderTemplate string, outputAST *schemaapi.Type, live bool) *cobra.Command {
	name := method.Name
	if override, ok := method.Tags[kcontainer.TagMethodCLIName]; ok {
		name = override
	}
	// A multi-segment cli.name ("page pull") is a group path; the command
	// itself is named by only the last segment (the verb). The group parents
	// are created by attachToTree in GenerateCLI / buildSlotCommand.
	name = lastCLISegment(name)

	cmd := &cobra.Command{
		Use:   buildUse(name, cliMeta),
		Short: method.Description,
	}

	if method.Tags[kcontainer.TagMethodCLIHidden] == "true" {
		cmd.Hidden = true
	}

	if alias, ok := method.Tags[kcontainer.TagMethodCLIAlias]; ok {
		cmd.Aliases = strings.Split(alias, ",")
	}

	// Register flags from CLIMeta. A flag is spelled with dashes (dry_run →
	// --dry-run); the field's underscore spelling stays accepted as a hidden
	// alias, normalised to the dashed flag, so scripts written with it run.
	flagValues := make(map[string]*string)
	boolFlagValues := make(map[string]*bool)
	if cliMeta != nil {
		aliases := map[string]string{}
		cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
			if dashed, ok := aliases[name]; ok {
				return pflag.NormalizedName(dashed)
			}
			return pflag.NormalizedName(name)
		})
		for _, f := range cliMeta.Flags {
			name := cliFlagName(f.Name)
			for _, old := range []string{f.Name, f.JSONKey} {
				if old != "" && old != name && cliFlagName(old) == name {
					aliases[old] = name
				}
			}
			if f.Type == "boolean" {
				val := new(bool)
				boolFlagValues[f.Field] = val
				if f.Short != "" {
					cmd.Flags().BoolVarP(val, name, f.Short, false, buildFlagUsage(f))
				} else {
					cmd.Flags().BoolVar(val, name, false, buildFlagUsage(f))
				}
			} else {
				val := new(string)
				if f.Default != "" {
					*val = f.Default
				}
				flagValues[f.Field] = val

				if f.Short != "" {
					cmd.Flags().StringVarP(val, name, f.Short, f.Default, buildFlagUsage(f))
				} else {
					cmd.Flags().StringVar(val, name, f.Default, buildFlagUsage(f))
				}
			}

			if f.Required {
				cmd.MarkFlagRequired(name)
			}
		}
	}

	// Set arg validation
	if cliMeta != nil {
		requiredArgs := 0
		for _, a := range cliMeta.Args {
			if a.Required {
				requiredArgs++
			}
		}
		if requiredArgs > 0 {
			cmd.Args = cobra.MinimumNArgs(requiredArgs)
		}
	}

	methodName := method.Name
	mode := console.AddOutputFlag(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		params, err := buildParams(cliMeta, flagValues, boolFlagValues, args)
		if err != nil {
			return err
		}
		// The arguments and flags are valid from here on: an error the method
		// itself answers (a refusal, a not-found) is not a usage mistake, so
		// cobra prints the error alone, without the usage text.
		cmd.SilenceUsage = true

		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}

		// Live rendering (the terminal's live view): for a method
		// flagged live that renders via `ui-cli`, install a terminal emitter on
		// the dispatch ctx so the method's snapshots re-render the rich screen in
		// place. Gated to rich mode with no `--of` file redirect; `--out json`
		// and non-TTY defer to the static path (the LiveRenderer itself no-ops
		// updates off a TTY and prints the final frame once in Finalize).
		callCtx := cmd.Context()
		liveRenderer := maybeLiveRenderer(cmd, live, outputAST)
		if liveRenderer != nil {
			callCtx = progress.WithEmitter(callCtx, liveRenderer)
		}

		// Route through the running server if one exists for this app
		// (so a single chrome / single browser context / etc. is reused
		// across CLI invocations). Falls back to in-process dispatch if
		// no server is running. See route.go.
		result, err := dispatchCall(callCtx, container, slotName, methodName, data)
		if err != nil {
			return err
		}

		// --of flag: write JSON directly to file (UTF-8, bypasses shell encoding)
		if flags := console.OutputFlagsFromContext(cmd.Context()); flags != nil && flags.File != "" {
			var parsed any
			json.Unmarshal(result, &parsed)
			return console.RenderConsoleOrFile(*mode, methodName, parsed, nil, cmd.Context())
		}

		// Use render template for rich/raw output, JSON for --out json
		if renderTemplate != "" {
			var parsed any
			if err := json.Unmarshal(result, &parsed); err != nil {
				return writeOutput(cmd, result)
			}
			return console.RenderConsole(*mode, methodName, parsed, renderTemplate)
		}

		// Data-driven `ui-cli` rendering (3L): when the output Type AST
		// carries a `ui-cli` extension — or the user forced `--out
		// table/text/tree` — dispatch to the terminal renderer. Falls
		// through to pretty-JSON below when neither condition holds.
		if shouldUseUICLIRender(outputAST, cmd) {
			// Live path: repaint the final frame in place (covers a throttled-out
			// last snapshot) and restore the terminal. Off a TTY, Finalize prints
			// the result once — identical to the static render below.
			if liveRenderer != nil {
				liveRenderer.Finalize(result)
				return failureExit(cmd, outputAST, result)
			}
			ctx := render.RenderCtxFromWriter(cmd.OutOrStdout())
			ctx.ModeOverride = outputModeOverride(cmd)
			if err := render.Render(outputAST, result, ctx, cmd.OutOrStdout()); err != nil {
				return err
			}
			return failureExit(cmd, outputAST, result)
		}

		return writeOutput(cmd, result)
	}

	return cmd
}

// maybeLiveRenderer returns a LiveRenderer to install for a live method, or nil
// when live rendering doesn't apply. It applies only when: the method is flagged
// live, its output renders via a `ui-cli` extension, output is not forced to JSON
// (`--out json`), and there is no `--of` file redirect. The renderer itself gates
// TTY-ness — off a TTY, updates no-op and Finalize prints the final frame once —
// so callers get correct behavior for piped output without extra checks here.
func maybeLiveRenderer(cmd *cobra.Command, live bool, outputAST *schemaapi.Type) *render.LiveRenderer {
	if !live || !shouldUseUICLIRender(outputAST, cmd) {
		return nil
	}
	if outputModeOverride(cmd) == "json" {
		return nil
	}
	if flags := console.OutputFlagsFromContext(cmd.Context()); flags != nil && flags.File != "" {
		return nil
	}
	rctx := render.RenderCtxFromWriter(cmd.OutOrStdout())
	rctx.ModeOverride = outputModeOverride(cmd) // "" → rich (ui-cli extension)
	return render.NewLiveRenderer(outputAST, rctx, cmd.OutOrStdout())
}

// failureExit turns a rendered result that reports failed items into a non-zero
// exit, so a scripted `push` / `sync` can tell a clean run from a total failure.
// Output has already been written at this point — the returned error only sets
// the exit code, so usage is silenced and the message stays a one-liner (the
// per-item detail is in the rendered footer).
func failureExit(cmd *cobra.Command, outputAST *schemaapi.Type, result json.RawMessage) error {
	n, ok := render.FailureCount(outputAST, result)
	if !ok || n == 0 {
		return nil
	}
	// Output (including the per-item failure detail) is already on screen, so
	// suppress cobra's usage dump AND its "Error:" line — the sidecar's main()
	// prints the message itself, and cobra printing it too double-reports.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return fmt.Errorf("%d item(s) failed", n)
}

// shouldUseUICLIRender reports whether the new data-driven `ui-cli`
// renderer should handle output for this command. True when the output
// type's AST carries a `ui-cli` extension — opt-in by data-type
// authors. Honors the pagination-envelope heuristic: ui-cli tagged on
// the array field of a `{Items []Item; Total int}` envelope counts.
// False keeps the pre-3L behavior (legacy template or plain JSON) so
// methods that haven't opted in render exactly as before.
func shouldUseUICLIRender(ast *schemaapi.Type, cmd *cobra.Command) bool {
	return render.HasUICLI(ast)
}

// outputModeOverride maps the `--out` flag into the renderer's
// ModeOverride contract. Only `--out json` forces a renderer mode;
// `--out rich` and `--out raw` defer to the type's ui-cli extension
// (RenderCtxFromWriter already strips ANSI when stdout isn't a TTY).
func outputModeOverride(cmd *cobra.Command) string {
	flags := console.OutputFlagsFromContext(cmd.Context())
	if flags == nil {
		return ""
	}
	if flags.Mode == console.OutputJSON {
		return "json"
	}
	return ""
}

// cliFlagName is a generated flag's command-line name: the kebab form of its
// field's name (dry_run → dry-run, depends_on → depends-on).
func cliFlagName(name string) string {
	return strings.ReplaceAll(name, "_", "-")
}

// buildUse creates the Use string for a method command, including arg placeholders.
func buildUse(name string, cliMeta *kcontainer.CLIMeta) string {
	if cliMeta == nil || len(cliMeta.Args) == 0 {
		return name
	}
	parts := []string{name}
	for _, a := range cliMeta.Args {
		if a.Required {
			parts = append(parts, "<"+strings.ToLower(a.Field)+">")
		} else {
			parts = append(parts, "["+strings.ToLower(a.Field)+"]")
		}
	}
	return strings.Join(parts, " ")
}

// buildFlagUsage creates the usage string for a flag, including enum values.
func buildFlagUsage(f kcontainer.CLIFlag) string {
	if len(f.Enum) > 0 {
		return f.Desc + " (" + strings.Join(f.Enum, "|") + ")"
	}
	if f.Type == "stringSlice" {
		return strings.TrimSpace(f.Desc + " (comma-separated, or a JSON array when an element holds a comma)")
	}
	if f.Type == "int" || f.Type == "integer" {
		return strings.TrimSpace(f.Desc + " (a whole number)")
	}
	if f.Type == "float" || f.Type == "number" {
		return strings.TrimSpace(f.Desc + " (a number)")
	}
	if f.Type == "bytes" {
		return strings.TrimSpace(f.Desc + " (the text as written; @path reads a file; base64:<data> for binary)")
	}
	return f.Desc
}

// buildParams constructs the JSON params map from CLI args and flags.
// Returns an error when a typed flag value fails to parse (e.g. malformed
// stringMap input). The error is surfaced to the user as a CLI error.
func buildParams(cliMeta *kcontainer.CLIMeta, flagValues map[string]*string, boolFlagValues map[string]*bool, args []string) (map[string]any, error) {
	params := make(map[string]any)

	if cliMeta == nil {
		return params, nil
	}

	// Positional args
	for i, a := range cliMeta.Args {
		if i < len(args) {
			params[fieldToJSON(a.Field)] = args[i]
		}
	}

	// String flags
	for _, f := range cliMeta.Flags {
		if f.Type == "boolean" {
			continue // handled below
		}
		val, ok := flagValues[f.Field]
		if !ok || val == nil {
			continue
		}
		v := *val
		if f.Path {
			abs, err := cwdPath(v)
			if err != nil {
				return nil, fmt.Errorf("--%s: %w", cliFlagName(f.Name), err)
			}
			v = abs
		}
		if v == "" && f.Default == "" {
			continue // skip empty non-default flags
		}

		// The params payload must land under the JSON key the receiving
		// struct expects (its json tag, e.g. "dry_run"), which can differ
		// from the CLI flag Name when a `cli:` tag renames the flag
		// (e.g. flag "dry-run" → json "dry_run"). Prefer JSONKey; fall
		// back to Name for flags built without it.
		jsonKey := f.JSONKey
		if jsonKey == "" {
			jsonKey = f.Name
		}

		switch f.Type {
		case "int", "integer":
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("--%s wants a whole number, got %q", cliFlagName(f.Name), v)
			}
			params[jsonKey] = n
		case "float", "number":
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("--%s wants a number, got %q", cliFlagName(f.Name), v)
			}
			params[jsonKey] = n
		case "stringSlice":
			list, err := parseStringSlice(v)
			if err != nil {
				return nil, fmt.Errorf("--%s: %w", cliFlagName(f.Name), err)
			}
			params[jsonKey] = list
		case "stringMap":
			parsed, err := parseStringMap(v)
			if err != nil {
				return nil, fmt.Errorf("--%s: %w", cliFlagName(f.Name), err)
			}
			params[jsonKey] = parsed
		case "jsonObject":
			// map[string]any-shaped fields: caller passes raw JSON
			// text on the command line; unmarshal into a generic
			// map and store as the param value. The receiving
			// handler later json.Marshal+Unmarshal the whole params
			// map into its typed Input struct, so nested object
			// shapes survive the trip.
			raw, err := jsonFlagText(v)
			if err != nil {
				return nil, fmt.Errorf("--%s: %w", cliFlagName(f.Name), err)
			}
			var parsed map[string]any
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return nil, fmt.Errorf("--%s: invalid JSON object: %w", cliFlagName(f.Name), err)
			}
			params[jsonKey] = parsed
		case "bytes":
			// A []byte field: the text as written — a person or an agent
			// passes markdown, not its base64 — or a file's bytes (`@path`),
			// or base64 for binary (`base64:<data>`). json.Marshal of the
			// []byte re-encodes it, so the handler decodes the same bytes.
			raw, err := bytesFlagValue(v)
			if err != nil {
				return nil, fmt.Errorf("--%s: %w", cliFlagName(f.Name), err)
			}
			params[jsonKey] = raw
		case "jsonValue":
			// A struct or a list of them (an actor, deliverables): the
			// caller passes its JSON; it lands as is and the handler decodes
			// it into the field's type.
			raw, err := jsonFlagText(v)
			if err != nil {
				return nil, fmt.Errorf("--%s: %w", cliFlagName(f.Name), err)
			}
			var parsed any
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return nil, fmt.Errorf("--%s: invalid JSON: %w", cliFlagName(f.Name), err)
			}
			params[jsonKey] = parsed
		default:
			params[jsonKey] = v
		}
	}

	// Boolean flags
	for _, f := range cliMeta.Flags {
		if f.Type != "boolean" {
			continue
		}
		val, ok := boolFlagValues[f.Field]
		if !ok || val == nil {
			continue
		}
		if *val {
			jsonKey := f.JSONKey
			if jsonKey == "" {
				jsonKey = f.Name
			}
			params[jsonKey] = true
		}
	}

	return params, nil
}

// jsonFlagText is a JSON flag's text: the value itself, or — written `@path`,
// as curl does — the content of that file, for JSON too long for a command line.
// cwdPath is a path flag as the person typed it, resolved against the working
// directory: empty is that directory, an absolute path is kept.
func cwdPath(v string) (string, error) {
	if v == "" {
		v = "."
	}
	return filepath.Abs(v)
}

func jsonFlagText(v string) ([]byte, error) {
	if path, ok := strings.CutPrefix(v, "@"); ok {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read the JSON of %s: %w", path, err)
		}
		return raw, nil
	}
	return []byte(v), nil
}

// bytesFlagValue is a []byte flag's bytes: `@path` reads the file (as a JSON
// flag does), `base64:<data>` decodes binary, anything else is the text itself.
func bytesFlagValue(v string) ([]byte, error) {
	if path, ok := strings.CutPrefix(v, "@"); ok {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		return raw, nil
	}
	if data, ok := strings.CutPrefix(v, "base64:"); ok {
		raw, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("invalid base64: %w", err)
		}
		return raw, nil
	}
	return []byte(v), nil
}

// parseStringSlice reads a list flag. A JSON array — on the command line or,
// written `@path`, in a file — is the list as is, so an element may hold a
// comma. Any other value is split on commas, each element trimmed and empty
// ones dropped.
//
//	`["a, b","c"]` → ["a, b", "c"]
//	"a,b,c"        → ["a", "b", "c"]
//	"a, b ,c"      → ["a", "b", "c"]
//	""             → []string{}
func parseStringSlice(v string) ([]string, error) {
	trimmed := strings.TrimSpace(v)
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "@") {
		raw, err := jsonFlagText(trimmed)
		if err != nil {
			return nil, err
		}
		var list []string
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, fmt.Errorf("invalid JSON array of strings: %w", err)
		}
		if list == nil {
			list = []string{}
		}
		return list, nil
	}
	if v == "" {
		return []string{}, nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// parseStringMap parses a "k=v,k=v" CLI value into a map[string]string.
// Whitespace around keys and values is trimmed; quoted values lose their
// surrounding quotes. The pair separator is `,`, but commas INSIDE single or
// double quotes are part of the value (so a value can itself be a CSV list).
// An entry without "=" or with an empty key is rejected with an error so the
// user gets a clear message instead of silent data loss.
//
//	"a=1,b=2"                  → {"a": "1", "b": "2"}
//	"app_id=foo"               → {"app_id": "foo"}
//	"x = 'hello world'"        → {"x": "hello world"}
//	"app_ids='uuid1,uuid2'"    → {"app_ids": "uuid1,uuid2"}    // value-internal commas preserved
//	`app_ids="uuid1,uuid2"`    → {"app_ids": "uuid1,uuid2"}
//	"bad"                      → error
func parseStringMap(v string) (map[string]string, error) {
	out := make(map[string]string)
	if v == "" {
		return out, nil
	}
	for _, pair := range splitTopLevelCommas(v) {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.Index(pair, "=")
		if eq < 0 {
			return nil, fmt.Errorf("expected key=value, got %q", pair)
		}
		key := strings.TrimSpace(pair[:eq])
		val := strings.TrimSpace(pair[eq+1:])
		if key == "" {
			return nil, fmt.Errorf("empty key in %q", pair)
		}
		// Strip optional surrounding single or double quotes.
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
				val = val[1 : len(val)-1]
			}
		}
		out[key] = val
	}
	return out, nil
}

// splitTopLevelCommas splits s on `,` but treats commas inside single or
// double quotes as part of the surrounding token. Quote characters remain in
// the returned tokens — parseStringMap strips them after the split.
func splitTopLevelCommas(s string) []string {
	var parts []string
	var b strings.Builder
	var quote byte // 0 when not inside quotes; '\'' or '"' otherwise
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == 0 && (c == '\'' || c == '"'):
			quote = c
			b.WriteByte(c)
		case quote != 0 && c == quote:
			quote = 0
			b.WriteByte(c)
		case quote == 0 && c == ',':
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	parts = append(parts, b.String())
	return parts
}

// fieldToJSON converts a Go field name to its JSON equivalent (camelCase).
// This is a simple lowercase-first-letter conversion.
func fieldToJSON(field string) string {
	if field == "" {
		return field
	}
	return strings.ToLower(field[:1]) + field[1:]
}

// writeOutput writes the JSON result to stdout.
// If result is nil or JSON null, nothing is printed (handler already wrote output).
func writeOutput(cmd *cobra.Command, result json.RawMessage) error {
	if len(result) == 0 || string(result) == "null" {
		return nil
	}

	var formatted []byte
	var err error

	var parsed any
	if err := json.Unmarshal(result, &parsed); err == nil {
		formatted, err = json.MarshalIndent(parsed, "", "  ")
		if err != nil {
			formatted = result
		}
	} else {
		formatted = result
	}

	_, err = fmt.Fprintln(os.Stdout, string(formatted))
	return err
}

// WithContext returns a context.Context from a cobra.Command, for use in handlers.
func WithContext(cmd *cobra.Command) context.Context {
	return cmd.Context()
}
