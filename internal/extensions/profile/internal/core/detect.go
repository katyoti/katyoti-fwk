// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os/exec"
	"os/user"
	"strings"
	"time"
	"unicode"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// DetectOS suggests a profile from the identity of the OS user running this
// process: the account name gives the handle, the display name the first and
// last name, and every email source this OS offers a candidate. Read-only; a
// source that is missing or fails is skipped.
func DetectOS(ctx context.Context) api.Suggestion {
	var s api.Suggestion
	if u, err := user.Current(); err == nil {
		s.ID = handleFrom(u.Username)
		s.FirstName, s.LastName = splitDisplayName(u.Name)
	}
	var emails []api.EmailCandidate
	emails = append(emails, osEmails()...) // Windows: Office identity, UPN
	if e := gitConfig(ctx, "user.email"); e != "" {
		emails = append(emails, api.EmailCandidate{Email: e, Source: "git"})
	}
	if s.FirstName == "" && s.LastName == "" {
		s.FirstName, s.LastName = splitDisplayName(gitConfig(ctx, "user.name"))
	}
	s.Emails = dedupeEmails(emails)
	return s
}

// handleFrom derives a handle from an account name: `CORP\JDOE` and
// `jdoe@corp.example` → `jdoe`, `Jean.Dupont` → `jean-dupont`. Returns ""
// when nothing valid remains (the user then types one).
func handleFrom(account string) string {
	if i := strings.LastIndex(account, `\`); i >= 0 {
		account = account[i+1:]
	}
	if i := strings.Index(account, "@"); i >= 0 {
		account = account[:i]
	}
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(account) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			hyphen = false
		case b.Len() > 0 && !hyphen:
			b.WriteByte('-')
			hyphen = true
		}
	}
	h := strings.TrimRight(b.String(), "-")
	if len(h) > 63 {
		h = strings.TrimRight(h[:63], "-")
	}
	if api.ValidateHandle(h) != nil {
		return ""
	}
	return h
}

// splitDisplayName splits a display name into first and last name. The AD
// convention writes the surname in capitals, in either position
// (`Christophe STROHL`, `DE LA FONTAINE Jean`): when some words are all
// capitals and some are not, the capital words are the surname. Otherwise the
// first word is the first name and the rest the last name.
func splitDisplayName(name string) (first, last string) {
	words := strings.Fields(name)
	var upper, other []string
	for _, w := range words {
		if isAllCaps(w) {
			upper = append(upper, titleWord(w))
		} else {
			other = append(other, w)
		}
	}
	if len(upper) > 0 && len(other) > 0 {
		return strings.Join(other, " "), strings.Join(upper, " ")
	}
	switch len(words) {
	case 0:
		return "", ""
	case 1:
		return words[0], ""
	}
	return words[0], strings.Join(words[1:], " ")
}

// isAllCaps: at least two letters, none lowercase.
func isAllCaps(w string) bool {
	letters := 0
	for _, r := range w {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 2
}

// titleWord: `STROHL` → `Strohl`, `LE-GALL` → `Le-Gall`.
func titleWord(w string) string {
	parts := strings.Split(strings.ToLower(w), "-")
	for i, p := range parts {
		r := []rune(p)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		parts[i] = string(r)
	}
	return strings.Join(parts, "-")
}

// dedupeEmails keeps the first occurrence of each address (case-insensitive),
// lowercased — mailbox names are matched case-insensitively in practice, and
// AD sources shout them.
func dedupeEmails(in []api.EmailCandidate) []api.EmailCandidate {
	seen := map[string]bool{}
	var out []api.EmailCandidate
	for _, c := range in {
		e := strings.ToLower(strings.TrimSpace(c.Email))
		if e == "" || seen[e] || api.ValidateEmail(e) != nil {
			continue
		}
		seen[e] = true
		out = append(out, api.EmailCandidate{Email: e, Source: c.Source})
	}
	return out
}

// gitConfig reads one global git setting; "" when git is absent or unset.
func gitConfig(ctx context.Context, key string) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "config", "--global", key)
	publish.NoConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
