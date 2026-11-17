// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// READ-ONLY PAGES ON THE PHONE (decision): beside
// its one-act cards the page server shows, read-only, the delivery overview
// (/overview) and what waits on the person (/todo), over the same tailnet-only
// listener. Nothing on them acts: an item asking an act links to its one-tap
// card when one is open.

// glanceRoutes adds the read-only pages to the routes.
func (s *Server) glanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /overview", s.overviewPage)
	mux.HandleFunc("GET /todo", s.todoPage)
}

// streamGlance is one stream on the overview: its next planned release and
// the increments it ships, then the stream's other open increments.
type streamGlance struct {
	Name, Next, NextDate string
	Awaiting             int
	Ships, Others        []incGlance
}

type incGlance struct {
	Title, Priority, State string
	Done, Total, Awaiting  int
}

// overview reads every stream's delivery from planification.
func overview(ctx context.Context) ([]streamGlance, error) {
	streams, err := application.UseService[planapi.StreamService](ctx)
	if err != nil {
		return nil, err
	}
	rels, err := application.UseService[planapi.ReleaseService](ctx)
	if err != nil {
		return nil, err
	}
	incs, err := application.UseService[planapi.IncrementService](ctx)
	if err != nil {
		return nil, err
	}
	sl, err := streams.List(ctx, planapi.ListStreamsInput{})
	if err != nil {
		return nil, err
	}
	rl, err := rels.List(ctx, planapi.ListInput{})
	if err != nil {
		return nil, err
	}
	il, err := incs.List(ctx, planapi.ListIncrementsInput{State: planapi.IncrementOpen})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(sl.Streams, func(i, j int) bool { return sl.Streams[i].Order < sl.Streams[j].Order })
	var out []streamGlance
	for _, st := range sl.Streams {
		g := streamGlance{Name: st.Name}
		var mine []planapi.Release
		for _, r := range rl.Releases {
			if r.Stream == st.ID {
				mine = append(mine, r)
			}
		}
		next, ok := planapi.NextPlanned(mine)
		if ok {
			g.Next, g.NextDate = next.Name, next.Date
			if next.Version != "" {
				g.Next = planapi.ReleaseLabel(next.Version, next.Name)
			}
		}
		var open []planapi.Increment
		for _, i := range il.Increments {
			if i.Stream == st.ID || (ok && i.ShipsIn == next.ID) {
				open = append(open, i)
			}
		}
		sort.SliceStable(open, func(a, b int) bool { return open[a].Priority < open[b].Priority })
		for _, i := range open {
			// Where it stands: planification's one standing, in its words.
			standing := planapi.IncrementStanding(planapi.IncrementStandingInput{Increment: i})
			ig := incGlance{Title: i.Title, Priority: i.Priority}
			if s, err := incs.Summary(ctx, planapi.GetInput{ID: i.ID}); err == nil {
				ig.Done, ig.Total, ig.Awaiting = s.Done, s.Total, s.Awaiting
				g.Awaiting += s.Awaiting
				if s.Standing != "" {
					standing = s.Standing
				}
			}
			ig.State, _ = planapi.StandingLabel(standing, false)
			if ok && i.ShipsIn == next.ID {
				g.Ships = append(g.Ships, ig)
			} else {
				g.Others = append(g.Others, ig)
			}
		}
		out = append(out, g)
	}
	return out, nil
}

// todoGroup is what waits on the person for one release and increment.
type todoGroup struct {
	Place string
	Items []todoGlance
}

type todoGlance struct {
	Action, Title, Asks, Card string
}

