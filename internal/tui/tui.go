package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// Pick opens a fuzzy-finder prepopulated with initialQuery and returns the
// chosen candidate, or "" if the user cancelled (Esc / Ctrl-C).
//
// Renders on stderr so that the binary's stdout stays clean for shell eval
// output.
func Pick(candidates []string, initialQuery string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no candidates to pick from")
	}
	m := newModel(candidates, initialQuery)
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
	all       []string
	matches   []string
	cursor    int
	selected  string
	cancelled bool
	quitting  bool
}

func newModel(candidates []string, initial string) model {
	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.SetValue(initial)
	ti.CursorEnd()
	ti.Focus()

	m := model{input: ti, all: candidates}
	m.matches = Filter(candidates, initial)
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
				m.selected = m.matches[m.cursor]
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

var (
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	stylePrompt   = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
)

func (m model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(stylePrompt.Render("> "))
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	for i, n := range m.matches {
		if i == m.cursor {
			b.WriteString(styleSelected.Render("▶ " + n))
		} else {
			b.WriteString("  " + n)
		}
		b.WriteString("\n")
	}
	if len(m.matches) == 0 {
		b.WriteString("  (no matches)\n")
	}
	return b.String()
}

// Filter ranks candidates by fuzzy match against query. Empty query returns
// all candidates in their original order.
func Filter(candidates []string, query string) []string {
	if strings.TrimSpace(query) == "" {
		out := make([]string, len(candidates))
		copy(out, candidates)
		return out
	}
	matches := fuzzy.Find(query, candidates)
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.Str
	}
	return out
}
