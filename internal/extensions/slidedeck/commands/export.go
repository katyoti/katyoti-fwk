// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	displayapi "github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	deckdisplay "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/display"
	"github.com/spf13/cobra"
)

// The display server makes a PowerPoint slide by slide: give it time.
var exportClient = &http.Client{Timeout: 15 * time.Minute}

// errMissing marks an error that names a missing dependency: printed as is.
type errMissing struct{ msg string }

func (e errMissing) Error() string { return e.msg }

func newExportCommand() *cobra.Command {
	var (
		format, out, displayURL string
		universe                bool
	)
	cmd := &cobra.Command{
		Use:   "export <deck.md>",
		Short: "Export a deck file as PowerPoint, HTML or one PNG per slide",
		Long: `Exports a .deck.md file with no GUI and no universe, through the display
server (ktw-display) — the same page and the same export the GUI's Export menu
and the agent's deck-export tool run:

  pptx            a PowerPoint of editable slides in the deck's theme
  pptx-pictures   a PowerPoint of one picture per slide, exactly as shown
  html            one HTML page that presents offline
  png             one picture per slide, slide-01.png, slide-02.png ... in a folder

The files the deck names beside it (pictures, draw.io diagrams) go with it.

It needs two programs:
  - a display server (ktw-display). By default the command starts one of its
    own, on a free port with a fresh token, and stops it at the end. It looks
    for it at KTW_DISPLAY_BIN, then the installed one, then beside this program,
    then app/ktw-display/dist/. --display uses one already running instead
    (its URL with ?token=).
  - for pptx, pptx-pictures and png, a Chromium browser the display server
    drives headless: Edge, Chrome or Chromium where they install, or the one
    KTW_BROWSER names.
When one is missing the command stops at once and says what, where it looked
and how to provide it.

--out is the file to write (default: beside the deck, with the format's
extension); for png, the folder (default: <deck>.slides/ beside it).

Examples:
  ktw-docs deck export review.deck.md --format png
  ktw-docs deck export review.deck.md --format pptx --out review.pptx
  ktw-docs deck export review.deck.md --format html --display "http://127.0.0.1:41234/?token=abc"`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd.Context(), cmd.OutOrStdout(), args[0], format, out, displayURL, universe)
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "pptx, pptx-pictures, html or png")
	cmd.Flags().StringVar(&out, "out", "", "The file to write, or the folder for png (default beside the deck)")
	cmd.Flags().StringVar(&displayURL, "display", "", "A running display server's URL with its token (http://127.0.0.1:<port>/?token=<token>); default: start one")
	cmd.Flags().BoolVar(&universe, "all-sidecars", false, "Take the theme and slide groups from every sidecar of the universe, not only this one")
	_ = cmd.MarkFlagRequired("format")
	return cmd
}

