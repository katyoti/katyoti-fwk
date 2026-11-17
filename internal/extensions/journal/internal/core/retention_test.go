// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

func created(t *testing.T, id, module string, at time.Time) cwmapi.EventEnvelope {
	t.Helper()
	p, _ := json.Marshal(cwmapi.ObjectEventPayload{ObjectID: id, Module: module, Type: "note", Action: "created"})
	return cwmapi.EventEnvelope{Topic: cwmapi.ObjectCreatedSubject("t", "u", "w", cwmapi.SubjectToken(id)), Timestamp: at, Payload: p}
}

// TestTheFirstRecordOfADayPrunesThePastRetention: a day older than the
// retention goes, with its entries file; a day inside it stays.
func TestTheFirstRecordOfADayPrunesThePastRetention(t *testing.T) {
	ctx, cwm := cwmapp.New(t, journalcontrib.Pack(""), cwmapp.SystemModules(t))
	s := New(30)
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	old, recent := now.AddDate(0, 0, -45), now.AddDate(0, 0, -5)
	for _, at := range []time.Time{old, recent, now} {
		if err := s.Record(ctx, created(t, "billing.x", "billing", at)); err != nil {
			t.Fatalf("Record %s: %v", at.Format("2006-01-02"), err)
		}
	}
	_, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: api.Module, ID: api.DayID(old)})
	if !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("the day past the retention is still there (err %v)", err)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: api.Module, ID: api.DayID(recent)}); err != nil {
		t.Errorf("a day inside the retention was pruned: %v", err)
	}
	out, err := s.Query(ctx, api.QueryInput{ObjectID: "billing.x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Records) != 2 || !out.Records[0].At.Equal(now) || !out.Records[1].At.Equal(recent) {
		t.Fatalf("records = %+v, want now then recent", out.Records)
	}
	_ = context.Background
}
