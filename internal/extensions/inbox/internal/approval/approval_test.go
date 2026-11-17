// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person/passkeytest"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person/passkeytest/passkeysvc"
)

const origin = "https://pc.tailnet.test"
const rpID = "pc.tailnet.test"

func post(t *testing.T, url string, body []byte) (int, []byte) {
	t.Helper()
	r, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, b
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	r, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

// world is a page server over the real passkey service, an http test server
// in front of it, and the person's phone with its passkey registered through
// a one-time link.
func world(t *testing.T) (*Server, profileapi.PasskeyService, *httptest.Server, *passkeytest.Phone, *time.Time) {
	t.Helper()
	pk := passkeysvc.Service(t.TempDir())
	s := New(0, pk, func(context.Context) (string, error) { return origin, nil })
	now := time.Now()
	s.Now = func() time.Time { return now }
	hs := httptest.NewServer(s.Handler())
	t.Cleanup(hs.Close)
	phone := passkeytest.NewPhone(t, rpID)

	reg, err := s.RegisterOpen(context.Background(), "jdoe")
	if err != nil || !strings.HasPrefix(reg.Link, origin+"/register/") {
		t.Fatalf("register link: %+v %v", reg, err)
	}
	path := strings.TrimPrefix(reg.Link, origin)
	if code, body := get(t, hs.URL+path); code != 200 || !strings.Contains(body, "Register your passkey") {
		t.Fatalf("registration page: %d %s", code, body)
	}
	code, opts := post(t, hs.URL+path+"/begin", nil)
	if code != 200 {
		t.Fatalf("registration begin: %d %s", code, opts)
	}
	if code, b := post(t, hs.URL+path+"/finish", phone.Create(t, origin, opts)); code != 200 {
		t.Fatalf("registration finish: %d %s", code, b)
	}
	if code, _ := get(t, hs.URL+path); code != http.StatusGone {
		t.Fatalf("the registration link served twice: %d; want gone", code)
	}
	return s, pk, hs, phone, &now
}

// approveWith opens an approval of act and has phone answer its card.
func approveWith(t *testing.T, s *Server, hs *httptest.Server, phone *passkeytest.Phone, act string) (string, int) {
	t.Helper()
	open, err := s.Open(context.Background(), api.ApprovalOpenInput{Person: "jdoe", Act: act, Title: "Decide: which colour?", What: "Blue is chosen."})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.TrimPrefix(open.Link, origin)
	if code, body := get(t, hs.URL+path); code != 200 || !strings.Contains(body, "Decide: which colour?") || !strings.Contains(body, "Blue is chosen.") {
		t.Fatalf("the card: %d %s", code, body)
	}
	code, opts := post(t, hs.URL+path+"/begin", nil)
	if code != 200 {
		t.Fatalf("approval begin: %d %s", code, opts)
	}
	code, _ = post(t, hs.URL+path+"/finish", phone.Get(t, origin, opts))
	return open.Handle, code
}

// THE PHONE APPROVAL: an act's card, Approve with the person's
// passkey and Face ID verified once for that act, Refuse; and nothing
// approved without the person.
func TestThePhoneApproval(t *testing.T) {
	s, pk, hs, phone, now := world(t)
	ctx := context.Background()

	h, code := approveWith(t, s, hs, phone, "decide:m.OD-1")
	if code != 200 {
		t.Fatalf("Approve with Face ID: %d", code)
	}
	st, _ := s.Status(api.ApprovalStatusInput{Handle: h})
	if st.State != api.ApprovalApproved || st.Proof == "" || st.Act != "decide:m.OD-1" {
		t.Fatalf("status: %+v; want approved with its proof", st)
	}
	if c, _ := pk.Check(ctx, profileapi.PasskeyCheckInput{Proof: st.Proof, Act: "decide:m.OD-1", Person: "jdoe"}); !c.Valid {
		t.Fatalf("the act's guard check: %+v; want valid", c)
	}
	if c, _ := pk.Check(ctx, profileapi.PasskeyCheckInput{Proof: st.Proof, Act: "decide:m.OD-1", Person: "jdoe"}); c.Valid {
		t.Fatal("the proof checked twice")
	}
	if code, _ := post(t, hs.URL+"/a/"+h+"/finish", nil); code != http.StatusGone {
		t.Fatalf("an approved card answered again: %d; want gone", code)
	}

	// Nothing approved without the person.
	phone.UV = false
	if h, code := approveWith(t, s, hs, phone, "decide:m.OD-2"); code != http.StatusForbidden {
		t.Fatalf("no Face ID: %d; want forbidden", code)
	} else if st, _ := s.Status(api.ApprovalStatusInput{Handle: h}); st.State != api.ApprovalPending {
		t.Fatalf("no Face ID left it %s; want pending", st.State)
	}
	phone.UV = true
	if _, code := approveWith(t, s, hs, passkeytest.NewPhone(t, rpID), "decide:m.OD-3"); code != http.StatusForbidden {
		t.Fatalf("another passkey: %d; want forbidden", code)
	}
	if _, code := approveWith(t, s, hs, passkeytest.NewPhone(t, "evil.example"), "decide:m.OD-4"); code != http.StatusForbidden {
		t.Fatalf("another relying party: %d; want forbidden", code)
	}
	if code, _ := get(t, hs.URL+"/a/no-such-handle"); code != http.StatusGone {
		t.Fatalf("an unknown card: %d; want gone", code)
	}

	// Refuse.
	open, _ := s.Open(ctx, api.ApprovalOpenInput{Person: "jdoe", Act: "hold:k", Title: "Hold"})
	if code, _ := post(t, hs.URL+"/a/"+open.Handle+"/refuse", nil); code != 200 {
		t.Fatalf("refuse: %d", code)
	}
	if st, _ := s.Status(api.ApprovalStatusInput{Handle: open.Handle}); st.State != api.ApprovalRefused || st.Proof != "" {
		t.Fatalf("refused: %+v", st)
	}

	// Expired.
	open, _ = s.Open(ctx, api.ApprovalOpenInput{Person: "jdoe", Act: "waive:r", Title: "Waive"})
	*now = now.Add(approvalTTL + time.Minute)
	if code, _ := get(t, hs.URL+"/a/"+open.Handle); code != http.StatusGone {
		t.Fatalf("an expired card: %d; want gone", code)
	}
	if st, _ := s.Status(api.ApprovalStatusInput{Handle: open.Handle}); st.State != api.ApprovalRefused {
		t.Fatalf("expired: %s; want refused", st.State)
	}

	// The person's own acts: never from an agent's session.
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	svc := Service{S: s, TS: &Tailscale{Run: func(context.Context, ...string) ([]byte, error) { return nil, nil }}}
	if _, err := svc.RegisterOpen(agent, api.ApprovalRegisterInput{Person: "jdoe"}); err == nil {
		t.Fatal("an agent's session opened a passkey registration")
	}
	if _, err := svc.Publish(agent, api.ApprovalPublishInput{}); err == nil {
		t.Fatal("an agent's session published the page")
	}
}

// TAILNET ONLY: the page binds 127.0.0.1; it is published with
// tailscale serve, never funnel; its origin is the PC's tailnet name over
// https; a page funnel exposes is not published.
func TestTheApprovalPageIsTailnetOnly(t *testing.T) {
	s := New(0, nil, nil)
	l, err := s.Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if !strings.HasPrefix(l.Addr().String(), "127.0.0.1:") {
		t.Fatalf("the page listens on %s; want 127.0.0.1 only", l.Addr())
	}
	args := strings.Join(PublishArgs(DefaultPort), " ")
	if !strings.HasPrefix(args, "serve ") || strings.Contains(args, "funnel") || !strings.Contains(args, "http://127.0.0.1:") {
		t.Fatalf("publish runs tailscale %s; want serve to 127.0.0.1, never funnel", args)
	}

	funnel := false
	ts := &Tailscale{Run: func(_ context.Context, a ...string) ([]byte, error) {
		switch strings.Join(a, " ") {
		case "status --json":
			return []byte(`{"Self":{"DNSName":"pc.tailnet.test."}}`), nil
		case "serve status --json":
			b, _ := json.Marshal(map[string]any{
				"Web":         map[string]any{"pc.tailnet.test:443": map[string]any{"Handlers": map[string]any{"/": map[string]any{"Proxy": "http://127.0.0.1:47821"}}}},
				"AllowFunnel": map[string]bool{"pc.tailnet.test:443": funnel},
			})
			return b, nil
		}
		return nil, nil
	}}
	if o, err := ts.Origin(context.Background()); err != nil || o != origin {
		t.Fatalf("origin: %q %v; want %s", o, err, origin)
	}
	svc := Service{S: New(DefaultPort, nil, ts.Origin), TS: ts}
	st, _ := svc.PageStatus(context.Background(), api.ApprovalPublishInput{})
	if !st.Published || st.Funnel || st.Origin != origin {
		t.Fatalf("served on the tailnet: %+v; want published", st)
	}
	funnel = true
	st, _ = svc.PageStatus(context.Background(), api.ApprovalPublishInput{})
	if st.Published || !st.Funnel || !strings.Contains(st.Note, "funnel") {
		t.Fatalf("exposed by funnel: %+v; want not published, saying why", st)
	}
}