func runExport(ctx context.Context, w io.Writer, file, format, out, displayURL string, universe bool) error {
	// Ctrl-C or a terminate ends the export through its context, so the
	// display server started for it is stopped (deferred) rather than left.
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	f := deckdisplay.Format(strings.TrimSpace(format))
	if !f.Valid() && f != deckdisplay.FormatPNG {
		return fmt.Errorf("--format must be pptx, pptx-pictures, html or png, not %q", format)
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	q := queryOf(ctx, universe)
	markdown := string(src)
	if groups, err := deckapi.GroupsFor(ctx, q, markdown); err == nil {
		markdown = deckapi.InlineGroups(markdown, groups)
	}
	deck := deckapi.Parse(markdown)
	if len(deck.Slides) == 0 {
		return fmt.Errorf("%s: the deck has no slide", file)
	}
	// The theme is read before any server starts: a wrong name fails first.
	var theme *deckapi.ResolvedTheme
	if name, _ := deck.Settings["theme"].(string); name != "" && name != deckapi.DefaultTheme {
		t, err := deckapi.ResolveTheme(ctx, q, name)
		if err != nil {
			hint := ""
			if !universe {
				hint = " (this program's themes only: --all-sidecars reads every sidecar's)"
			}
			return fmt.Errorf("theme %q%s: %w", name, hint, err)
		}
		theme = &t
	}

	origin, token, stop, err := displayServer(ctx, w, displayURL)
	if err != nil {
		return err
	}
	defer stop()

	if theme != nil {
		if _, err := deckdisplay.PublishTheme(ctx, exportClient, origin, token, *theme); err != nil {
			return explain(err)
		}
	}
	var assets *deckdisplay.AssetsRef
	dir := filepath.Dir(abs)
	files, missing := deckdisplay.ReadBeside(deckapi.LocalAssets(markdown), func(rel string) ([]byte, error) {
		return os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	})
	for _, m := range missing {
		fmt.Fprintf(w, "warning: %s names %s, which is not beside it\n", file, m)
	}
	if len(files) > 0 {
		ref, err := deckdisplay.PublishAssets(ctx, exportClient, origin, token, deckdisplay.AssetsScope("file:"+abs), files)
		if err != nil {
			return explain(err)
		}
		assets = &ref
	}
	markdown = deckapi.MarkDeck(ctx, markdown)
	serverFormat := f
	if f == deckdisplay.FormatPNG {
		serverFormat = deckdisplay.FormatPptxPictures
	}
	data, err := deckdisplay.Export(ctx, exportClient, origin, token, markdown, serverFormat, assets)
	if err != nil {
		return explain(err)
	}

	base := strings.TrimSuffix(strings.TrimSuffix(abs, ".md"), ".deck")
	if f == deckdisplay.FormatPNG {
		pics, err := deckdisplay.SlidePictures(data)
		if err != nil {
			return err
		}
		folder := out
		if folder == "" {
			folder = base + ".slides"
		}
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return err
		}
		for i, p := range pics {
			if err := os.WriteFile(filepath.Join(folder, fmt.Sprintf("slide-%02d.png", i+1)), p, 0o644); err != nil {
				return err
			}
		}
		fmt.Fprintf(w, "%d slide pictures written to %s\n", len(pics), folder)
		return nil
	}
	path := out
	if path == "" {
		path = base + f.Extension()
	} else if filepath.Ext(path) == "" {
		path += f.Extension()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s written: %s (%d slides, %d KB)\n", f, path, len(deck.Slides), len(data)/1024)
	return nil
}

// displayServer is the display server the export goes through: the one
// --display names, checked at once, or one started for this command.
func displayServer(ctx context.Context, w io.Writer, displayURL string) (origin, token string, stop func(), err error) {
	if displayURL != "" {
		origin, token, err = deckdisplay.Origin(displayURL)
		if err != nil {
			return "", "", nil, err
		}
		probe := &http.Client{Timeout: 3 * time.Second}
		if err := displayapi.Reachable(ctx, probe, origin); err != nil {
			return "", "", nil, errMissing{fmt.Sprintf(`deck export needs a display server (ktw-display); none answers at %s/healthz (%v).
Provide it: start one with its own port and token
    KTW_DISPLAY_PORT=<port> KTW_DISPLAY_TOKEN=<token> ktw-display
and pass --display "http://127.0.0.1:<port>/?token=<token>" — or leave out --display and the command starts one itself.`, origin, err)}
		}
		if token == "" {
			return "", "", nil, errMissing{fmt.Sprintf("deck export: --display %s carries no token; give the server's URL with ?token=<its KTW_DISPLAY_TOKEN>", displayURL)}
		}
		return origin, token, func() {}, nil
	}
	found := displayapi.LocateDisplayBinary()
	if found.Path == "" {
		return "", "", nil, errMissing{fmt.Sprintf(`deck export needs a display server (ktw-display) and found none. Looked at:
    %s
Provide it: build it (make ktw-display places it in bin/ beside the other programs), install KaTyoti, set %s to its path, or pass --display with the URL and token of a running one.`, strings.Join(found.Looked, "\n    "), displayapi.DisplayBinEnv)}
	}
	fmt.Fprintf(w, "display server: %s\n", found.Path)
	s, err := displayapi.StartStandalone(ctx, found.Path)
	if err != nil {
		return "", "", nil, errMissing{fmt.Sprintf("deck export needs a display server (ktw-display): %v\nProvide another: set %s to a working ktw-display, or pass --display with the URL and token of a running one.", err, displayapi.DisplayBinEnv)}
	}
	return s.Origin, s.Token, s.Stop, nil
}

// explain keeps the display server's word on a refused token or a missing
// browser and says what to do about it.
func explain(err error) error {
	var m errMissing
	if errors.As(err, &m) {
		return err
	}
	if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
		return errMissing{fmt.Sprintf("%v\nThe display server refused the token: give --display the URL with ?token=<the KTW_DISPLAY_TOKEN it was started with>, or leave out --display and the command starts one itself.", err)}
	}
	if strings.Contains(err.Error(), "Chromium browser") || strings.Contains(err.Error(), "the browser did not start") {
		return errMissing{fmt.Sprintf("%v\nThe pptx, pptx-pictures and png exports capture the slides in a headless Chromium browser: install Microsoft Edge, Google Chrome or Chromium, or set KTW_BROWSER to its executable in the display server's environment (a server this command starts inherits it). The html export needs none.", err)}
	}
	return err
}
