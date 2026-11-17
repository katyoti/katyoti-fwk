// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// ServiceMethod is re-exported from core/reflect.
type ServiceMethod = corereflect.ServiceMethod

// DiscoverMethods is re-exported from core/reflect.
var DiscoverMethods = corereflect.DiscoverMethods

// BuildKTSPSlot builds a KTSP Slot from a service instance using reflection.
// `instance` is the instance name bound under (name); pass
// ktsp.DefaultInstance for singletons. The slot's typeName IS the
// contract identifier — the SlotType categorical marker was retired in
// the Phase-A++ cleanup.
func BuildKTSPSlot(name, instance, desc string, impl any) *ktsp.Slot {
	return buildKTSPSlot(name, instance, desc, impl, nil, nil, nil)
}

// BuildKTSPSlotWithTags builds a KTSP Slot with custom tags and method definitions.
func BuildKTSPSlotWithTags(name, instance, desc string, impl any, tags ktsp.SlotTags, methodDefs map[string]meta.MethodDef) *ktsp.Slot {
	return buildKTSPSlot(name, instance, desc, impl, tags, methodDefs, nil)
}

// BuildKTSPSlotLazy builds a KTSP Slot whose method handlers re-resolve the
// service instance on every request via the resolver. This lets a later
// PutService (e.g., wrapping the local SkillManager with a composite that
// includes remote sidecars) be picked up by the KTSP surface — the auto-
// container would otherwise capture the impl at AppInit time.
//
// impl is still used at build time for type discovery (method shape +
// JSON schemas) and as the fallback dispatch target if resolve returns
// an error or nil.
func BuildKTSPSlotLazy(name, instance, desc string, impl any, tags ktsp.SlotTags, methodDefs map[string]meta.MethodDef, resolve func() (any, error)) *ktsp.Slot {
	return buildKTSPSlot(name, instance, desc, impl, tags, methodDefs, resolve)
}

func buildKTSPSlot(name, instance, desc string, impl any, tags ktsp.SlotTags, methodDefs map[string]meta.MethodDef, resolve func() (any, error)) *ktsp.Slot {
	implVal := reflect.ValueOf(impl)
	serviceType := implVal.Type()

	slot := ktsp.NewSlot(name, instance, desc, tags)

	discovered := corereflect.DiscoverMethods(serviceType)
	for _, sm := range discovered {
		// Skip hidden methods
		if def, ok := methodDefs[sm.Name]; ok && def.Hidden {
			continue
		}

		methodName := corereflect.ToKebab(sm.Name)
		cliMeta := BuildCLIMeta(sm.InputType)

		// Generate JSON Schemas from the Go types so sidecar clients (and the
		// LLM agent bridge) actually see typed contracts. Without these, every
		// published method falls back to `{"type":"object"}` and is effectively
		// uncallable over the wire. Failure is non-fatal — we log and move on,
		// because an incomplete schema is better than refusing to publish.
		inputSchema := generateInputSchema(sm)
		outputSchema := generateOutputSchema(sm)
		outputAST := generateOutputAST(sm)

		// Method description, render template, and CLI-name override from definition.
		// CLIName is injected as a tag so cli/generate.go picks it up via
		// TagMethodCLIName — the method is still stored under its kebab-cased
		// wire name so KTSP/HTTP/MCP addressing stays unchanged.
		methodDesc := fmt.Sprintf("%s operation", sm.Name)
		var renderTemplate string
		var methodTags ktsp.MethodTags
		var timeoutS int
		var live bool
		if def, ok := methodDefs[sm.Name]; ok {
			if def.Short != "" {
				methodDesc = def.Short
			}
			renderTemplate = def.RenderTemplate
			if def.CLIName != "" {
				methodTags = ktsp.MethodTags{ktsp.TagMethodCLIName: def.CLIName}
			}
			timeoutS = def.TimeoutS
			live = def.Live
			if def.ReadOnly {
				if methodTags == nil {
					methodTags = ktsp.MethodTags{}
				}
				methodTags[ktsp.TagMethodReadOnly] = "true"
			}
			if def.LongRunning {
				if methodTags == nil {
					methodTags = ktsp.MethodTags{}
				}
				methodTags[ktsp.TagMethodLong] = "true"
			}
			if def.NoLLM {
				if methodTags == nil {
					methodTags = ktsp.MethodTags{}
				}
				methodTags[ktsp.TagMethodMCP] = "false"
			}
		}

		handler := buildReflectHandler(implVal, sm, resolve)
		slot.Add(ktsp.RawMethod(methodName, ktsp.MethodOpts{
			Desc:           methodDesc,
			Tags:           methodTags,
			InputSchema:    inputSchema,
			OutputSchema:   outputSchema,
			OutputAST:      outputAST,
			CLIMeta:        cliMeta,
			RenderTemplate: renderTemplate,
			TimeoutS:       timeoutS,
			Live:           live,
		}, handler))
	}

	return slot
}

