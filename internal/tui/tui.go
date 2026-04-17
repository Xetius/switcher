package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// Item is a single row in the picker: the Name the user matches against and a
// Group used to assign a color. Items sharing a Group share a color.
type Item struct {
	Name  string
	Group string
}

// Pick opens a fuzzy-finder prepopulated with initialQuery and returns the
// chosen item's Name, or "" if the user cancelled (Esc / Ctrl-C).
//
// Renders on stderr so that the binary's stdout stays clean for shell eval
// output.
func Pick(items []Item, initialQuery string) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no candidates to pick from")
	}
	// The shell wrapper captures our stdout via `out=$(command switch ...)`.
	// lipgloss defaults to probing os.Stdout for color support; a captured
	// stdout is not a TTY, so colors would be disabled even though we render
	// to stderr (which remains a TTY). Bind the renderer to stderr so color
	// detection uses the correct stream.
	lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(stderrWriter()))

	m := newModel(items, initialQuery)
	p := tea.NewProgram(m, tea.WithOutput(stderrWriter()), tea.WithInput(stdinReader()))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	fm := final.(model)
	if fm.cancelled {
		return "", nil
	}
	return fm.selected, nil
}

type model struct {
	input     textinput.Model
	all       []Item
	matches   []Item
	colors    map[string]lipgloss.Color
	cursor    int
	selected  string
	cancelled bool
	quitting  bool
}

func newModel(items []Item, initial string) model {
	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.SetValue(initial)
	ti.CursorEnd()
	ti.Focus()

	m := model{
		input:  ti,
		all:    items,
		colors: assignColors(items),
	}
	m.matches = Filter(items, initial)
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if len(m.matches) > 0 {
				m.selected = m.matches[m.cursor].Name
				m.quitting = true
				return m, tea.Quit
			}
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(m.matches)-1 {
				m.cursor++
			}
			return m, nil
		}
	}

	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.matches = Filter(m.all, m.input.Value())
		m.cursor = 0
	}
	return m, cmd
}

var stylePrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))

func (m model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(stylePrompt.Render("> "))
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	for i, it := range m.matches {
		nameStyle := lipgloss.NewStyle().Foreground(m.colors[it.Group])
		if i == m.cursor {
			b.WriteString(nameStyle.Bold(true).Render("▶ " + it.Name))
		} else {
			b.WriteString("  ")
			b.WriteString(nameStyle.Render(it.Name))
		}
		b.WriteString("\n")
	}
	if len(m.matches) == 0 {
		b.WriteString("  (no matches)\n")
	}
	return b.String()
}

// Filter ranks items by fuzzy match against query. Empty query returns items
// in their original order. Matching is against Name only.
func Filter(items []Item, query string) []Item {
	if strings.TrimSpace(query) == "" {
		out := make([]Item, len(items))
		copy(out, items)
		return out
	}
	names := make([]string, len(items))
	byName := make(map[string]Item, len(items))
	for i, it := range items {
		names[i] = it.Name
		byName[it.Name] = it
	}
	matches := fuzzy.Find(query, names)
	out := make([]Item, len(matches))
	for i, m := range matches {
		out[i] = byName[m.Str]
	}
	return out
}

// groupPalette is the ordered list of colors assigned to distinct groups in
// encounter order. Chosen to be readable on both dark and light terminals and
// to avoid red (which is usually reserved for errors).
var groupPalette = []lipgloss.Color{
	lipgloss.Color("86"),  // teal
	lipgloss.Color("213"), // pink
	lipgloss.Color("208"), // orange
	lipgloss.Color("51"),  // cyan
	lipgloss.Color("226"), // yellow
	lipgloss.Color("141"), // purple
	lipgloss.Color("118"), // lime
	lipgloss.Color("45"),  // sky
}

// defaultColor is used when an item has an empty Group (no coloring requested).
const defaultColor = lipgloss.Color("15") // bright white / default fg

// assignColors returns a group → color map, assigning palette entries in the
// order groups are first seen. Wraps around the palette if there are more
// groups than colors.
func assignColors(items []Item) map[string]lipgloss.Color {
	out := make(map[string]lipgloss.Color)
	next := 0
	for _, it := range items {
		if it.Group == "" {
			out[""] = defaultColor
			continue
		}
		if _, ok := out[it.Group]; ok {
			continue
		}
		out[it.Group] = groupPalette[next%len(groupPalette)]
		next++
	}
	return out
}
