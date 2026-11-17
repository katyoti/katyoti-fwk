// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// The folders as the Windows Sandbox sees them: the channel's marketplace and
// the runner read-only, the results written back to the machine running the
// tool.
const (
	SandboxMarket  = `C:\Proof\market`
	SandboxTools   = `C:\Proof\tools`
	SandboxResults = `C:\Proof\results`
	// SandboxRunner holds the runner's own state, apart from the machine
	// under test.
	SandboxRunner = `C:\Proof\runner`
)

// The files the tool writes for a Sandbox run.
const (
	BootstrapFile = "bootstrap.cmd"
	PlanFile      = "plan.json"
	RunnerExe     = "ktw-compose.exe"
	RunnerLog     = "runner.log"
)

// SandboxExe is where Windows keeps Windows Sandbox once its optional feature
// is enabled.
func SandboxExe() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "WindowsSandbox.exe")
}

// ErrNoSandbox says Windows Sandbox is not there, and how to enable it.
var ErrNoSandbox = errors.New("Windows Sandbox is not enabled on this machine")

// RequireSandbox fails, saying how to enable it, when exe is not there.
func RequireSandbox(exe string) error {
	if _, err := os.Stat(exe); err == nil {
		return nil
	}
	return fmt.Errorf(`%w (no %s).
Enable it once, in a PowerShell run as administrator, then restart Windows:
  Enable-WindowsOptionalFeature -Online -FeatureName Containers-DisposableClientVM -All
(Windows 10/11 Pro, Enterprise or Education, virtualization on in the firmware.)
Or run the same stages on this machine, closed in a folder of its own: --kind local`, ErrNoSandbox, exe)
}

// wsb is a Windows Sandbox configuration file.
type wsb struct {
	XMLName       xml.Name       `xml:"Configuration"`
	VGpu          string         `xml:"VGpu"`
	Networking    string         `xml:"Networking"`
	MemoryInMB    int            `xml:"MemoryInMB,omitempty"`
	MappedFolders []mappedFolder `xml:"MappedFolders>MappedFolder"`
	LogonCommand  string         `xml:"LogonCommand>Command"`
}

type mappedFolder struct {
	HostFolder    string `xml:"HostFolder"`
	SandboxFolder string `xml:"SandboxFolder"`
	ReadOnly      bool   `xml:"ReadOnly"`
}

