// SPDX-License-Identifier: Apache-2.0

// Package redact scrubs secrets from byte streams before they reach disk.
//
// The threat model is AT-REST disclosure: session journals (llm.jsonl /
// user.jsonl) and --trace NDJSON record raw LLM and tool I/O, so any secret
// the agent reads (env vars, API keys in config files, tokens in tool
// output) would otherwise persist verbatim. Redaction is applied at the
// single marshaled-line chokepoint of each writer — one scan per line,
// nesting-agnostic, and JSON-safe (markers are plain text inside string
// values).
//
// Two matching layers:
//  1. ENV VALUES — the values of process env vars whose NAMES look
//     secret-ish (KEY / TOKEN / SECRET / PASSW / CREDENTIAL / PWD / AUTH),
//     matched in both raw and JSON-escaped form. Replaced with
//     `[REDACTED:env:<NAME>]` so journals stay diagnosable.
//  2. PATTERNS — a small, conservative catalog of well-known credential
//     shapes (provider API keys, bearer tokens, JWTs, PEM private keys,
//     URL-embedded passwords). Replaced with `[REDACTED:<kind>]`.
//
// IMPORTANT for future work: the engine event plane is in-process today
// (MemoryEventBus), so disk is the only persistence of these payloads. If
// session events are ever bridged to NATS / JetStream (the
// KTW_SESSION_JOURNAL design), that bridge MUST route through the same
// Redactor BEFORE publish — otherwise redacted files sit next to an
// unredacted stream.
package redact

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
)

// RedactionSettings is the user-facing configuration (Redaction.yaml via
// the standard config pipeline). The zero value means ENABLED with the
// built-in catalog — redaction is opt-out, not opt-in.
type RedactionSettings struct {
	// Disabled turns redaction off entirely.
	Disabled bool `json:"disabled" yaml:"disabled"`

	// ExtraEnvVars lists additional env var NAMES whose values should be
	// redacted regardless of the secret-ish name heuristic.
	ExtraEnvVars []string `json:"extraEnvVars" yaml:"extraEnvVars"`

	// ExtraPatterns lists additional regular expressions to scrub. Each
	// match is replaced with [REDACTED:custom].
	ExtraPatterns []string `json:"extraPatterns" yaml:"extraPatterns"`

	// MinEnvValueLen guards against redacting trivially short env values
	// ("1", "on", …) that would riddle journals with false markers.
	// 0 means the default (8).
	MinEnvValueLen int `json:"minEnvValueLen" yaml:"minEnvValueLen" default:"8"`
}

// secretishName matches env var names that conventionally hold secrets.
var secretishName = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET|PASSW|CREDENTIAL|PWD|AUTH)`)

type compiledPattern struct {
	re   *regexp.Regexp
	repl string
}

// builtinPatterns is deliberately conservative: every entry is a shape
// that essentially cannot be anything BUT a credential. False positives
// corrupt journals; false negatives are caught by the env layer.
var builtinPatterns = []compiledPattern{
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`), "[REDACTED:api-key]"},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "[REDACTED:aws-key]"},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`), "[REDACTED:github-token]"},
	{regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`), "[REDACTED:slack-token]"},
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{20,}`), "[REDACTED:bearer]"},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), "[REDACTED:jwt]"},
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`), "[REDACTED:private-key]"},
}

// urlCredential redacts only the password of a URL's user-info (the part between the
// user name's colon and the at sign), keeping the scheme, the user and the host.
var urlCredential = regexp.MustCompile(`(://[^/\s:@"\\]+:)[^@\s/"\\]+(@)`)

// Redactor scrubs lines. Safe for concurrent use after construction
// (all state is read-only).
type Redactor struct {
	envReplacer *strings.Replacer
	patterns    []compiledPattern
}

// New builds a Redactor from settings. Returns nil when redaction is
// disabled — callers treat a nil *Redactor as a no-op.
func New(s RedactionSettings) *Redactor {
	return NewWithValues(s, nil)
}

