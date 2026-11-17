// SPDX-License-Identifier: Apache-2.0

package passkey

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// Service is the PasskeyService over a Store.
type Service struct{ S *Store }

var _ api.PasskeyService = Service{}

// mine refuses the person's own passkey acts from an agent's session.
func mine(ctx context.Context, what string) error {
	if c, ok := cause.CauseFrom(ctx); ok && c.Kind == cause.KindAgent {
		return fmt.Errorf("passkey.%s: the person registers and revokes their passkeys themselves, on their phone - not from an agent's session (%s)", what, c.ID)
	}
	return nil
}

// RegistrationBegin starts registering a passkey: the person's own act.
func (s Service) RegistrationBegin(ctx context.Context, in api.PasskeyBeginInput) (api.PasskeyBeginOutput, error) {
	if err := mine(ctx, "RegistrationBegin"); err != nil {
		return api.PasskeyBeginOutput{}, err
	}
	h, o, err := s.S.BeginRegistration(in.Person, in.Origin)
	return api.PasskeyBeginOutput{Handle: h, Options: o}, err
}

// RegistrationFinish verifies and stores the phone's new passkey.
func (s Service) RegistrationFinish(ctx context.Context, in api.PasskeyFinishInput) (api.PasskeyFinishOutput, error) {
	if err := mine(ctx, "RegistrationFinish"); err != nil {
		return api.PasskeyFinishOutput{}, err
	}
	err := s.S.FinishRegistration(in.Handle, in.Response)
	return api.PasskeyFinishOutput{OK: err == nil}, err
}

// ApprovalBegin starts the approval of one act.
func (s Service) ApprovalBegin(_ context.Context, in api.PasskeyBeginInput) (api.PasskeyBeginOutput, error) {
	h, o, err := s.S.BeginApproval(in.Person, in.Act, in.Origin)
	return api.PasskeyBeginOutput{Handle: h, Options: o}, err
}

// ApprovalFinish needs no guard: only the person's passkey signs a valid
// answer, whoever carries it.
func (s Service) ApprovalFinish(_ context.Context, in api.PasskeyFinishInput) (api.PasskeyFinishOutput, error) {
	err := s.S.FinishApproval(in.Handle, in.Response)
	return api.PasskeyFinishOutput{OK: err == nil}, err
}

// Check answers once whether the approval holds for the act and the person.
func (s Service) Check(_ context.Context, in api.PasskeyCheckInput) (api.PasskeyCheckOutput, error) {
	if err := s.S.Check(in.Proof, in.Act, in.Person); err != nil {
		return api.PasskeyCheckOutput{Reason: err.Error()}, nil
	}
	return api.PasskeyCheckOutput{Valid: true}, nil
}

// Status is how many passkeys the person registered.
func (s Service) Status(_ context.Context, in api.PasskeyStatusInput) (api.PasskeyStatusOutput, error) {
	return api.PasskeyStatusOutput{Passkeys: s.S.Count(in.Person)}, nil
}

// Revoke removes the person's passkeys: the person's own act.
func (s Service) Revoke(ctx context.Context, in api.PasskeyStatusInput) (api.PasskeyStatusOutput, error) {
	if err := mine(ctx, "Revoke"); err != nil {
		return api.PasskeyStatusOutput{}, err
	}
	err := s.S.Revoke(in.Person)
	return api.PasskeyStatusOutput{Passkeys: s.S.Count(in.Person)}, err
}
