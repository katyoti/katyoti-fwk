// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// guiStepTimeout bounds a gui step that names none.
const guiStepTimeout = 60 * time.Second

// guiPoll is how often a gui step looks at the page again.
const guiPoll = 250 * time.Millisecond

// runGUI drives the GUI a previous stage started over its debugging port: it
// attaches to its page (never opening one) and runs the steps in order, each
// printed on out; the first that does not hold ends the stage. shots are the
// screenshots it saved, for the stage's image check.
func runGUI(ctx context.Context, g *api.GUIStage, expand func(string) (string, error), out io.Writer) (shots []string, err error) {
	port := g.Port
	if port == 0 {
		port = api.GUIDebugPort
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	// The GUI a previous stage started may still be opening: its port is waited
	// for as long as the first step allows.
	wait := guiStepTimeout
	if len(g.Steps) > 0 && g.Steps[0].TimeoutSec > 0 {
		wait = time.Duration(g.Steps[0].TimeoutSec) * time.Second
	}
	if err := awaitDebugPort(ctx, base, wait); err != nil {
		return nil, err
	}
	page, err := guiPage(ctx, base)
	if err != nil {
		return nil, err
	}
	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, base)
	defer cancelAlloc()
	// Attached to the GUI's own page. chromedp closes a page it attached to
	// when its context ends (on a remote browser, always): the target is
	// forgotten first, so the end only disconnects and the person's window
	// stays open.
	pageCtx, cancelPage := chromedp.NewContext(allocCtx, chromedp.WithTargetID(page))
	defer func() {
		if c := chromedp.FromContext(pageCtx); c != nil {
			c.Target = nil
		}
		cancelPage()
	}()
	// Attached here, not in a step: chromedp keeps the connection of its
	// first run's context, and a step's ends with the step.
	if err := chromedp.Run(pageCtx); err != nil {
		return nil, fmt.Errorf("attach to the GUI's page on %s: %w", base, err)
	}

	for i, st := range g.Steps {
		kind, err := st.Kind()
		if err != nil {
			return shots, fmt.Errorf("step %d: %w", i+1, err)
		}
		timeout := time.Duration(st.TimeoutSec) * time.Second
		if timeout <= 0 {
			timeout = guiStepTimeout
		}
		sctx, cancel := context.WithTimeout(pageCtx, timeout)
		what, err := guiStep(sctx, kind, st, expand, &shots)
		cancel()
		if err != nil {
			return shots, fmt.Errorf("step %d (%s): %w", i+1, what, err)
		}
		fmt.Fprintf(out, "gui: %s\n", what)
	}
	return shots, nil
}

func guiStep(ctx context.Context, kind string, st api.GUIStep, expand func(string) (string, error), shots *[]string) (string, error) {
	switch kind {
	case "title":
		what := fmt.Sprintf("title contains %q", st.Title)
		var last string
		err := poll(ctx, func() (bool, error) {
			if err := chromedp.Run(ctx, chromedp.Title(&last)); err != nil {
				return false, err
			}
			return strings.Contains(last, st.Title), nil
		})
		return what, seen(err, fmt.Sprintf("the title is %q", last))
	case "click":
		what := "clicked " + st.Click
		if _, err := awaitElement(ctx, st.Click, "", ""); err != nil {
			return what, err
		}
		return what, chromedp.Run(ctx, chromedp.Click(selector(st.Click), chromedp.ByQuery, chromedp.NodeVisible))
	case "type":
		what := fmt.Sprintf("typed into %s", st.Type)
		text, err := expand(st.Text)
		if err != nil {
			return what, err
		}
		if _, err := awaitElement(ctx, st.Type, "", ""); err != nil {
			return what, err
		}
		// Focused, the text goes at the end of what the field holds - or in its
		// place with Clear (the field selected, then erased) - typed as keys,
		// so a screen's own handlers see each one.
		sel, _ := json.Marshal(selector(st.Type))
		place := `(() => { const e = document.querySelector(` + string(sel) + `); const n = (e.value || '').length; e.setSelectionRange(` + map[bool]string{true: "0", false: "n"}[st.Clear] + `, n); return n })()`
		var held int
		actions := []chromedp.Action{chromedp.Focus(selector(st.Type), chromedp.ByQuery, chromedp.NodeVisible), chromedp.Evaluate(place, &held)}
		if st.Clear {
			actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
				if held == 0 {
					return nil
				}
				return chromedp.KeyEvent(kb.Backspace).Do(ctx)
			}))
		}
		actions = append(actions, chromedp.KeyEvent(text))
		return what, chromedp.Run(ctx, actions...)
	case "wait":
		what := st.Wait
		if st.State != "" {
			what += " is " + st.State
		}
		if st.Contains != "" {
			what += fmt.Sprintf(" shows %q", st.Contains)
		}
		_, err := awaitElement(ctx, st.Wait, st.State, st.Contains)
		return what, err
	case "shot":
		path, err := expand(st.Shot)
		if err != nil {
			return "shot", err
		}
		what := "saved " + filepath.Base(path)
		var png []byte
		if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&png)); err != nil {
			return what, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return what, err
		}
		if err := os.WriteFile(path, png, 0o644); err != nil {
			return what, err
		}
		*shots = append(*shots, path)
		return what, nil
	}
	return kind, fmt.Errorf("unknown step")
}

