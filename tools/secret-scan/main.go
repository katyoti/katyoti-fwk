// SPDX-License-Identifier: Apache-2.0

// Command secret-scan refuses a commit that adds a secret: the
// pre-commit hook (.githooks/pre-commit) runs it on the staged change.
//
// It reads the staged change from git (secretscan.Change, Scan) - the paths added, copied, modified or
// renamed, and the lines added only (git diff --cached --text -U0), so a
// file's untouched lines never trip it; --text reads a binary file's lines
// too, so a key behind a NUL byte is seen, and --no-textconv reads the stored
// bytes, never a .gitattributes converter's output - and answers one line per finding,
// "path[:line]: <kind>", exiting 1 when there is any:
//   - a staged path the one secret-path list names (secrets.IsSecretPath:
//     .env*, *.env, .secret/, keys, certificates, token and credential files);
//   - an added line holding a credential shape (internal/core/redact's
//     patterns: provider and cloud keys, tokens, JWTs, private-key headers,
//     URL passwords).
//
// A line carrying the marker "secret-scan: fake" is a test fixture and passes.
// No pattern is defined here: both lists are read from their one owner. The
// environment's values are not compared (redact's env layer would match the
// working folder or the author's name), only the shapes.
//
// It asks git for the cached diff, so it honours GIT_INDEX_FILE - the
// temporary-index commits the agents make.
//
// With -dir it scans a folder of artefacts instead (the
// release artefacts): every file, binaries read as bytes, for secret paths,
// credential shapes and the terms of the deny-list -deny names
// (secretscan.ScanTree, which compose's marketplace build calls too).
//
// A built binary carries its libraries' samples and messages, some shaped
// like a credential: -allow names the ones that pass, by path and kind.
//
// Usage: secret-scan [-commit <rev>] | -dir <folder> [-deny <file>] [-allow <file>]
//
//	-commit <rev>  scan what that commit added against its first parent
//	               instead of the staged change (to try the scanner on history)
//	-dir <folder>  scan every file under the folder (or that one file)
//	-deny <file>   with -dir: one case-insensitive regular expression per line,
//	               # the reason (secretscan.ListLine); a term found is printed
//	               as its line (deny-list term deny-list.txt line 6), never in clear
//	-allow <file>  with -dir: one "<glob> <kind>" per line, # the reason
//
// Exit: 0 clean, 1 a secret found, 2 git, the folder or a list could
// not be read.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/katyoti/katyoti-fwk/tools/secret-scan/secretscan"
)

func main() {
	commit := flag.String("commit", "", "scan that commit's change against its first parent instead of the staged one")
	dir := flag.String("dir", "", "scan every file under this folder (or this file) instead of a change")
	deny := flag.String("deny", "", "with -dir: a deny-list file, one case-insensitive regular expression per line")
	allow := flag.String("allow", "", "with -dir: an allow-list file, one \"<glob> <kind>\" per line")
	flag.Parse()

	var findings []secretscan.Finding
	if *dir != "" {
		var terms []*regexp.Regexp
		var refs map[string]string
		var err error
		if *deny != "" {
			if terms, refs, err = secretscan.LoadDenyListRefs(*deny); err != nil {
				fmt.Fprintln(os.Stderr, "secret-scan:", err)
				os.Exit(2)
			}
		}
		var allows []secretscan.Allow
		if *allow != "" {
			if allows, err = secretscan.LoadAllowList(*allow); err != nil {
				fmt.Fprintln(os.Stderr, "secret-scan:", err)
				os.Exit(2)
			}
		}
		if findings, err = secretscan.ScanTree(*dir, secretscan.NewShapes(), terms); err != nil {
			fmt.Fprintln(os.Stderr, "secret-scan:", err)
			os.Exit(2)
		}
		// A deny-list term is printed as its line, never in clear.
		findings = secretscan.RefDenyTerms(secretscan.Filter(findings, allows), refs)
	} else {
		if *deny != "" || *allow != "" {
			fmt.Fprintln(os.Stderr, "secret-scan: -deny and -allow go with -dir")
			os.Exit(2)
		}
		paths, diff, err := secretscan.Change(".", *commit)
		if err != nil {
			fmt.Fprintln(os.Stderr, "secret-scan:", err)
			os.Exit(2)
		}
		findings = secretscan.Scan(paths, secretscan.ParseAdded(diff), secretscan.NewShapes())
	}
	for _, f := range findings {
		fmt.Println(f)
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}
