// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"

	codeapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// BusLanguagePack implements codeapi.LanguagePack by dispatching each method to
// the owning language sidecar's `<provider>:/LanguagePack/<method>` KTSP slot.
// It is the sidecar-topology counterpart of the in-process goPack: the binary
// that runs the code methodology (ktw-code) registers one BusLanguagePack per
// language as the named service the methodology resolves via
// `UseNamedService[LanguagePack](ctx, lang)`. The PROVIDER is the language
// discriminator (ktw-gocode = go, a future ktw-tscode = typescript) — there is
// no instance segment in the bus path.
//
// Method slot names come from the shared codeapi.LPMethod* constants (=
// reflect.ToKebab of the Go method names), so this client and the gocode
// publisher reference one source and cannot drift. Mirrors the proven
// manifest/api.BusCatalogReader shape.
type BusLanguagePack struct {
	sender   ktsp.Sender
	provider string // owning sidecar, e.g. "ktw-gocode"
}

// NewBusLanguagePack binds a proxy to the given sidecar provider.
func NewBusLanguagePack(sender ktsp.Sender, provider string) *BusLanguagePack {
	return &BusLanguagePack{sender: sender, provider: provider}
}

func (b *BusLanguagePack) path(method string) string {
	// Kebab bus slot — the URI grammar is kebab-only (LanguagePackSlotType is
	// PascalCase and used only for in-process DI). Must equal the published
	// ServiceDef.Name (LanguagePackBusSlot).
	return b.provider + ":/" + codeapi.LanguagePackBusSlot + "/" + method
}

// busCall is the send+decode helper shared by every method (Go methods can't be
// generic, so this is a free function the methods delegate to).
func busCall[I any, O any](ctx context.Context, b *BusLanguagePack, method string, in I) (O, error) {
	var out O
	raw, err := b.sender.Call(ctx, b.path(method), in)
	if err != nil {
		return out, fmt.Errorf("BusLanguagePack[%s].%s: %w", b.provider, method, err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("BusLanguagePack[%s].%s decode: %w", b.provider, method, err)
	}
	return out, nil
}

func (b *BusLanguagePack) Describe(ctx context.Context, in codeapi.DescribeInput) (codeapi.LanguagePackInfo, error) {
	return busCall[codeapi.DescribeInput, codeapi.LanguagePackInfo](ctx, b, codeapi.LPMethodDescribe, in)
}

func (b *BusLanguagePack) Discover(ctx context.Context, in codeapi.DiscoverInput) (codeapi.DiscoverOutput, error) {
	return busCall[codeapi.DiscoverInput, codeapi.DiscoverOutput](ctx, b, codeapi.LPMethodDiscover, in)
}

func (b *BusLanguagePack) ProposeUCsFromTests(ctx context.Context, in codeapi.ProposeUCsInput) (codeapi.ProposeUCsOutput, error) {
	return busCall[codeapi.ProposeUCsInput, codeapi.ProposeUCsOutput](ctx, b, codeapi.LPMethodProposeUCsFromTests, in)
}

func (b *BusLanguagePack) ReverseEngineerFeatures(ctx context.Context, in codeapi.ReverseEngineerInput) (codeapi.ReverseEngineerOutput, error) {
	return busCall[codeapi.ReverseEngineerInput, codeapi.ReverseEngineerOutput](ctx, b, codeapi.LPMethodReverseEngineerFeatures, in)
}

func (b *BusLanguagePack) RunUnitTest(ctx context.Context, in codeapi.RunUnitTestInput) (codeapi.RunUnitTestOutput, error) {
	return busCall[codeapi.RunUnitTestInput, codeapi.RunUnitTestOutput](ctx, b, codeapi.LPMethodRunUnitTest, in)
}

func (b *BusLanguagePack) GateCheck(ctx context.Context, in codeapi.GateCheckInput) (codeapi.GateCheckOutput, error) {
	return busCall[codeapi.GateCheckInput, codeapi.GateCheckOutput](ctx, b, codeapi.LPMethodGateCheck, in)
}

var _ codeapi.LanguagePack = (*BusLanguagePack)(nil)
