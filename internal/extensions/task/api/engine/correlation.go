// SPDX-License-Identifier: Apache-2.0

package engine

// Correlation is the universal identity envelope carried by every typed
// session event: it lets
// any consumer cross-link the two session lenses — the task tree (what ran,
// with which input/output) and the team view (who collaborated, what they
// said, what state the work is in). Click a member → their runs; click a
// run → its team context.
//
// Stamping is LAYERED — each layer fills only what it knows, never
// overwriting a non-empty field:
//   - the executor fills RunID (it already stamps the emitter);
//   - the spawner fills RunID + ParentRunID on child lifecycle events;
//   - orchestration fills TeamID + MemberID on team/mailbox events;
//   - SessionID is filled where the emitter knows it; for journal entries
//     it is also implicit (the journal file lives in the session dir);
//   - UserID is RESERVED — plumbed now, empty until user auth lands, so
//     journals are forward-compatible.
type Correlation struct {
	SessionID   string `json:"sessionId,omitzero"`
	UserID      string `json:"userId,omitzero"` // RESERVED for future auth
	RunID       string `json:"runId,omitzero"`
	ParentRunID string `json:"parentRunId,omitzero"`
	TeamID      string `json:"teamId,omitzero"`
	MemberID    string `json:"memberId,omitzero"` // persona name, scoped by TeamID
}

// HasCorrelation is the embeddable carrier — typed events embed it to gain
// the envelope and satisfy CorrelationCarrier.
type HasCorrelation struct {
	Correlation Correlation `json:"correlation,omitzero"`
}

// CorrelationRef exposes the envelope for layered stamping.
func (h *HasCorrelation) CorrelationRef() *Correlation { return &h.Correlation }

// CorrelationCarrier is implemented (via HasCorrelation embedding) by every
// typed event that participates in the envelope.
type CorrelationCarrier interface {
	CorrelationRef() *Correlation
}

// FillCorrelation merges src into the payload's envelope, filling ONLY
// empty fields (layered stamping: earlier, closer-to-the-source layers
// win). No-op when payload doesn't carry an envelope.
func FillCorrelation(payload any, src Correlation) {
	carrier, ok := payload.(CorrelationCarrier)
	if !ok {
		return
	}
	c := carrier.CorrelationRef()
	if c.SessionID == "" {
		c.SessionID = src.SessionID
	}
	if c.UserID == "" {
		c.UserID = src.UserID
	}
	if c.RunID == "" {
		c.RunID = src.RunID
	}
	if c.ParentRunID == "" {
		c.ParentRunID = src.ParentRunID
	}
	if c.TeamID == "" {
		c.TeamID = src.TeamID
	}
	if c.MemberID == "" {
		c.MemberID = src.MemberID
	}
}