// element is what a gui step reads of an element.
type element struct {
	State string `json:"state"`
	Text  string `json:"text"`
}

// awaitElement waits for the element of that id to be on the page - with a
// state, for its data-state; with a text, for that text in it - and says what
// it last saw when it does not come.
func awaitElement(ctx context.Context, id, state, contains string) (element, error) {
	sel, _ := json.Marshal(selector(id))
	js := `(() => { const e = document.querySelector(` + string(sel) + `); return e ? {state: e.getAttribute('data-state') || '', text: e.innerText || ''} : null })()`
	var el *element
	err := poll(ctx, func() (bool, error) {
		el = nil
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, &el)); err != nil {
			return false, err
		}
		return el != nil && (state == "" || el.State == state) && strings.Contains(el.Text, contains), nil
	})
	switch {
	case err == nil:
		return *el, nil
	case el == nil:
		return element{}, seen(err, "no element "+id+" on the page")
	case state != "" && el.State != state:
		return *el, seen(err, fmt.Sprintf("%s is %q", id, el.State))
	default:
		return *el, seen(err, fmt.Sprintf("%s shows %q", id, tail(el.Text, 200)))
	}
}

// selector is the CSS selector of an element id (: data-element).
func selector(id string) string {
	return `[data-element="` + strings.ReplaceAll(id, `"`, `\"`) + `"]`
}

// poll calls ok until it holds or ctx ends; a call that fails keeps polling
// (the page may be loading) and its error is answered at the end.
func poll(ctx context.Context, ok func() (bool, error)) error {
	var last error
	for {
		done, err := ok()
		if err == nil && done {
			return nil
		}
		if err != nil {
			last = err
		}
		select {
		case <-ctx.Done():
			if last != nil && ctx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("%w (last: %v)", ctx.Err(), last)
			}
			return ctx.Err()
		case <-time.After(guiPoll):
		}
	}
}

// seen words a step that timed out by what was last seen.
func seen(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if _, last, ok := strings.Cut(err.Error(), "(last: "); ok {
			return fmt.Errorf("timed out: %s (%s", what, last)
		}
		return fmt.Errorf("timed out: %s", what)
	}
	return fmt.Errorf("%w: %s", err, what)
}

// guiServedWait is how long a gui stage waits for the GUI's served page to be
// listed before it takes another page (the GUI may still be loading).
const guiServedWait = 15 * time.Second

// guiPage is the GUI's page among the debugging port's targets: its served
// page once listed, else - after guiServedWait - its first page.
func guiPage(ctx context.Context, base string) (target.ID, error) {
	deadline := time.Now().Add(guiServedWait)
	for {
		id, served, err := listPage(ctx, base)
		if err != nil || served || time.Now().After(deadline) {
			return id, err
		}
		select {
		case <-ctx.Done():
			return id, nil
		case <-time.After(guiPoll):
		}
	}
}

// listPage reads the port's targets once: the served page (true), else the
// first page.
func listPage(ctx context.Context, base string) (target.ID, bool, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/json/list", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("the GUI's pages on %s: %w", base, err)
	}
	defer resp.Body.Close()
	var targets []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", false, fmt.Errorf("the GUI's pages on %s: %w", base, err)
	}
	// The GUI's page is served (Wails: http://wails.localhost); a browser's
	// own pages (edge://, about:) come only when nothing else does.
	var first target.ID
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		if strings.HasPrefix(t.URL, "http://") || strings.HasPrefix(t.URL, "https://") {
			return target.ID(t.ID), true, nil
		}
		if first == "" {
			first = target.ID(t.ID)
		}
	}
	if first == "" {
		return "", false, fmt.Errorf("the GUI on %s shows no page", base)
	}
	return first, false, nil
}

// awaitDebugPort waits for the GUI's debugging port to answer: the GUI a
// previous stage started may still be opening.
func awaitDebugPort(ctx context.Context, base string, timeout time.Duration) error {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := poll(cctx, func() (bool, error) {
		req, _ := http.NewRequestWithContext(cctx, http.MethodGet, base+"/json/version", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false, err
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK, nil
	})
	if err != nil {
		return fmt.Errorf("no GUI answers on %s - was it started with KTW_GUI_DEBUG_PORT=<port>? (%v)", base, err)
	}
	return nil
}