// generateInputSchema produces a decoded JSON Schema (map/slice tree) for the
// method's input struct. Returns nil for no-input methods and for methods
// whose input type cannot be introspected (failure is logged but not fatal —
// a published method with a missing schema is better than one that refuses
// to exist).
//
// We decode the raw JSON schema into `any` so both JSON and YAML encoders
// can walk the tree naturally. Storing the raw bytes (json.RawMessage) would
// cause yaml.v3 to serialize them as a `[]int` byte sequence — illegible.
func generateInputSchema(sm corereflect.ServiceMethod) any {
	if sm.InputType == nil {
		return nil
	}
	t := sm.InputType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	raw, err := corereflect.JSONSchemaForType(t)
	if err != nil {
		slog.Warn("ktsp: input schema generation failed", "method", sm.Name, "type", t.String(), "error", err)
		return nil
	}
	return decodeSchema(raw, sm.Name, "input")
}

// generateOutputSchema produces a decoded JSON Schema for the method's
// output struct. See generateInputSchema for error handling rationale.
func generateOutputSchema(sm corereflect.ServiceMethod) any {
	if sm.OutputType == nil {
		return nil
	}
	t := sm.OutputType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	raw, err := corereflect.JSONSchemaForType(t)
	if err != nil {
		slog.Warn("ktsp: output schema generation failed", "method", sm.Name, "type", t.String(), "error", err)
		return nil
	}
	return decodeSchema(raw, sm.Name, "output")
}

// generateOutputAST produces the canonical Type AST for the method's
// output type, returned as `any` so MethodOpts.OutputAST stays free of
// a schemaapi import. The CLI renderer (internal/extensions/tui/render)
// type-asserts back to `*schemaapi.Type` at command-run time and reads
// the attached `ui-cli` extension to drive output formatting. Nil = no
// AST captured (no output type / reflection failure); the renderer
// falls back to pretty JSON in that case.
func generateOutputAST(sm corereflect.ServiceMethod) any {
	if sm.OutputType == nil {
		return nil
	}
	t := sm.OutputType
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	ast, err := corereflect.TypeASTForType(t)
	if err != nil {
		slog.Warn("ktsp: output AST generation failed", "method", sm.Name, "type", t.String(), "error", err)
		return nil
	}
	return ast
}

// decodeSchema unmarshals a raw JSON schema into a generic Go value so it
// can be re-encoded to either JSON or YAML without the `[]byte → []int`
// yaml.v3 footgun. Logs and returns nil on decode failure.
func decodeSchema(raw json.RawMessage, methodName, kind string) any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		slog.Warn("ktsp: schema decode failed", "method", methodName, "kind", kind, "error", err)
		return nil
	}
	return v
}

