// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// whatsNewSkipped are change-log sections that are not changes for a reader.
var whatsNewSkipped = map[string]bool{"Réserves": true}

var entryTitle = regexp.MustCompile(`^### \d+\. (.+)$`)

// WhatsNewMarkdown turns a version's change log (docs/confluence-tew/changelog/
// <version>.md) into « Quoi de neuf dans <version> » — never written by hand:
// « En un coup d'œil » as it is, then each domain's changes, each with what it
// brings. It answers the version (the file's name) and the markdown.
func WhatsNewMarkdown(changelog string) (string, []byte, error) {
	data, err := os.ReadFile(changelog)
	if err != nil {
		return "", nil, fmt.Errorf("the change log %s: %w", changelog, err)
	}
	version := strings.TrimSuffix(filepath.Base(changelog), filepath.Ext(changelog))
	var glance, body strings.Builder
	section, inEntry := "", false
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			section, inEntry = strings.TrimSpace(strings.TrimPrefix(line, "## ")), false
			if section != "En un coup d'œil" && !whatsNewSkipped[section] {
				fmt.Fprintf(&body, "\n## %s\n", section)
			}
		case section == "En un coup d'œil":
			if strings.TrimSpace(line) != "---" {
				glance.WriteString(line + "\n")
			}
		case section == "" || whatsNewSkipped[section]:
		case entryTitle.MatchString(line):
			fmt.Fprintf(&body, "\n### %s\n", entryTitle.FindStringSubmatch(line)[1])
			inEntry = true
		case inEntry && strings.HasPrefix(line, "**Ce que ça vous apporte :**"):
			fmt.Fprintf(&body, "\n%s\n", strings.TrimSpace(strings.TrimPrefix(line, "**Ce que ça vous apporte :**")))
			inEntry = false
		}
	}
	md := fmt.Sprintf("# Quoi de neuf dans %s\n\n%s%s", version, strings.TrimSpace(glance.String())+"\n", body.String())
	return version, []byte(md), nil
}
