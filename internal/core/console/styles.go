// SPDX-License-Identifier: Apache-2.0

package console

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// =============================================================================
// Shared styles
// =============================================================================

var (
	TitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	LabelStyle  = lipgloss.NewStyle().Width(22).Foreground(lipgloss.Color("244"))
	ValueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	WarnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	OkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	ErrorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	DimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	BoxStyle    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)
)

// =============================================================================
// Operation symbols
// =============================================================================

// Sym holds styled operation indicators for CLI output.
var Sym = struct {
	Download string
	Upload   string
	Conflict string
	NoChange string
	Create   string
	Delete   string
	Warning  string
	OK       string
	Error    string
	Skip     string
}{
	Download: lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Render("<= "),
	Upload:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(" =>"),
	Conflict: lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render("<=>"),
	NoChange: lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("   "),
	Create:   lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render("[+]"),
	Delete:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("[-]"),
	Warning:  lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true).Render("/!\\"),
	OK:       lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("[x]"),
	Error:    lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("[!]"),
	Skip:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[.]"),
}

// =============================================================================
// Section helpers
// =============================================================================

// PrintHeader prints a styled section title with spacing.
func PrintHeader(title string) {
	fmt.Println()
	fmt.Println(TitleStyle.Render("  " + title))
	fmt.Println()
}

// PrintRow prints a label-value pair using shared styles.
func PrintRow(label, value string) {
	if value == "" {
		value = DimStyle.Render("(not set)")
	} else {
		value = ValueStyle.Render(value)
	}
	fmt.Println("  " + LabelStyle.Render(label) + value)
}

// PrintSymRow prints a symbol followed by a message.
func PrintSymRow(sym, message string) {
	fmt.Printf("  %-5s %s\n", sym, message)
}

// PrintDim prints dimmed informational text.
func PrintDim(msg string) {
	fmt.Println(DimStyle.Render("  " + msg))
}

// PrintWarn prints a warning message with the warning symbol.
func PrintWarn(msg string) {
	fmt.Printf("  %s %s\n", Sym.Warning, WarnStyle.Render(msg))
}

// PrintOK prints a success message.
func PrintOK(msg string) {
	fmt.Println(OkStyle.Render("  " + msg))
}

// PrintError prints an error message.
func PrintError(msg string) {
	fmt.Println(ErrorStyle.Render("  " + msg))
}

// =============================================================================
// Table printer
// =============================================================================

// Table renders columnar data with auto-computed widths.
type Table struct {
	headers   []string
	rows      [][]string
	colStyles map[int]lipgloss.Style
	indent    int
	separator string
	raw       bool // when true, skip ANSI styling
}

// NewTable creates a new table with the given column headers.
func NewTable(headers ...string) *Table {
	return &Table{
		headers:   headers,
		colStyles: make(map[int]lipgloss.Style),
		indent:    2,
		separator: "  ",
	}
}

// AddRow appends a row of values. Extra columns are ignored; missing columns are blank.
func (t *Table) AddRow(cols ...string) {
	row := make([]string, len(t.headers))
	for i := range row {
		if i < len(cols) {
			row[i] = cols[i]
		}
	}
	t.rows = append(t.rows, row)
}

// SetColumnStyle sets a lipgloss style for a specific column index.
func (t *Table) SetColumnStyle(col int, style lipgloss.Style) {
	t.colStyles[col] = style
}

// Render returns the formatted table string.
func (t *Table) Render() string {
	if len(t.headers) == 0 {
		return ""
	}

	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = len(h)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	indent := strings.Repeat(" ", t.indent)
	var sb strings.Builder

	var headerParts []string
	for i, h := range t.headers {
		headerParts = append(headerParts, fmt.Sprintf("%-*s", widths[i], h))
	}
	headerLine := strings.Join(headerParts, t.separator)
	if t.raw {
		sb.WriteString(indent + headerLine + "\n")
	} else {
		headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
		sb.WriteString(indent + headerStyle.Render(headerLine) + "\n")
	}

	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	totalWidth += len(t.separator) * (len(widths) - 1)
	separator := strings.Repeat("-", totalWidth)
	if !t.raw {
		separator = DimStyle.Render(strings.Repeat("─", totalWidth))
	}
	sb.WriteString(indent + separator + "\n")

	for _, row := range t.rows {
		var parts []string
		for i, cell := range row {
			formatted := fmt.Sprintf("%-*s", widths[i], cell)
			if !t.raw {
				if style, ok := t.colStyles[i]; ok {
					formatted = style.Render(formatted)
				}
			}
			parts = append(parts, formatted)
		}
		sb.WriteString(indent + strings.Join(parts, t.separator) + "\n")
	}

	return sb.String()
}

// Print renders and prints the table to stdout.
func (t *Table) Print() {
	fmt.Print(t.Render())
}

// =============================================================================
// Tree printer
// =============================================================================

// TreeNode represents one node in a printable tree hierarchy.
type TreeNode struct {
	Prefix   []string
	Label    string
	Children []*TreeNode
}

// NewTree creates a root tree node with the given label and optional prefix columns.
func NewTree(label string, prefix ...string) *TreeNode {
	return &TreeNode{Label: label, Prefix: prefix}
}

// Add adds a child node and returns it for chaining.
func (n *TreeNode) Add(label string, prefix ...string) *TreeNode {
	child := &TreeNode{Label: label, Prefix: prefix}
	n.Children = append(n.Children, child)
	return child
}

// Render returns the tree as a formatted string with box-drawing characters.
func (n *TreeNode) Render() string {
	var sb strings.Builder
	n.renderTo(&sb, "", true, true)
	return sb.String()
}

// Print renders and prints the tree to stdout.
func (n *TreeNode) Print() {
	fmt.Print(n.Render())
}

func (n *TreeNode) renderTo(sb *strings.Builder, linePrefix string, isLast bool, isRoot bool) {
	var branch string
	if isRoot {
		branch = ""
	} else if isLast {
		branch = "└── "
	} else {
		branch = "├── "
	}

	prefixStr := ""
	if len(n.Prefix) > 0 {
		prefixStr = strings.Join(n.Prefix, "  ") + "  "
	}

	sb.WriteString("  " + prefixStr + linePrefix + branch + n.Label + "\n")

	childPrefix := linePrefix
	if !isRoot {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range n.Children {
		child.renderTo(sb, childPrefix, i == len(n.Children)-1, false)
	}
}
