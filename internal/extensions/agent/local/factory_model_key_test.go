// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	iprofile "github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/profile"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// noKeyProfiles resolves a profile whose model's key is not set: creating the
// model is refused as core.UnsetKeys refuses it.
type noKeyProfiles struct{}

func (noKeyProfiles) ResolveProfile(string) (iprofile.AgentProfile, error) {
	return iprofile.AgentProfile{Model: iprofile.ProfileModel{Name: "foundry-haiku"}}, nil
}
func (noKeyProfiles) CreateModelFromProfile(iprofile.AgentProfile, map[string]any) (llm.Model, error) {
	return nil, errors.New("anthropic: key ANTHROPIC_FOUNDRY_API_KEY, ANTHROPIC_FOUNDRY_BASE_URL is not set — import it (vault, keys file or config-set), then restart the universe: its programs read their keys when they start")
}
func (noKeyProfiles) DefaultProfileName() string                  { return "fast" }
func (noKeyProfiles) ModelCallLogging(iprofile.AgentProfile) bool { return false }

// : the person's case - the keys not imported - fails at the
// model's creation, before any call: the run's refusal carries the same key
// as a failed call, agent.modelKeyMissing with the model, so the chat says it
// in the person's language with the next step.
func TestAModelWhoseKeyIsNotSetIsRefusedWithItsKey(t *testing.T) {
	_, err := NewAgentMachineFactory(noKeyProfiles{}, nil).CreateMachine(context.Background(), &api.AgentRunRequest{Prompt: "go"})
	if err == nil {
		t.Fatal("a model with no key was created")
	}
	m, ok := usermsg.Of(err)
	if !ok || m.Key != "agent.modelKeyMissing" || m.Params["model"] != "foundry-haiku" {
		t.Fatalf("the refusal %q carries %+v %v; want agent.modelKeyMissing with its model", err, m, ok)
	}
}
