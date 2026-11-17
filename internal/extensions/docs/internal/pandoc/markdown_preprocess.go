// SPDX-License-Identifier: Apache-2.0

package pandoc

import (
	"fmt"
	"regexp"
	"strings"
)

// PreprocessMarkdown applies all text-level preprocessing passes before Pandoc.
// Diagram preprocessing (PreprocessDiagrams) is handled separately because it
// requires a Kroki client and I/O.
func PreprocessMarkdown(input string) string {
	input = preprocessGitHubAlerts(input)
	input = preprocessMkDocsAdmonitions(input)
	input = preprocessInline(input)
	return input
}

// --- GitHub Alerts ---

// githubAlertRE matches > [!NOTE], > [!WARNING], etc.
var githubAlertRE = regexp.MustCompile(`^>\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`)

// githubAlertToDivClass maps GitHub alert types to Pandoc fenced div classes.
var githubAlertToDivClass = map[string]string{
	"note":      "note",
	"tip":       "tip",
	"important": "important",
	"warning":   "warning",
	"caution":   "caution",
}

// preprocessGitHubAlerts converts GitHub-style blockquote alerts to Pandoc
// fenced divs with a title.
func preprocessGitHubAlerts(input string) string {
	lines := strings.Split(input, "\n")
	var output []string
	i := 0
	for i < len(lines) {
		m := githubAlertRE.FindStringSubmatch(strings.TrimRight(lines[i], " \t"))
		if m == nil {
			output = append(output, lines[i])
			i++
			continue
		}

		alertType := strings.ToLower(m[1])
		divClass := githubAlertToDivClass[alertType]
		if divClass == "" {
			divClass = "note"
		}
		title := strings.ToUpper(alertType[:1]) + alertType[1:]

		output = append(output, fmt.Sprintf(`::: {.%s title="%s"}`, divClass, title))

		i++
		// Collect blockquote body lines, stripping the > prefix.
		for i < len(lines) {
			trimmed := strings.TrimRight(lines[i], " \t")
			if strings.HasPrefix(trimmed, "> ") {
				output = append(output, trimmed[2:])
				i++
			} else if trimmed == ">" {
				output = append(output, "")
				i++
			} else {
				break
			}
		}

		output = append(output, ":::")
		output = append(output, "")
	}
	return strings.Join(output, "\n")
}

// --- MkDocs Admonitions ---

var mkdocsAdmonitionRE = regexp.MustCompile(`^!!!\s+(\w+)\s*(?:"([^"]*)")?\s*$`)
var mkdocsCollapsibleRE = regexp.MustCompile(`^\?\?\?\s+(\w+)\s*(?:"([^"]*)")?\s*$`)

func preprocessMkDocsAdmonitions(input string) string {
	lines := strings.Split(input, "\n")
	var output []string
	i := 0
	for i < len(lines) {
		if m := mkdocsCollapsibleRE.FindStringSubmatch(lines[i]); m != nil {
			admonType := m[1]
			title := m[2]
			if title == "" {
				title = strings.ToUpper(admonType[:1]) + admonType[1:]
			}

			output = append(output, "<details>")
			output = append(output, fmt.Sprintf("<summary>%s</summary>", title))
			output = append(output, "")

			i++
			body := collectIndentedBody(lines, &i)
			output = append(output, body...)

			output = append(output, "</details>")
			output = append(output, "")
			continue
		}

		if m := mkdocsAdmonitionRE.FindStringSubmatch(lines[i]); m != nil {
			admonType := m[1]
			title := m[2]
			if title == "" {
				title = strings.ToUpper(admonType[:1]) + admonType[1:]
			}

			output = append(output, fmt.Sprintf(`::: {.%s title="%s"}`, admonType, title))

			i++
			body := collectIndentedBody(lines, &i)
			output = append(output, body...)

			output = append(output, ":::")
			output = append(output, "")
			continue
		}

		output = append(output, lines[i])
		i++
	}
	return strings.Join(output, "\n")
}

func collectIndentedBody(lines []string, i *int) []string {
	var body []string
	for *i < len(lines) {
		if strings.HasPrefix(lines[*i], "    ") {
			body = append(body, lines[*i][4:])
			*i++
		} else if strings.HasPrefix(lines[*i], "\t") {
			body = append(body, lines[*i][1:])
			*i++
		} else if strings.TrimSpace(lines[*i]) == "" {
			body = append(body, "")
			*i++
		} else {
			break
		}
	}
	return body
}

// --- Inline Preprocessing ---

func preprocessInline(input string) string {
	lines := strings.Split(input, "\n")
	var output []string
	inCodeFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inCodeFence = !inCodeFence
			output = append(output, line)
			continue
		}
		if inCodeFence {
			output = append(output, line)
			continue
		}
		line = replaceEmoji(line)
		line = replaceStatusBadge(line)
		line = replaceHighlight(line)
		output = append(output, line)
	}
	return strings.Join(output, "\n")
}

// --- Emoji ---

