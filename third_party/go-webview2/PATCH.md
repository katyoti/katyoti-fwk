# go-webview2 v1.0.22, patched for katyoti

This folder is `github.com/wailsapp/go-webview2` v1.0.22 as published, with one
change, used through `replace github.com/wailsapp/go-webview2 => ./third_party/go-webview2`
in the repository's `go.mod`.

## Why

The clean-machine proof attaches to the built GUI over WebView2's remote
debugging port (`--remote-debugging-port=<port>`). Nothing can pass that argument:

- go-webview2 empties `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` and the registry
  overrides at init and again before it creates an environment
  (`webviewloader/env_create.go`, `preventEnvAndRegistryOverrides`);
- Wails v2.16 fills `edge.Chromium.AdditionalBrowserArgs` only from its own
  options (`--disable-gpu`, `--disable-features=…`,
  `internal/frontend/desktop/windows/frontend.go`) and has no option for more.

## The change

- `pkg/edge/extra_browser_args.go` (new): `var ExtraBrowserArgs []string`, the
  arguments a program adds to every environment it creates, and
  `browserArguments`, which joins a Chromium's own arguments and them;
  `extra_browser_args_test.go` tests the join.
- `pkg/edge/chromium.go`, where the environment is created:
  `strings.Join(e.AdditionalBrowserArgs, " ")` becomes
  `browserArguments(e.AdditionalBrowserArgs)`, and its unused `strings` import
  goes.

ktw-gui fills `edge.ExtraBrowserArgs` from `KTW_GUI_DEBUG_PORT` before the
window opens (`app/ktw-gui/debug_port_windows.go`); nothing else sets it.

## On an update of go-webview2 or Wails

1. Copy the new version of `github.com/wailsapp/go-webview2` here (the one
   the new Wails requires: `go mod graph | grep go-webview2`), replacing this folder.
2. Add `pkg/edge/extra_browser_args.go` and its test back, and change the join in
   `pkg/edge/chromium.go` the same way.
3. `go test ./third_party/go-webview2/pkg/edge/ ./app/ktw-gui/ -run 'ExtraBrowserArgs|DebugPort'`.

Drop the patch, and this folder, once go-webview2 or Wails offers extra browser
arguments.
