// SPDX-License-Identifier: Apache-2.0

// Package approval is the one-tap phone approval: a small page
// server the serving process runs on 127.0.0.1 only, published to the
// person's tailnet with tailscale serve - never funnel. An act waiting on the
// person opens an approval; its card shows what the act is and what approving
// does, Approve runs WebAuthn get over the person's passkey (iam's
// PasskeyService) and Refuse runs nothing. A passkey is registered only
// through a one-time link the person opened themselves.
package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// DefaultPort is the approval page's local port; tailscale serve forwards the
// tailnet's https to it.
const DefaultPort = 47821

// Lifetimes of an approval and of a registration link.
const (
	approvalTTL = 10 * time.Minute
	registerTTL = 10 * time.Minute
)

// approval is one act waiting on the person's phone.
type approval struct {
	person, act, title, what string
	expires                  time.Time
	state                    string
	ceremony                 string // the passkey approval's handle: the proof once approved
}

// registration is a one-time link to register the person's passkey.
type registration struct {
	person   string
	expires  time.Time
	ceremony string
}

// Server is the approval page and its approvals.
type Server struct {
	Port     int
	Passkeys profileapi.PasskeyService
	// Origin is the page's https origin on the tailnet (Tailscale); an error
	// when the page is not published.
	Origin func(ctx context.Context) (string, error)
	Now    func() time.Time

	mu        sync.Mutex
	approvals map[string]*approval
	regs      map[string]*registration
}

// New is a page server on port over the person's passkeys.
func New(port int, passkeys profileapi.PasskeyService, origin func(context.Context) (string, error)) *Server {
	return &Server{Port: port, Passkeys: passkeys, Origin: origin, Now: time.Now,
		approvals: map[string]*approval{}, regs: map[string]*registration{}}
}

func handle() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Listen binds the page on 127.0.0.1 only: the tailnet reaches it through
// tailscale serve, nothing else does.
func (s *Server) Listen() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port))
}

// Serve answers the page until ctx ends.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	// The pages read the application's services: each request carries ctx.
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Open opens the approval of one act: its handle and its card's link.
func (s *Server) Open(ctx context.Context, in api.ApprovalOpenInput) (api.ApprovalOpenOutput, error) {
	if in.Person == "" || in.Act == "" {
		return api.ApprovalOpenOutput{}, errors.New("approval: person and act required")
	}
	origin, err := s.Origin(ctx)
	if err != nil {
		return api.ApprovalOpenOutput{}, fmt.Errorf("approval: the approval page is not published: %v", err)
	}
	h := handle()
	s.mu.Lock()
	s.approvals[h] = &approval{person: in.Person, act: in.Act, title: in.Title, what: in.What, expires: s.Now().Add(approvalTTL), state: api.ApprovalPending}
	s.mu.Unlock()
	return api.ApprovalOpenOutput{Handle: h, Link: origin + "/a/" + h}, nil
}

// Status says where an approval stands, spending nothing.
func (s *Server) Status(in api.ApprovalStatusInput) (api.ApprovalStatusOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.approvals[in.Handle]
	if a == nil {
		return api.ApprovalStatusOutput{}, errors.New("approval: no such approval")
	}
	if a.state == api.ApprovalPending && s.Now().After(a.expires) {
		a.state = api.ApprovalRefused
	}
	out := api.ApprovalStatusOutput{State: a.state, Act: a.act}
	if a.state == api.ApprovalApproved {
		out.Proof = a.ceremony
	}
	return out, nil
}

// RegisterOpen opens a one-time passkey registration for the person.
func (s *Server) RegisterOpen(ctx context.Context, person string) (api.ApprovalRegisterOutput, error) {
	origin, err := s.Origin(ctx)
	if err != nil {
		return api.ApprovalRegisterOutput{}, fmt.Errorf("approval: the approval page is not published: %v", err)
	}
	h := handle()
	exp := s.Now().Add(registerTTL)
	s.mu.Lock()
	s.regs[h] = &registration{person: person, expires: exp}
	s.mu.Unlock()
	return api.ApprovalRegisterOutput{Link: origin + "/register/" + h, Expires: exp.UTC().Format(time.RFC3339)}, nil
}

// Handler is the page's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /a/{h}", s.card)
	mux.HandleFunc("POST /a/{h}/begin", s.approveBegin)
	mux.HandleFunc("POST /a/{h}/finish", s.approveFinish)
	mux.HandleFunc("POST /a/{h}/refuse", s.refuse)
	mux.HandleFunc("GET /register/{h}", s.registerPage)
	mux.HandleFunc("POST /register/{h}/begin", s.registerBegin)
	mux.HandleFunc("POST /register/{h}/finish", s.registerFinish)
	s.glanceRoutes(mux)
	return mux
}

// pending is the approval a request names, still waiting.
func (s *Server) pending(h string) (*approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.approvals[h]
	switch {
	case a == nil:
		return nil, errors.New("no such approval")
	case a.state != api.ApprovalPending:
		return nil, fmt.Errorf("this approval is already %s", a.state)
	case s.Now().After(a.expires):
		a.state = api.ApprovalRefused
		return nil, errors.New("this approval expired")
	}
	return a, nil
}

