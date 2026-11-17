// SPDX-License-Identifier: Apache-2.0

// Package secrets is the one definition of what an agent must never see:
// the secret paths its file tools, shells and workers refuse, and the
// allow-listed environment its child processes start from.
//
// The list is defined here once and read by every consumer - KaTyoti's file
// tools and bash tool, the Claude Code worker's deny rules, the MCP server's
// children, and the repository hygiene that ignores and refuses secret files
// at commit (GitignorePatterns). A consumer never copies it.
//
// The package is pure: no DI, no application context.
package secrets

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// GitignorePatterns returns the secret paths as gitignore patterns, in the
// order they are documented. IsSecretPath matches exactly these.
func GitignorePatterns() []string {
	return []string{
		".env",
		".env.*",
		"*.env",
		"!.env.example",
		"!.env.*.example",
		".secret/",
		".secrets/",
		"**/hub/keys/",
		"*.pem",
		"*.key",
		"*.p12",
		"*.pfx",
		"*.jks",
		"*.keystore",
		"*.token",
		"id_rsa",
		"id_dsa",
		"id_ecdsa",
		"id_ed25519",
		".netrc",
		".pgpass",
		".git-credentials",
	}
}

var secretDirs = map[string]bool{".secret": true, ".secrets": true}

var secretExts = map[string]bool{
	".pem": true, ".key": true, ".p12": true, ".pfx": true,
	".jks": true, ".keystore": true, ".token": true,
}

var secretNames = map[string]bool{
	"id_rsa": true, "id_dsa": true, "id_ecdsa": true, "id_ed25519": true,
	".netrc": true, ".pgpass": true, ".git-credentials": true,
}

// IsSecretPath reports whether path (absolute or relative, either
// separator) names a secret file or a file inside a secret folder:
// .env, .env.* (their .example excepted) and *.env, anything under .secret/,
// .secrets/ or a hub's keys folder (<DataDir>/hub/keys, the bus seeds),
// private keys and certificates (*.pem, *.key, *.p12, *.pfx, *.jks,
// *.keystore, id_rsa and its kin, never their .pub), token files (*.token)
// and credential files (.netrc, .pgpass, .git-credentials). The
// comparison ignores case.
func IsSecretPath(path string) bool {
	p := strings.ToLower(strings.TrimSpace(path))
	if p == "" {
		return false
	}
	p = strings.ReplaceAll(p, `\`, "/")
	segs := strings.Split(p, "/")
	// Drop empty and "." segments; keep the order.
	clean := segs[:0]
	for _, s := range segs {
		if s != "" && s != "." {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		return false
	}
	for i, s := range clean {
		if secretDirs[s] {
			return true
		}
		if s == "hub" && i+1 < len(clean) && clean[i+1] == "keys" {
			return true
		}
	}
	return isSecretName(clean[len(clean)-1])
}

func isSecretName(base string) bool {
	if base == ".env" {
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		return !strings.HasSuffix(base, ".example")
	}
	if strings.HasSuffix(base, ".env") {
		return true
	}
	if secretNames[base] {
		return true
	}
	return secretExts[filepath.Ext(base)]
}

// Refusal is the error a tool answers for a secret path.
func Refusal(tool, path string) error {
	return fmt.Errorf("%s: %s is a secret path - agents never read or write secret files (.env*, *.env, .secret/, keys, tokens)", tool, path)
}

// CommandSecretPath returns the first word of a shell command that names a
// secret path ("" when none). It is a lexical guard - a command can always
// build a path the words do not show - and it backs the allow-listed
// environment, which is what keeps a shell's secrets out of reach.
func CommandSecretPath(command string) string {
	for _, w := range commandWords.FindAllString(command, -1) {
		w = strings.Trim(w, `"'`+"`")
		w = strings.TrimPrefix(w, "@")
		if IsSecretPath(w) {
			return w
		}
	}
	return ""
}

// commandWords splits a command on blanks, quotes and shell operators.
var commandWords = regexp.MustCompile(`[^\s"'` + "`" + `;|&<>()=]+`)

// allowedEnv is the environment a child process of an agent inherits:
// what shells, toolchains and the OS need to run, nothing that carries a
// credential. Names compare upper-case (Windows names are case-free).
var allowedEnv = map[string]bool{
	// POSIX and terminals
	"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
	"TERM": true, "COLORTERM": true, "LANG": true, "LANGUAGE": true, "TZ": true,
	"TMPDIR": true, "PWD": true, "EDITOR": true, "PAGER": true,
	// Windows
	"SYSTEMROOT": true, "SYSTEMDRIVE": true, "WINDIR": true, "COMSPEC": true,
	"PATHEXT": true, "TEMP": true, "TMP": true, "USERPROFILE": true,
	"HOMEDRIVE": true, "HOMEPATH": true, "APPDATA": true, "LOCALAPPDATA": true,
	"PROGRAMDATA": true, "PROGRAMFILES": true, "PROGRAMFILES(X86)": true,
	"PROGRAMW6432": true, "COMMONPROGRAMFILES": true, "COMMONPROGRAMFILES(X86)": true,
	"COMMONPROGRAMW6432": true, "ALLUSERSPROFILE": true, "PUBLIC": true,
	"NUMBER_OF_PROCESSORS": true, "PROCESSOR_ARCHITECTURE": true,
	"PROCESSOR_IDENTIFIER": true, "OS": true, "USERNAME": true,
	"USERDOMAIN": true, "COMPUTERNAME": true,
	// Git Bash / MSYS
	"MSYSTEM": true, "MSYS": true, "MSYS2_PATH_TYPE": true, "CHERE_INVOKING": true,
	"MINGW_PREFIX": true, "EXEPATH": true,
	// Toolchains
	"GOPATH": true, "GOROOT": true, "GOCACHE": true, "GOMODCACHE": true,
	"GOFLAGS": true, "GOTOOLCHAIN": true, "GOPROXY": true, "GOPRIVATE": true,
	"GONOPROXY": true, "GONOSUMDB": true, "GOSUMDB": true, "CGO_ENABLED": true,
	"GOOS": true, "GOARCH": true, "BUN_INSTALL": true, "NODE_PATH": true,
	// Network (a proxy URL's password is scrubbed from results by redaction)
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
}

// allowedEnvPrefixes admit families of harmless names, minus any whose
// name looks like it holds a credential.
var allowedEnvPrefixes = []string{"LC_", "XDG_", "KTW_", "KATYOTI_"}

var credentialName = regexp.MustCompile(`(KEY|TOKEN|SECRET|PASSW|CREDENTIAL|AUTH|SEED|NKEY|PRIVATE)`)

// AgentEnv filters environ (os.Environ's "NAME=value" form) down to the
// allow-list: the environment an agent's shell, worker or command starts
// from. A connector key, an API key or any other variable not listed is
// dropped.
func AgentEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		if AllowedEnvName(name) {
			out = append(out, kv)
		}
	}
	return out
}

// AllowedEnvName reports whether a variable of that name may reach an
// agent's child process.
func AllowedEnvName(name string) bool {
	up := strings.ToUpper(name)
	if allowedEnv[up] {
		return true
	}
	for _, p := range allowedEnvPrefixes {
		if strings.HasPrefix(up, p) {
			return !credentialName.MatchString(up)
		}
	}
	return false
}
