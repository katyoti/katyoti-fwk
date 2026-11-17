// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Tailscale publishes the page to the person's tailnet: its origin is the PC's
// MagicDNS name over https; tailscale serve forwards it to the page's local
// port; tailscale funnel, which would expose it to the Internet, is never used
// and makes the page count as not published.
type Tailscale struct {
	// Run runs the tailscale command line; replaced in tests.
	Run func(ctx context.Context, args ...string) ([]byte, error)
}

// NewTailscale runs the installed tailscale.
func NewTailscale() *Tailscale { return &Tailscale{Run: runTailscale} }

func runTailscale(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	exe := "tailscale"
	if runtime.GOOS == "windows" {
		if p := filepath.Join(os.Getenv("ProgramFiles"), "Tailscale", "tailscale.exe"); fileExists(p) {
			exe = p
		}
	}
	return exec.CommandContext(ctx, exe, args...).Output()
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Origin is https://<the PC's MagicDNS name>, the passkey's relying party.
func (t *Tailscale) Origin(ctx context.Context) (string, error) {
	out, err := t.Run(ctx, "status", "--json")
	if err != nil {
		return "", fmt.Errorf("tailscale is not running here: %v", err)
	}
	var st struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &st); err != nil || strings.TrimSuffix(st.Self.DNSName, ".") == "" {
		return "", errors.New("tailscale gives this PC no name on the tailnet")
	}
	return "https://" + strings.TrimSuffix(st.Self.DNSName, "."), nil
}

// PublishArgs is the one command that publishes the page: serve, never funnel.
func PublishArgs(port int) []string {
	return []string{"serve", "--bg", "--https=443", fmt.Sprintf("http://127.0.0.1:%d", port)}
}

// Publish runs tailscale serve for the page's port.
func (t *Tailscale) Publish(ctx context.Context, port int) error {
	if _, err := t.Run(ctx, PublishArgs(port)...); err != nil {
		return fmt.Errorf("tailscale serve: %v", err)
	}
	return nil
}

// Published reads tailscale serve's state: whether the origin forwards to the
// page's port, and whether funnel exposes it to the Internet.
func (t *Tailscale) Published(ctx context.Context, port int) (served, funnel bool, err error) {
	out, err := t.Run(ctx, "serve", "status", "--json")
	if err != nil {
		return false, false, fmt.Errorf("tailscale serve status: %v", err)
	}
	var st struct {
		Web map[string]struct {
			Handlers map[string]struct {
				Proxy string `json:"Proxy"`
			} `json:"Handlers"`
		} `json:"Web"`
		AllowFunnel map[string]bool `json:"AllowFunnel"`
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return false, false, err
	}
	target := fmt.Sprintf("127.0.0.1:%d", port)
	for hostPort, web := range st.Web {
		for _, h := range web.Handlers {
			if strings.Contains(h.Proxy, target) {
				served = true
				if st.AllowFunnel[hostPort] {
					funnel = true
				}
			}
		}
	}
	return served, funnel, nil
}
