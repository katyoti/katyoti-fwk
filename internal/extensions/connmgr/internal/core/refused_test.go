// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
)

// : a member the hub refuses is never started - Acquire answers
// ErrMemberRefused with the reason, whoever calls one of its slots.
func TestARefusedMemberIsNotStarted(t *testing.T) {
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "ktw-x", Transport: "stdio", Binary: "does-not-exist.exe",
		Refused: "ktw-x reports version v1.1.0 and the universe pins v1.0.0"}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	_, err = m.Acquire(context.Background(), "ktw-x")
	if !errors.Is(err, cmapi.ErrMemberRefused) || !strings.Contains(err.Error(), "pins v1.0.0") {
		t.Fatalf("Acquire = %v; want ErrMemberRefused naming the versions", err)
	}
	if st, _ := m.Status("ktw-x"); st.State == cmapi.ConnSpawning || st.State == cmapi.ConnReady {
		t.Fatalf("a refused member was started: %+v", st)
	}
}
