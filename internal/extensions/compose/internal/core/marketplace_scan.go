// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
	"github.com/katyoti/katyoti-fwk/tools/secret-scan/secretscan"
)

// scanArtefacts scans everything a build would publish before anything is
// written: the build's binaries for the definition's
// platforms, the config-sets, the seeds and their tasks, the solution cases
// and their documents, the change log. A secret path or a credential shape
// refuses every release; the terms of a deny-list only where the definition
// names one. KaTyoti's own binaries' library samples pass
// (secretscan.ReleaseAllow), with the definition's allow-list. Each problem
// names the file.
func scanArtefacts(def *authoring.Definition, dist, whatsNew string, noBuild bool) []string {
	var problems []string
	var deny []*regexp.Regexp
	if def.SecretScan.Deny != "" {
		d, err := secretscan.LoadDenyList(def.SecretScan.Deny)
		if err != nil {
			return []string{err.Error()}
		}
		deny = d
	}
	allow := secretscan.ReleaseAllow()
	if def.SecretScan.Allow != "" {
		a, err := secretscan.LoadAllowList(def.SecretScan.Allow)
		if err != nil {
			return []string{err.Error()}
		}
		allow = append(allow, a...)
	}
	var roots []string
	if !noBuild {
		for _, p := range def.Platforms {
			roots = append(roots, filepath.Join(dist, p))
		}
	}
	for _, c := range def.ConfigSets {
		roots = append(roots, c.Path)
	}
	for _, sd := range def.Seeds {
		roots = append(roots, sd.Path)
		if sd.Tasks != "" {
			roots = append(roots, sd.Tasks)
		}
	}
	for _, sol := range def.Solutions {
		roots = append(roots, sol.Case)
		if sol.Docs != "" {
			roots = append(roots, sol.Docs)
		}
	}
	if whatsNew != "" {
		roots = append(roots, whatsNew)
	}
	shapes := secretscan.NewShapes()
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			continue // a missing folder is already a problem of its own
		}
		found, err := secretscan.ScanTree(root, shapes, deny)
		if err != nil {
			problems = append(problems, fmt.Sprintf("secret scan of %s: %v", root, err))
			continue
		}
		for _, f := range secretscan.Filter(found, allow) {
			file := root
			if info.IsDir() {
				file = filepath.Join(root, filepath.FromSlash(f.Path))
			}
			where := file
			if f.Line > 0 {
				where = fmt.Sprintf("%s:%d", file, f.Line)
			}
			problems = append(problems, fmt.Sprintf("%s: %s, a secret: it cannot be published", where, f.Kind))
		}
	}
	return problems
}
