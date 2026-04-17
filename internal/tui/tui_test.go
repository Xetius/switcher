package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func itemsOf(names ...string) []Item {
	out := make([]Item, len(names))
	for i, n := range names {
		out[i] = Item{Name: n}
	}
	return out
}

func names(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Name
	}
	return out
}

func TestFilterEmptyQueryReturnsAll(t *testing.T) {
	got := Filter(itemsOf("a", "b", "c"), "")
	if ns := names(got); len(ns) != 3 || ns[0] != "a" || ns[2] != "c" {
		t.Errorf("got %v", ns)
	}
}

func TestFilterFuzzy(t *testing.T) {
	// "ade" uniquely matches abc-dev: abc-prod has no 'e', abc-pt has no 'd'.
	got := Filter(itemsOf("abc-dev", "abc-pt", "abc-prod", "other"), "ade")
	if len(got) == 0 || got[0].Name != "abc-dev" {
		t.Errorf("ade: want abc-dev first, got %v", names(got))
	}

	// "ap" matches abc-pt and abc-prod, excludes abc-dev (no 'p').
	got = Filter(itemsOf("abc-dev", "abc-pt", "abc-prod"), "ap")
	found := map[string]bool{}
	for _, it := range got {
		found[it.Name] = true
	}
	if !found["abc-pt"] || !found["abc-prod"] {
		t.Errorf("ap: want abc-pt and abc-prod in %v", names(got))
	}
	if found["abc-dev"] {
		t.Errorf("ap: unexpected abc-dev in %v", names(got))
	}
}

func TestModelInitialMatchesQuery(t *testing.T) {
	m := newModel(itemsOf("abc-dev", "abc-pt", "abc-prod"), "ade")
	if len(m.matches) == 0 || m.matches[0].Name != "abc-dev" {
		t.Errorf("matches = %v", names(m.matches))
	}
}

func TestModelEnterSelects(t *testing.T) {
	m := newModel(itemsOf("abc-dev", "abc-prod"), "")
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := mi.(model)
	if mm.selected != "abc-dev" {
		t.Errorf("selected = %q", mm.selected)
	}
	if mm.cancelled {
		t.Error("cancelled true after enter")
	}
}

func TestModelEscCancels(t *testing.T) {
	m := newModel(itemsOf("a"), "")
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !mi.(model).cancelled {
		t.Error("expected cancelled")
	}
}

func TestModelDownUpMoveCursor(t *testing.T) {
	m := newModel(itemsOf("a", "b", "c"), "")
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mi, _ = mi.(model).Update(tea.KeyMsg{Type: tea.KeyDown})
	if mi.(model).cursor != 2 {
		t.Errorf("cursor = %d", mi.(model).cursor)
	}
	mi, _ = mi.(model).Update(tea.KeyMsg{Type: tea.KeyUp})
	if mi.(model).cursor != 1 {
		t.Errorf("cursor = %d", mi.(model).cursor)
	}
}

func TestModelDownClampsAtEnd(t *testing.T) {
	m := newModel(itemsOf("a", "b"), "")
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mi, _ = mi.(model).Update(tea.KeyMsg{Type: tea.KeyDown})
	mi, _ = mi.(model).Update(tea.KeyMsg{Type: tea.KeyDown})
	if mi.(model).cursor != 1 {
		t.Errorf("cursor = %d, want 1", mi.(model).cursor)
	}
}

func TestModelEnterOnEmptyMatchesDoesNotSelect(t *testing.T) {
	m := newModel(itemsOf("a"), "")
	m.matches = nil
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if mi.(model).selected != "" {
		t.Error("should not select when no matches")
	}
}

func TestAssignColorsSharesWithinGroup(t *testing.T) {
	items := []Item{
		{Name: "abc-dev", Group: "ABCAdmin"},
		{Name: "abc-prod", Group: "ABCAdmin"},
		{Name: "other-dev", Group: "Other"},
	}
	colors := assignColors(items)

	if colors["ABCAdmin"] == "" {
		t.Fatal("ABCAdmin has no color")
	}
	if colors["Other"] == "" {
		t.Fatal("Other has no color")
	}
	if colors["ABCAdmin"] == colors["Other"] {
		t.Error("distinct groups must have distinct colors")
	}
}

func TestAssignColorsDeterministicByOrder(t *testing.T) {
	a := []Item{{Name: "x", Group: "A"}, {Name: "y", Group: "B"}}
	b := []Item{{Name: "x", Group: "A"}, {Name: "y", Group: "B"}}
	if assignColors(a)["A"] != assignColors(b)["A"] {
		t.Error("A must get the same color each run")
	}
	if assignColors(a)["B"] != assignColors(b)["B"] {
		t.Error("B must get the same color each run")
	}
}

func TestAssignColorsWrapsPalette(t *testing.T) {
	// Build more groups than the palette has entries; ensure we still get a
	// color for every group (wrapping is fine).
	var items []Item
	for i := 0; i < len(groupPalette)+3; i++ {
		items = append(items, Item{Name: string(rune('a' + i)), Group: string(rune('A' + i))})
	}
	colors := assignColors(items)
	for _, it := range items {
		if colors[it.Group] == lipgloss.Color("") {
			t.Errorf("group %q missing color", it.Group)
		}
	}
}

func TestAssignColorsEmptyGroupUsesDefault(t *testing.T) {
	colors := assignColors([]Item{{Name: "x"}})
	if colors[""] != defaultColor {
		t.Errorf("empty group color = %q, want %q", colors[""], defaultColor)
	}
}