var emojiMap = map[string]string{
	"smile":                    "\U0001F60A",
	"sad":                      "\U0001F622",
	"tongue":                   "\U0001F61B",
	"grin":                     "\U0001F600",
	"wink":                     "\U0001F609",
	"thumbsup":                 "\U0001F44D",
	"thumbs_up":                "\U0001F44D",
	"+1":                       "\U0001F44D",
	"thumbsdown":               "\U0001F44E",
	"thumbs_down":              "\U0001F44E",
	"-1":                       "\U0001F44E",
	"information_source":       "\u2139\uFE0F",
	"info":                     "\u2139\uFE0F",
	"white_check_mark":         "\u2705",
	"check_mark":               "\u2705",
	"heavy_check_mark":         "\u2705",
	"x":                        "\u274C",
	"cross_mark":               "\u274C",
	"warning":                  "\u26A0\uFE0F",
	"heavy_plus_sign":          "\u2795",
	"heavy_minus_sign":         "\u2796",
	"question":                 "\u2753",
	"bulb":                     "\U0001F4A1",
	"star":                     "\u2B50",
	"heart":                    "\u2764\uFE0F",
	"broken_heart":             "\U0001F494",
	"rocket":                   "\U0001F680",
	"fire":                     "\U0001F525",
	"tada":                     "\U0001F389",
	"eyes":                     "\U0001F440",
	"thinking":                 "\U0001F914",
	"100":                      "\U0001F4AF",
	"clap":                     "\U0001F44F",
	"wave":                     "\U0001F44B",
	"pray":                     "\U0001F64F",
	"muscle":                   "\U0001F4AA",
	"bug":                      "\U0001F41B",
	"memo":                     "\U0001F4DD",
	"lock":                     "\U0001F512",
	"key":                      "\U0001F511",
	"link":                     "\U0001F517",
	"gear":                     "\u2699\uFE0F",
	"hammer":                   "\U0001F528",
	"wrench":                   "\U0001F527",
	"package":                  "\U0001F4E6",
	"clipboard":                "\U0001F4CB",
	"calendar":                 "\U0001F4C5",
	"chart_with_upwards_trend": "\U0001F4C8",
	"mag":                      "\U0001F50D",
	"bell":                     "\U0001F514",
	"email":                    "\U0001F4E7",
	"hourglass":                "\u231B",
	"stopwatch":                "\u23F1\uFE0F",
	"zap":                      "\u26A1",
	"sparkles":                 "\u2728",
	"construction":             "\U0001F6A7",
	"no_entry":                 "\u26D4",
	"pencil":                   "\u270F\uFE0F",
	"books":                    "\U0001F4DA",
	"green_circle":             "\U0001F7E2",
	"red_circle":               "\U0001F534",
	"yellow_circle":            "\U0001F7E1",
	"blue_circle":              "\U0001F535",
	"white_circle":             "\u26AA",
	"black_circle":             "\u26AB",
	"arrow_right":              "\u27A1\uFE0F",
	"arrow_left":               "\u2B05\uFE0F",
	"arrow_up":                 "\u2B06\uFE0F",
	"arrow_down":               "\u2B07\uFE0F",
}

var emojiRE = regexp.MustCompile(`:([a-z0-9][a-z0-9_+-]*):{1}`)

func replaceEmoji(line string) string {
	return emojiRE.ReplaceAllStringFunc(line, func(match string) string {
		shortcode := match[1 : len(match)-1]
		if emoji, ok := emojiMap[shortcode]; ok {
			return emoji
		}
		return match
	})
}

// --- Status Badge ---

var statusBadgeRE = regexp.MustCompile(`(?i)!\[([^\]]*)\]\[status-(blue|green|yellow|red|purple|grey|gray)\]`)

var statusColorCSS = map[string][2]string{
	"blue":   {"#0052CC", "#FFFFFF"},
	"green":  {"#00875A", "#FFFFFF"},
	"yellow": {"#FF991F", "#172B4D"},
	"red":    {"#DE350B", "#FFFFFF"},
	"purple": {"#6554C0", "#FFFFFF"},
	"grey":   {"#97A0AF", "#172B4D"},
	"gray":   {"#97A0AF", "#172B4D"},
}

func replaceStatusBadge(line string) string {
	return statusBadgeRE.ReplaceAllStringFunc(line, func(match string) string {
		m := statusBadgeRE.FindStringSubmatch(match)
		title := m[1]
		color := strings.ToLower(m[2])
		css := statusColorCSS[color]
		return fmt.Sprintf(
			`<span style="background-color: %s; color: %s; padding: 2px 8px; border-radius: 3px; font-size: 0.85em; font-weight: bold; text-transform: uppercase;">%s</span>`,
			css[0], css[1], title)
	})
}

// --- Highlighted Text ---

var highlightRE = regexp.MustCompile(`==([^=]+)==`)

func replaceHighlight(line string) string {
	return highlightRE.ReplaceAllString(line, `<mark>$1</mark>`)
}