// todo reads « À faire » from planification, grouped by release then
// increment, each item linked to its card when an approval of it is open.
func (s *Server) todo(ctx context.Context) ([]todoGroup, error) {
	views, err := application.UseService[planapi.ViewService](ctx)
	if err != nil {
		return nil, err
	}
	q, err := views.PersonQueue(ctx, planapi.PersonQueueInput{})
	if err != nil {
		return nil, err
	}
	var groups []todoGroup
	at := map[string]int{}
	for _, it := range q.Items {
		if it.Held != nil || it.Action == planapi.ActionNotice {
			continue
		}
		rel, inc := it.ReleaseTitle, it.IncrementTitle
		if rel == "" {
			rel = "In no release"
		}
		if inc == "" {
			inc = "outside any increment"
		}
		place := rel + " › " + inc
		i, ok := at[place]
		if !ok {
			i = len(groups)
			at[place] = i
			groups = append(groups, todoGroup{Place: place})
		}
		groups[i].Items = append(groups[i].Items, todoGlance{Action: it.Action, Title: it.Title, Asks: it.Asks, Card: s.cardOf(it.Object, it.Request)})
	}
	sort.SliceStable(groups, func(a, b int) bool {
		na, nb := strings.HasPrefix(groups[a].Place, "In no release"), strings.HasPrefix(groups[b].Place, "In no release")
		if na != nb {
			return nb
		}
		return groups[a].Place < groups[b].Place
	})
	return groups, nil
}

// cardOf is the link of a pending approval whose act names one of ids.
func (s *Server) cardOf(ids ...string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, a := range s.approvals {
		if a.state != api.ApprovalPending || s.Now().After(a.expires) {
			continue
		}
		for _, id := range ids {
			if id != "" && (strings.Contains(a.act, id) || strings.Contains(a.title, id)) {
				return "/a/" + h
			}
		}
	}
	return ""
}

func (s *Server) overviewPage(w http.ResponseWriter, r *http.Request) {
	streams, err := overview(r.Context())
	if err != nil {
		http.Error(w, "the overview could not be read: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	glance(w, map[string]any{"Title": "Delivery", "Streams": streams})
}

func (s *Server) todoPage(w http.ResponseWriter, r *http.Request) {
	groups, err := s.todo(r.Context())
	if err != nil {
		http.Error(w, "« À faire » could not be read: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	glance(w, map[string]any{"Title": "To do", "Groups": groups, "Todo": true})
}

func glance(w http.ResponseWriter, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = glanceTmpl.Execute(w, data)
}

var glanceTmpl = template.Must(template.New("glance").Parse(strings.TrimSpace(`
<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>KaTyoti - {{.Title}}</title>
<style>body{font-family:system-ui,sans-serif;margin:16px;max-width:620px}h1{font-size:1.3em}h2{font-size:1.05em;margin:18px 0 6px}
nav a{margin-right:14px}.dim{color:#666;font-size:.9em}.item{border:1px solid #ddd;border-radius:10px;padding:10px;margin:6px 0}
.badge{display:inline-block;font-size:.75em;padding:1px 7px;border-radius:9px;background:#eee;margin-right:6px}.wait{background:#efe1fb}</style></head>
<body><nav><a href="/overview">Delivery</a><a href="/todo">To do</a></nav><h1>{{.Title}}</h1>
{{if .Todo}}{{if not .Groups}}<p class="dim">Nothing waits on you.</p>{{end}}
{{range .Groups}}<h2>{{.Place}}</h2>{{range .Items}}<div class="item"><span class="badge">{{.Action}}</span><b>{{.Title}}</b><div class="dim">{{.Asks}}</div>{{if .Card}}<a href="{{.Card}}">Open its card</a>{{end}}</div>{{end}}{{end}}
{{else}}{{if not .Streams}}<p class="dim">No stream yet.</p>{{end}}
{{range .Streams}}<h2>{{.Name}}</h2><div class="dim">{{if .Next}}Next release: {{.Next}}{{if .NextDate}} · {{.NextDate}}{{end}}{{else}}No release planned{{end}}{{if .Awaiting}} · <span class="badge wait">{{.Awaiting}} waiting on you</span>{{end}}</div>
{{if .Ships}}<p class="dim">Ships in it</p>{{range .Ships}}<div class="item"><span class="badge">{{.Priority}}</span><b>{{.Title}}</b><div class="dim">{{.Done}}/{{.Total}} done · {{.State}}{{if .Awaiting}} · {{.Awaiting}} waiting on you{{end}}</div></div>{{end}}{{end}}
{{if .Others}}<p class="dim">Not in that release yet</p>{{range .Others}}<div class="item"><span class="badge">{{.Priority}}</span><b>{{.Title}}</b><div class="dim">{{.Done}}/{{.Total}} done · {{.State}}{{if .Awaiting}} · {{.Awaiting}} waiting on you{{end}}</div></div>{{end}}{{end}}{{end}}{{end}}
</body></html>`)))