// buildReflectHandler creates a KTSP raw handler that dispatches to a method via reflection.
//
// If resolve is non-nil, it is called on every request to fetch the current
// service implementation (so a Bootstrap-time service replacement — e.g.,
// wrapping with a composite — is honored at request time). Otherwise the
// captured implVal is used directly.
func buildReflectHandler(implVal reflect.Value, sm corereflect.ServiceMethod, resolve func() (any, error)) func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	return func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		dispatchVal := implVal
		if resolve != nil {
			cur, err := resolve()
			if err == nil && cur != nil {
				dispatchVal = reflect.ValueOf(cur)
			}
		}
		method := dispatchVal.MethodByName(sm.Name)
		if !method.IsValid() {
			return nil, fmt.Errorf("method %s not found on resolved service", sm.Name)
		}

		var results []reflect.Value

		if sm.InputType == nil {
			results = method.Call([]reflect.Value{reflect.ValueOf(ctx)})
		} else {
			inPtr := reflect.New(sm.InputType)
			if len(params) > 0 {
				if err := json.Unmarshal(params, inPtr.Interface()); err != nil {
					return nil, fmt.Errorf("unmarshal %s input: %w", sm.Name, err)
				}
			}
			results = method.Call([]reflect.Value{reflect.ValueOf(ctx), inPtr.Elem()})
		}

		if !results[1].IsNil() {
			return nil, results[1].Interface().(error)
		}

		out, err := json.Marshal(results[0].Interface())
		if err != nil {
			return nil, fmt.Errorf("marshal %s output: %w", sm.Name, err)
		}
		return out, nil
	}
}

// BuildCLIMeta generates CLIMeta from struct tags on the input type.
func BuildCLIMeta(inputType reflect.Type) *ktsp.CLIMeta {
	if inputType == nil {
		return nil
	}
	if inputType.Kind() == reflect.Ptr {
		inputType = inputType.Elem()
	}
	if inputType.Kind() != reflect.Struct {
		return nil
	}

	cliMeta := &ktsp.CLIMeta{}

	for _, field := range cliFields(inputType) {

		desc := field.Tag.Get("description")
		if desc == "" {
			desc = field.Tag.Get("desc")
		}

		cliTag := field.Tag.Get("cli")
		flagType := corereflect.GoTypeToFlagType(field.Type)
		isPath := field.Tag.Get("path") == "cwd"

		if cliTag != "" && cliTag != "-" {
			required := field.Tag.Get("required") == "true"
			if cliTag == "arg" {
				cliMeta.Args = append(cliMeta.Args, ktsp.CLIArg{
					Field: field.Name, Desc: desc, Required: required,
				})
				continue
			}
			parts := strings.SplitN(cliTag, ",", 2)
			flagName := parts[0]
			var short string
			if len(parts) > 1 {
				short = parts[1]
			}
			// The params payload must still land under the json key, even
			// though the flag name comes from the cli tag. Fall back to the
			// flag name when there is no json tag.
			jsonKey := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
			if jsonKey == "" || jsonKey == "-" {
				jsonKey = flagName
			}
			cliMeta.Flags = append(cliMeta.Flags, ktsp.CLIFlag{
				Field: field.Name, Name: flagName, JSONKey: jsonKey, Short: short,
				Desc: desc, Default: field.Tag.Get("default"), Type: flagType, Path: isPath,
			})
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		flagName := strings.SplitN(jsonTag, ",", 2)[0]
		if flagName == "" {
			continue
		}

		cliMeta.Flags = append(cliMeta.Flags, ktsp.CLIFlag{
			Field: field.Name, Name: flagName, JSONKey: flagName, Type: flagType,
			Desc: desc, Default: field.Tag.Get("default"), Path: isPath,
		})
	}

	if len(cliMeta.Args) == 0 && len(cliMeta.Flags) == 0 {
		return nil
	}
	return cliMeta
}

// cliFields is a struct's exported fields, an embedded struct's (without a json
// name) hoisted into it — as encoding/json reads them: a field of the outer
// struct wins over an embedded one of the same name.
func cliFields(t reflect.Type) []reflect.StructField {
	var out, hoisted []reflect.StructField
	taken := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct && jsonName(f) == "" {
			hoisted = append(hoisted, cliFields(f.Type)...)
			continue
		}
		if f.IsExported() {
			out = append(out, f)
			taken[jsonName(f)] = true
		}
	}
	for _, f := range hoisted {
		if n := jsonName(f); n == "" || !taken[n] {
			out = append(out, f)
		}
	}
	return out
}

func jsonName(f reflect.StructField) string {
	return strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
}
