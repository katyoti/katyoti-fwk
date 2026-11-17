// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime/debug"
	"strings"
)

// DefaultMemoryLimit is the soft memory limit a KaTyoti program runs under
// when GOMEMLIMIT is not set and programMemoryLimits names no limit of its
// own: past it the Go collector runs as often as it must to stay under.
// Without one, a program lets its heap grow to twice its last live peak
// before collecting: the hub, which answers multi-MiB manifests to every
// command, keeps that much churn in garbage (its heap in use peaks near
// 230 MB under tests/live/hub-memory-budget.sh, well under this limit). It is
// a soft limit, not a cap: a program that really holds more keeps working past
// it, collecting more often; what it holds is bounded where it is held (the
// hub's replay cache once kept gigabytes of answers -).
const DefaultMemoryLimit int64 = 512 << 20

// NoMemoryLimit is the limit of a program that runs without a soft memory
// limit (the Go runtime's own default).
const NoMemoryLimit int64 = math.MaxInt64

// programMemoryLimits is the soft memory limit of a program whose live heap
// does not fit DefaultMemoryLimit. A limit below a program's live heap makes
// the collector run nearly continuously: ktw-agents holds 425-650 MB live on a
// real workspace, and under 512 MiB it collected every ~200 ms (3.3x CPU, a
// quarter fewer commands answered - review). It runs without a
// limit until measures the one it can hold.
var programMemoryLimits = map[string]int64{
	"ktw-agents": NoMemoryLimit,
}

// MemoryLimitOf answers the soft memory limit the program name runs under when
// GOMEMLIMIT is not set.
func MemoryLimitOf(name string) int64 {
	if limit, ok := programMemoryLimits[name]; ok {
		return limit
	}
	return DefaultMemoryLimit
}

// ApplyRuntimeGuards sets what every long-lived KaTyoti program runs under,
// first thing at its start:
//
//   - the soft memory limit: GOMEMLIMIT when the environment sets it (the Go
//     runtime already applied it), MemoryLimitOf(name) otherwise;
//   - the diagnostics endpoint: when KTW_PPROF_<NAME> (NAME is name in upper
//     case, '-' as '_': KTW_PPROF_HUB for the hub) names a port, net/http/pprof
//     is served on 127.0.0.1:<port> — heap and goroutine profiles of a
//     running program (curl 127.0.0.1:<port>/debug/pprof/heap). Off by
//     default; "1" is port 6060.
//
// It answers the memory limit in force.
func ApplyRuntimeGuards(name string) int64 {
	limit := applyMemoryLimit(name)
	if port := os.Getenv(PprofEnv(name)); port != "" {
		ServePprof(name, port)
	}
	return limit
}

// PprofEnv is the variable that opens the diagnostics endpoint of the program
// name: KTW_PPROF_<NAME>.
func PprofEnv(name string) string {
	return "KTW_PPROF_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func applyMemoryLimit(name string) int64 {
	if strings.TrimSpace(os.Getenv("GOMEMLIMIT")) != "" {
		return debug.SetMemoryLimit(-1) // the runtime read it at start
	}
	limit := MemoryLimitOf(name)
	debug.SetMemoryLimit(limit)
	return limit
}

// ServePprof serves the profiles of the program on 127.0.0.1:<port>, on a mux
// of their own (never the default one).
func ServePprof(name, port string) {
	if port == "1" {
		port = "6060"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	addr := net.JoinHostPort("127.0.0.1", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Warn("diagnostics: the pprof endpoint does not open", "program", name, "addr", addr, "error", err)
		return
	}
	slog.Info("diagnostics: pprof served", "program", name, "addr", addr)
	go func() { _ = http.Serve(ln, mux) }()
}