func (s *Server) card(w http.ResponseWriter, r *http.Request) {
	a, err := s.pending(r.PathValue("h"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	page(w, "Approve", a.title, a.what, a.act, "/a/"+r.PathValue("h"), true)
}

func (s *Server) approveBegin(w http.ResponseWriter, r *http.Request) {
	a, err := s.pending(r.PathValue("h"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	origin, err := s.Origin(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	out, err := s.Passkeys.ApprovalBegin(r.Context(), profileapi.PasskeyBeginInput{Person: a.person, Act: a.act, Origin: origin})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	a.ceremony = out.Handle
	s.mu.Unlock()
	writeJSON(w, out.Options)
}

func (s *Server) approveFinish(w http.ResponseWriter, r *http.Request) {
	a, err := s.pending(r.PathValue("h"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	s.mu.Lock()
	ceremony := a.ceremony
	s.mu.Unlock()
	if ceremony == "" {
		http.Error(w, "no approval was begun", http.StatusBadRequest)
		return
	}
	if _, err := s.Passkeys.ApprovalFinish(r.Context(), profileapi.PasskeyFinishInput{Handle: ceremony, Response: body}); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	s.mu.Lock()
	a.state = api.ApprovalApproved
	s.mu.Unlock()
	writeJSON(w, json.RawMessage(`{"state":"approved"}`))
}

func (s *Server) refuse(w http.ResponseWriter, r *http.Request) {
	a, err := s.pending(r.PathValue("h"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	s.mu.Lock()
	a.state = api.ApprovalRefused
	s.mu.Unlock()
	writeJSON(w, json.RawMessage(`{"state":"refused"}`))
}

// registration is the waiting registration a request names.
func (s *Server) registration(h string) (*registration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.regs[h]
	if g == nil || s.Now().After(g.expires) {
		delete(s.regs, h)
		return nil, errors.New("this registration link is not valid: open a new one (katyoti profile passkey register)")
	}
	return g, nil
}

func (s *Server) registerPage(w http.ResponseWriter, r *http.Request) {
	g, err := s.registration(r.PathValue("h"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	page(w, "Register", "Register your passkey", "This phone's passkey (Face ID or fingerprint) will approve your acts in KaTyoti.", g.person, "/register/"+r.PathValue("h"), false)
}

func (s *Server) registerBegin(w http.ResponseWriter, r *http.Request) {
	g, err := s.registration(r.PathValue("h"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	origin, err := s.Origin(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	out, err := s.Passkeys.RegistrationBegin(r.Context(), profileapi.PasskeyBeginInput{Person: g.person, Origin: origin})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	g.ceremony = out.Handle
	s.mu.Unlock()
	writeJSON(w, out.Options)
}

func (s *Server) registerFinish(w http.ResponseWriter, r *http.Request) {
	h := r.PathValue("h")
	g, err := s.registration(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if _, err := s.Passkeys.RegistrationFinish(r.Context(), profileapi.PasskeyFinishInput{Handle: g.ceremony, Response: body}); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	s.mu.Lock()
	delete(s.regs, h) // one time
	s.mu.Unlock()
	writeJSON(w, json.RawMessage(`{"state":"registered"}`))
}

func writeJSON(w http.ResponseWriter, raw json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// page writes the card: a title, what it does, and the buttons whose script
// runs WebAuthn (get to approve, create to register).
func page(w http.ResponseWriter, action, title, what, about, base string, approve bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.Execute(w, map[string]any{"Action": action, "Title": title, "What": what, "About": about, "Base": base, "Approve": approve})
}

var pageTmpl = template.Must(template.New("page").Parse(strings.TrimSpace(`
<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>KaTyoti - {{.Action}}</title>
<style>body{font-family:system-ui,sans-serif;margin:24px;max-width:520px}h1{font-size:1.3em}.about{color:#666;font-size:.9em}
button{font-size:1.1em;padding:14px 22px;margin:8px 8px 0 0;border-radius:10px;border:0}#ok{background:#1a7f37;color:#fff}#no{background:#ddd}#msg{margin-top:16px}</style></head>
<body><h1>{{.Title}}</h1><p>{{.What}}</p><p class="about">{{.About}}</p>
<button id="ok">{{.Action}}</button>{{if .Approve}}<button id="no">Refuse</button>{{end}}<p id="msg"></p>
<script>
const base={{.Base}};const msg=t=>document.getElementById('msg').textContent=t;
const b64=b=>btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/,'');
const unb64=s=>Uint8Array.from(atob(s.replace(/-/g,'+').replace(/_/g,'/')),c=>c.charCodeAt(0));
async function post(p,body){const r=await fetch(base+p,{method:'POST',headers:{'Content-Type':'application/json'},body:body?JSON.stringify(body):''});if(!r.ok)throw new Error(await r.text());return r.json();}
document.getElementById('ok').onclick=async()=>{try{
 const o=(await post('/begin')).publicKey;o.challenge=unb64(o.challenge);
 {{if .Approve}}(o.allowCredentials||[]).forEach(c=>c.id=unb64(c.id));
 const c=await navigator.credentials.get({publicKey:o});
 await post('/finish',{id:c.id,rawId:b64(c.rawId),type:c.type,response:{authenticatorData:b64(c.response.authenticatorData),clientDataJSON:b64(c.response.clientDataJSON),signature:b64(c.response.signature),userHandle:c.response.userHandle?b64(c.response.userHandle):null}});
 msg('Approved.');{{else}}o.user.id=unb64(o.user.id);(o.excludeCredentials||[]).forEach(c=>c.id=unb64(c.id));
 const c=await navigator.credentials.create({publicKey:o});
 await post('/finish',{id:c.id,rawId:b64(c.rawId),type:c.type,response:{attestationObject:b64(c.response.attestationObject),clientDataJSON:b64(c.response.clientDataJSON)}});
 msg('Your passkey is registered.');{{end}}
}catch(e){msg('Not done: '+e.message)}};
{{if .Approve}}document.getElementById('no').onclick=async()=>{try{await post('/refuse');msg('Refused.')}catch(e){msg(e.message)}};{{end}}
</script></body></html>`)))
