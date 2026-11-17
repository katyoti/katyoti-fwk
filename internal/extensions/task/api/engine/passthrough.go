// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// serviceMethod describes a publishable method discovered via reflection.
type serviceMethod struct {
	Name      string
	InputType reflect.Type // nil = no input (just ctx)
}

// PassthroughMachine wraps a Go interface as a stateless Machine.
// Each event.Type maps to a method name. Step deserializes the payload,
// binds it to the method, and returns a closure that calls it.
// The executor loop runs exactly once per call (Step → action → Done).
type PassthroughMachine struct {
	name    string
	impl    any
	methods map[string]serviceMethod
}

// WrapInterface creates a PassthroughMachine from any Go interface.
// Uses reflection to discover publishable methods:
//
//	func(ctx context.Context, in T) (U, error)
//	func(ctx context.Context) (U, error)
//
// The event.Type selects the method by name (PascalCase).
func WrapInterface(name string, impl any) *PassthroughMachine {
	t := reflect.TypeOf(impl)
	methods := discoverMethods(t)
	return &PassthroughMachine{name: name, impl: impl, methods: methods}
}

func (m *PassthroughMachine) Name() string { return m.name }

func (m *PassthroughMachine) InitialState(_ Event) State {
	return State{Name: "idle"}
}

func (m *PassthroughMachine) Step(ctx context.Context, _ *State, event Event) (ActionFunc, error) {
	method, ok := m.methods[event.Type]
	if !ok {
		return nil, fmt.Errorf("passthrough %q: unknown method %q", m.name, event.Type)
	}

	// Prepare the typed input (Step does type conversion)
	var inputVal reflect.Value
	if method.InputType != nil {
		inputPtr := reflect.New(method.InputType)
		if event.Payload != nil {
			data, err := json.Marshal(event.Payload)
			if err != nil {
				return nil, fmt.Errorf("passthrough %q.%s: marshal payload: %w", m.name, method.Name, err)
			}
			if err := json.Unmarshal(data, inputPtr.Interface()); err != nil {
				return nil, fmt.Errorf("passthrough %q.%s: unmarshal input: %w", m.name, method.Name, err)
			}
		}
		inputVal = inputPtr.Elem()
	}

	implVal := reflect.ValueOf(m.impl)
	methodVal := implVal.MethodByName(method.Name)

	return func(execCtx context.Context) (*StepResult, error) {
		args := []reflect.Value{reflect.ValueOf(execCtx)}
		if method.InputType != nil {
			args = append(args, inputVal)
		}

		results := methodVal.Call(args)

		var errOut error
		if !results[1].IsNil() {
			errOut = results[1].Interface().(error)
		}
		if errOut != nil {
			return nil, errOut
		}

		return &StepResult{
			NextState: "idle",
			Output:    results[0].Interface(),
		}, nil
	}, nil
}

// discoverMethods finds publishable methods on a type via reflection.
// A publishable method has signature:
//
//	func(ctx context.Context, in T) (U, error)
//	func(ctx context.Context) (U, error)
func discoverMethods(t reflect.Type) map[string]serviceMethod {
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	errType := reflect.TypeOf((*error)(nil)).Elem()
	isIface := t.Kind() == reflect.Interface

	methods := make(map[string]serviceMethod)
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		mt := m.Type

		if mt.NumOut() != 2 || !mt.Out(1).Implements(errType) {
			continue
		}

		offset := 0
		if !isIface {
			offset = 1 // skip receiver
		}
		paramCount := mt.NumIn() - offset

		switch paramCount {
		case 1: // func(ctx) (U, error)
			if mt.In(offset).Implements(ctxType) {
				methods[m.Name] = serviceMethod{Name: m.Name}
			}
		case 2: // func(ctx, T) (U, error)
			if mt.In(offset).Implements(ctxType) {
				inType := mt.In(offset + 1)
				if inType.Kind() == reflect.Ptr {
					inType = inType.Elem()
				}
				if inType.Kind() == reflect.Struct {
					methods[m.Name] = serviceMethod{Name: m.Name, InputType: mt.In(offset + 1)}
				}
			}
		}
	}
	return methods
}
