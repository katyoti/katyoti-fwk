// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// PasskeyServiceDef declares the person's passkey surface, served by the
// process serving the universe (ktw-agents): the approval page's ceremonies,
// and the check the guards of the person's acts ask.
var PasskeyServiceDef = meta.ServiceDef{
	Name:    "Passkey",
	BusName: "passkey",
	Short:   "The person's passkey: registered from the phone, approving one act in one tap",
	Long:    `The person registers a passkey once on the universe's approval page (from their phone, over Tailscale); each relayed act can then be approved on the phone with Face ID, the passkey signing a challenge bound to that act. Registering and revoking are the person's own acts: refused from an agent's session. Check answers once whether an approval was given for an act and a person.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"RegistrationBegin":  meta.Method("Start registering a passkey for the person at the approval page's origin: the options for the phone"),
		"RegistrationFinish": meta.Method("Verify the phone's new passkey and store it for the person"),
		"ApprovalBegin":      meta.Method("Start the approval of one act of the person: the options for the phone"),
		"ApprovalFinish":     meta.Method("Verify the phone's signature over the act's challenge (Face ID required)"),
		"Check":              meta.Method("Answer once whether an approval was given on the phone for an act and a person, then spend it"),
		"Status":             {Short: "How many passkeys the person registered", ReadOnly: true},
		"Revoke":             meta.Method("Remove the person's passkeys - the person's own act"),
	},
}

// PasskeyService is the person's passkey.
type PasskeyService interface {
	RegistrationBegin(ctx context.Context, in PasskeyBeginInput) (PasskeyBeginOutput, error)
	RegistrationFinish(ctx context.Context, in PasskeyFinishInput) (PasskeyFinishOutput, error)
	ApprovalBegin(ctx context.Context, in PasskeyBeginInput) (PasskeyBeginOutput, error)
	ApprovalFinish(ctx context.Context, in PasskeyFinishInput) (PasskeyFinishOutput, error)
	Check(ctx context.Context, in PasskeyCheckInput) (PasskeyCheckOutput, error)
	Status(ctx context.Context, in PasskeyStatusInput) (PasskeyStatusOutput, error)
	Revoke(ctx context.Context, in PasskeyStatusInput) (PasskeyStatusOutput, error)
}

// PasskeyBeginInput starts a ceremony for the person at the page's origin;
// Act names the act an approval is of.
type PasskeyBeginInput struct {
	Person string `json:"person" desc:"The person (their profile handle)." required:"true"`
	Act    string `json:"act,omitempty" desc:"An approval's act: its key <verb>:<object id>."`
	Origin string `json:"origin" desc:"The approval page's origin: https://<host> - the relying party." required:"true"`
}

// PasskeyBeginOutput is the ceremony's handle and the options the phone takes.
type PasskeyBeginOutput struct {
	Handle  string          `json:"handle"`
	Options json.RawMessage `json:"options" desc:"navigator.credentials.create or get's options."`
}

// PasskeyFinishInput is the phone's answer to a ceremony.
type PasskeyFinishInput struct {
	Handle   string          `json:"handle" required:"true"`
	Response json.RawMessage `json:"response" desc:"The phone's PublicKeyCredential as JSON." required:"true"`
}

// PasskeyFinishOutput says the ceremony passed.
type PasskeyFinishOutput struct {
	OK bool `json:"ok"`
}

// PasskeyCheckInput asks whether an approval holds for an act and a person.
type PasskeyCheckInput struct {
	Proof  string `json:"proof" desc:"The approval's handle." required:"true"`
	Act    string `json:"act" required:"true"`
	Person string `json:"person" required:"true"`
}

// PasskeyCheckOutput answers once: valid, or why not.
type PasskeyCheckOutput struct {
	Valid  bool   `json:"valid"`
	Reason string `json:"reason,omitempty"`
}

// PasskeyStatusInput names the person.
type PasskeyStatusInput struct {
	Person string `json:"person" required:"true"`
}

// PasskeyStatusOutput is how many passkeys the person has.
type PasskeyStatusOutput struct {
	Passkeys int `json:"passkeys"`
}