// NewWithValues is New whose env layer also scrubs values, a name to its
// value, whatever the name: the pairs of a .env a caller read itself, so a
// key kept under a name that does not look secret is still masked. A value
// shorter than the minimum length is left, as for the process env.
func NewWithValues(s RedactionSettings, values map[string]string) *Redactor {
	if s.Disabled {
		return nil
	}
	minLen := s.MinEnvValueLen
	if minLen <= 0 {
		minLen = 8
	}

	extra := make(map[string]bool, len(s.ExtraEnvVars))
	for _, name := range s.ExtraEnvVars {
		extra[name] = true
	}

	// Collect env values to scrub. Both the raw value and its
	// JSON-escaped form are registered so matching works on marshaled
	// journal lines (e.g. a value containing `"` appears as `\"`).
	var pairs []string
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		name, value := kv[:eq], kv[eq+1:]
		if len(value) < minLen {
			continue
		}
		if !extra[name] && !secretishName.MatchString(name) {
			continue
		}
		marker := "[REDACTED:env:" + name + "]"
		pairs = append(pairs, value, marker)
		if escaped := jsonEscape(value); escaped != value {
			pairs = append(pairs, escaped, marker)
		}
	}

	for name, value := range values {
		if len(value) < minLen {
			continue
		}
		marker := "[REDACTED:env:" + name + "]"
		pairs = append(pairs, value, marker)
		if escaped := jsonEscape(value); escaped != value {
			pairs = append(pairs, escaped, marker)
		}
	}

	r := &Redactor{patterns: compilePatterns(s)}
	if len(pairs) > 0 {
		r.envReplacer = strings.NewReplacer(pairs...)
	}
	return r
}

// NewShapesOnly is New without the env layer: it scrubs only the credential
// shapes (the built-in patterns and the settings' extra ones), never reads
// the process environment. A scan for credentials in files uses it, so that
// it neither depends on nor touches the environment of the process running it.
func NewShapesOnly(s RedactionSettings) *Redactor {
	if s.Disabled {
		return nil
	}
	return &Redactor{patterns: compilePatterns(s)}
}

// compilePatterns is the built-in patterns followed by the settings' extra
// ones; a pattern that does not compile is skipped.
func compilePatterns(s RedactionSettings) []compiledPattern {
	patterns := append([]compiledPattern(nil), builtinPatterns...)
	for _, p := range s.ExtraPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue // a broken custom pattern must not disable redaction
		}
		patterns = append(patterns, compiledPattern{re, "[REDACTED:custom]"})
	}
	return patterns
}

// jsonEscape returns the JSON string-encoding of s without the
// surrounding quotes.
func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil || len(b) < 2 {
		return s
	}
	return string(b[1 : len(b)-1])
}

// RedactString scrubs a string. Nil-receiver safe (returns input).
func (r *Redactor) RedactString(s string) string {
	if r == nil {
		return s
	}
	if r.envReplacer != nil {
		s = r.envReplacer.Replace(s)
	}
	for _, p := range r.patterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	s = urlCredential.ReplaceAllString(s, "${1}[REDACTED:url-credential]${2}")
	return s
}

// RedactBytes scrubs a marshaled line. Nil-receiver safe (returns input).
func (r *Redactor) RedactBytes(line []byte) []byte {
	if r == nil {
		return line
	}
	return []byte(r.RedactString(string(line)))
}

// Writer wraps w so every Write is redacted first. Intended for
// line-per-Write streams (journal / trace writers emit one NDJSON line
// per call); a secret split across two Write calls is not matched.
func Writer(w io.Writer, r *Redactor) io.Writer {
	if r == nil {
		return w
	}
	return &redactingWriter{w: w, r: r}
}

type redactingWriter struct {
	w io.Writer
	r *Redactor
}

// Write redacts p before forwarding. It reports the ORIGINAL length on
// success so io.Writer contract holds for callers tracking input bytes.
func (rw *redactingWriter) Write(p []byte) (int, error) {
	red := rw.r.RedactBytes(p)
	if _, err := rw.w.Write(red); err != nil {
		return 0, err
	}
	return len(p), nil
}
