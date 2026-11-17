// SPDX-License-Identifier: Apache-2.0

package sidecar

import (
	"context"
	"testing"

	scinternal "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

type noLLMIn struct {
	ID string `json:"id"`
}

type noLLMOut struct {
	OK bool `json:"ok"`
}

type noLLMService struct{}

func (noLLMService) Look(context.Context, noLLMIn) (noLLMOut, error) { return noLLMOut{OK: true}, nil }
func (noLLMService) Confirm(context.Context, noLLMIn) (noLLMOut, error) {
	return noLLMOut{OK: true}, nil
}

// A method declared NoLLM (a person's act, e.g. gates Confirm) is not on the
// LLM tool surface — `llm tools list|call` and `sc tools list` read this
// registry — while it stays published on the KTSP endpoint.
func TestNoLLMMethodIsNotAToolButStaysOnKTSP(t *testing.T) {
	defs := map[string]meta.MethodDef{
		"Look":    {Short: "Look", ReadOnly: true},
		"Confirm": {Short: "A person confirms", NoLLM: true},
	}
	svc := noLLMService{}
	slot := ktslocal.BuildKTSPSlotLazy("thing", ktsp.DefaultInstance, "Things", svc, nil, defs,
		func() (any, error) { return svc, nil })
	ep := ktsp.NewEndpoint("ktw-x", "x", "1.0.0")
	ep.AddSlot(slot)

	reg := scinternal.NewToolRegistry()
	populateRegistryFromEndpoint(reg, ep, "ktw-x")

	if _, ok := reg.Get("ktw-x:thing/look"); !ok {
		t.Fatalf("look is not a tool: %+v", reg.All())
	}
	if _, ok := reg.Get("ktw-x:thing/confirm"); ok {
		t.Error("confirm (NoLLM) is listed as an LLM tool")
	}
	onKTSP := false
	for _, s := range ep.Info().Slots {
		for _, m := range s.Methods {
			if m.Name == "confirm" {
				onKTSP = true
			}
		}
	}
	if !onKTSP {
		t.Error("confirm is no longer published on KTSP")
	}
}