// WSB is the configuration of the Sandbox a run starts: the marketplace and
// the tools mapped read-only, the results folder writable, the network on
// (programs are downloaded for real), and at logon the bootstrap.
// The preparation cache, when there is one, is mapped read-only too.
func WSB(market, tools, results, cache string, memoryMB int) ([]byte, error) {
	c := wsb{
		VGpu: "Default", Networking: "Default", MemoryInMB: memoryMB,
		MappedFolders: []mappedFolder{
			{HostFolder: market, SandboxFolder: SandboxMarket, ReadOnly: true},
			{HostFolder: tools, SandboxFolder: SandboxTools, ReadOnly: true},
			{HostFolder: results, SandboxFolder: SandboxResults, ReadOnly: false},
		},
		LogonCommand: `cmd.exe /c ` + SandboxTools + `\` + BootstrapFile,
	}
	if cache != "" {
		c.MappedFolders = append(c.MappedFolders, mappedFolder{HostFolder: cache, SandboxFolder: SandboxCache, ReadOnly: true})
	}
	raw, err := xml.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// Bootstrap is what the Sandbox runs at logon. A clean Windows has no bash,
// so it is a .cmd: the runner (this tool) gets a state of its own, apart from
// the machine under test, and runs the plan — the installer double-clicked,
// then each stage — writing the results into the mapped results folder. A
// window follows the runner's log, so whoever watches the Sandbox sees each
// stage as it goes.
func Bootstrap() string {
	lines := []string{
		"@echo off",
		"rem The clean-machine proof's bootstrap, run at logon in a fresh Windows Sandbox.",
		"rem A clean Windows has no bash: this is a .cmd. The runner keeps its own state",
		"rem in " + SandboxRunner + ", apart from the machine under test.",
		`mkdir "` + SandboxRunner + `" 2>nul`,
		`set "KTW_KATYOTI_ROOT=` + SandboxRunner + `\root"`,
		`set "KTW_DATA=` + SandboxRunner + `\data"`,
		`set "KTW_CACHE=` + SandboxRunner + `\cache"`,
		`cd /d "` + SandboxRunner + `"`,
		"rem The run shown as it goes: a window following the runner's log.",
		`type nul > "` + SandboxResults + `\` + RunnerLog + `"`,
		`start "Clean-machine proof" powershell -NoExit -NoProfile -Command "Get-Content -Wait '` + SandboxResults + `\` + RunnerLog + `'"`,
		`"` + SandboxTools + `\` + RunnerExe + `" scenario machine-stages "` + SandboxTools + `\` + PlanFile + `" --results "` + SandboxResults + `" > "` + SandboxResults + `\` + RunnerLog + `" 2>&1`,
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

// wingetScript installs winget the way Microsoft documents for Windows
// Sandbox (the WinGet PowerShell module's Repair-WinGetPackageManager), then
// shows the version through the App Installer alias the stages will use.
const wingetScript = `$ErrorActionPreference = 'Stop'; $ProgressPreference = 'SilentlyContinue'; ` +
	`Install-PackageProvider -Name NuGet -Force | Out-Null; ` +
	`Install-Module -Name Microsoft.WinGet.Client -Force -Repository PSGallery | Out-Null; ` +
	`Repair-WinGetPackageManager -AllUsers; ` +
	`& "$env:LOCALAPPDATA\Microsoft\WindowsApps\winget.exe" --version`

// PrepareStages are the stages that give a Windows Sandbox what a person's
// desktop has before the case's own (machine.prepare; absent: winget). Their
// ids start with "prepare-": timed like the others, they are not the proof.
func PrepareStages(prepare *[]string) []api.MachineStage { return prepareStages(prepare, false) }

// prepareStages are the preparation stages; cached, winget and the Visual C++
// runtime install offline from the preparation cache.
func prepareStages(prepare *[]string, cached bool) []api.MachineStage {
	names := []string{api.MachinePrepareWinget, api.MachinePrepareVCRedist}
	if prepare != nil {
		names = *prepare
	}
	var out []api.MachineStage
	for _, n := range names {
		switch n {
		case api.MachinePrepareWinget:
			out = append(out, api.MachineStage{
				ID:         "prepare-winget",
				Title:      "winget, as a Windows 10/11 desktop has it (the Sandbox does not) - preparation, not the proof",
				Cmd:        []string{"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", wingetScript},
				TimeoutMin: 15,
				Contains:   []string{"v"},
			})
			if cached {
				st := &out[len(out)-1]
				st.Title = "winget from the preparation cache, as a Windows 10/11 desktop has it - preparation, not the proof"
				st.Cmd = []string{"powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cachedWingetScript()}
			}
		case api.MachinePrepareVCRedist:
			// Machine-wide: the Sandbox account is an administrator, so it
			// installs without a prompt, through the winget prepared before.
			out = append(out, api.MachineStage{
				ID:    "prepare-vcredist",
				Title: "the Microsoft Visual C++ runtime, as a desktop usually has it - preparation, not the proof",
				Cmd: []string{"winget", "install", "--id", "Microsoft.VCRedist.2015+.x64", "--exact", "--source", "winget",
					"--silent", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"},
				TimeoutMin: 15,
			})
			if cached {
				st := &out[len(out)-1]
				st.Title = "the Microsoft Visual C++ runtime from the preparation cache, as a desktop usually has it - preparation, not the proof"
				st.Cmd = cachedVCRedist()
			}
		case api.MachinePrepareBash:
			// The case stages' command checks run in bash:
			// Git for Windows, machine-wide, where projectcmd looks for it.
			out = append(out, api.MachineStage{
				ID:    "prepare-bash",
				Title: "Git for Windows and its bash, for the case stages' checks - preparation, not the proof",
				Cmd: []string{"winget", "install", "--id", "Git.Git", "--exact", "--source", "winget", "--scope", "machine",
					"--silent", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"},
				TimeoutMin: 15,
			})
		}
	}
	return out
}

// needsBash is whether a stage runs case steps, whose checks run in bash.
func needsBash(stages []api.MachineStage) bool {
	for _, s := range stages {
		if s.Case != nil {
			return true
		}
	}
	return false
}

// SandboxRun is a run's folder on the machine running the tool:
// tools/ (the runner, its plan, the bootstrap), results/ and <case>.wsb.
type SandboxRun struct {
	Dir string
	// Market is the channel's marketplace folder (mapped read-only).
	Market string
	// Runner is this tool's executable, copied into tools/.
	Runner string
	Plan   Plan
	// MemoryMB is the Sandbox's memory (default 8192).
	MemoryMB int
	// Cache is the preparation cache folder, mapped read-only; empty: none.
	Cache string
}

func (s SandboxRun) Tools() string   { return filepath.Join(s.Dir, "tools") }
func (s SandboxRun) Results() string { return filepath.Join(s.Dir, "results") }
func (s SandboxRun) File() string    { return filepath.Join(s.Dir, s.Plan.Case+".wsb") }

// Prepare writes the run's folder afresh: the runner, its plan (the channel
// as the Sandbox sees it), the bootstrap, an empty results folder, the .wsb.
func (s SandboxRun) Prepare() (string, error) {
	if _, err := KeepPreviousResults(s.Dir); err != nil {
		return "", err
	}
	for _, p := range []string{s.Tools(), s.Results(), s.File()} {
		if err := os.RemoveAll(p); err != nil {
			return "", err
		}
	}
	for _, d := range []string{s.Tools(), s.Results()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", err
		}
	}
	if err := copyFile(s.Runner, filepath.Join(s.Tools(), RunnerExe)); err != nil {
		return "", fmt.Errorf("the runner: %w", err)
	}
	if err := WritePlan(filepath.Join(s.Tools(), PlanFile), s.Plan); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(s.Tools(), BootstrapFile), []byte(Bootstrap()), 0o644); err != nil {
		return "", err
	}
	mem := s.MemoryMB
	if mem == 0 {
		mem = 8192
	}
	raw, err := WSB(s.Market, s.Tools(), s.Results(), s.Cache, mem)
	if err != nil {
		return "", err
	}
	return s.File(), os.WriteFile(s.File(), raw, 0o644)
}

// Launch opens the Sandbox on its configuration; it does not wait for it.
func Launch(exe, file string) error {
	return exec.Command(exe, file).Start()
}

// Await waits for the results a run writes into dir to be done, saying each
// stage as it ends; past timeout it fails with the end of the runner's log.
func Await(ctx context.Context, dir string, timeout, poll time.Duration, out io.Writer) (Results, error) {
	if out == nil {
		out = io.Discard
	}
	deadline := time.Now().Add(timeout)
	said := map[string]bool{}
	for {
		res, err := LoadResults(filepath.Join(dir, ResultsFile))
		if err == nil {
			for _, s := range res.Stages {
				if s.Status != Skipped && !said[s.ID] {
					said[s.ID] = true
					fmt.Fprintf(out, "    %s %s in %s%s\n", s.ID, s.Status, s.Duration().Round(time.Millisecond), suffix(s.Failure))
				}
			}
			if res.Done {
				return res, nil
			}
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(dir, RunnerLog))
			return res, fmt.Errorf("no results after %s in %s; the runner's log ends:\n%s", timeout, dir, tail(string(bytes.TrimSpace(log)), 2000))
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// HistoryDir is where a run folder keeps the earlier runs' results.
const HistoryDir = "history"

// KeepPreviousResults moves a run folder's results/ - the previous run's,
// results.json, logs and screenshots - into history/<its start time>/ before a
// new run makes results/ afresh; results/ stays the last
// run's, where the checks read it. It answers the folder kept ("" when there
// was none).
func KeepPreviousResults(dir string) (string, error) {
	results := filepath.Join(dir, "results")
	if _, err := os.Stat(results); err != nil {
		return "", nil
	}
	stamp := ""
	if r, err := LoadResults(filepath.Join(results, ResultsFile)); err == nil && r.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339, r.StartedAt); err == nil {
			stamp = t.UTC().Format("20060102-150405")
		}
	}
	if stamp == "" {
		info, err := os.Stat(results)
		if err != nil {
			return "", err
		}
		stamp = info.ModTime().UTC().Format("20060102-150405")
	}
	kept := filepath.Join(dir, HistoryDir, stamp)
	for n := 2; ; n++ {
		if _, err := os.Stat(kept); os.IsNotExist(err) {
			break
		}
		kept = filepath.Join(dir, HistoryDir, fmt.Sprintf("%s-%d", stamp, n))
	}
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		return "", err
	}
	return kept, renameRetry(results, kept)
}
