// SPDX-License-Identifier: Apache-2.0

//go:build windows

package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// webView2Client is the EdgeUpdate client id of the WebView2 Runtime; its
// "pv" value is the installed version, Microsoft's documented detection.
const webView2Client = `Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// webView2Bootstrapper is Microsoft's Evergreen bootstrapper: it downloads and
// installs the runtime, per user when not elevated.
const webView2Bootstrapper = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

func thisGUIRuntime() *guiRuntime {
	return &guiRuntime{name: "Microsoft Edge WebView2 Runtime", present: webView2Present, install: installWebView2}
}

// webView2Present reads the runtime's version where it is installed for the
// machine (64- or 32-bit view) or for the person.
func webView2Present() bool {
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\` + webView2Client},
		{registry.LOCAL_MACHINE, `SOFTWARE\` + webView2Client},
		{registry.CURRENT_USER, `Software\` + webView2Client},
	} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		pv, _, err := key.GetStringValue("pv")
		_ = key.Close()
		if err == nil && pv != "" && pv != "0.0.0.0" {
			return true
		}
	}
	return false
}

// installWebView2 runs the Evergreen bootstrapper silently.
func installWebView2(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "katyoti-webview2-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	setup := filepath.Join(dir, "MicrosoftEdgeWebview2Setup.exe")
	dctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(dctx, http.MethodGet, webView2Bootstrapper, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download the bootstrapper: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download the bootstrapper: %s", resp.Status)
	}
	f, err := os.Create(setup)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	ictx, icancel := context.WithTimeout(ctx, 10*time.Minute)
	defer icancel()
	out, err := exec.CommandContext(ictx, setup, "/silent", "/install").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
