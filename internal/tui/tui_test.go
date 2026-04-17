package tui

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilterEmptyQueryReturnsAll(t *testing.T) {
	got := Filter([]string{"a", "b", "c"}, "")
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("got %v", got)
	}
}

func TestFilterFuzzy(t *testing.T) {
	got := Filter([]string{"psp-dev", "psp-pt", "psp-prod", "other"}, "ppd")
	if len(got) == 0 || got[0] != "psp-dev" {
		t.Errorf("ppd: want psp-dev first, got %v", got)
	}

	got = Filter([]string{"psp-dev", "psp-pt", "psp-prod"}, "ppp")
	found := map[string]bool{}
	for _, s := range got {
		found[s] = true
	}
	if !found["psp-pt"] || !found["psp-prod"] {
		t.Errorf("ppp: want psp-pt and psp-prod in %v", got)
	}
	if found["psp-dev"] {
		t.Errorf("ppp: unexpected psp-dev in %v", got)
	}
}

func TestModelInitialMatchesQuery(t *testing.T) {
	m := newModel([]string{"psp-dev", "psp-pt", "psp-prod"}, "ppd")
	if len(m.matches) == 0 || m.matches[0] != "psp-dev" {
		t.Errorf("matches = %v", m.matches)
	}
}

func TestModelEnterSelects(t *testing.T) {
	m := newModel([]string{"psp-dev", "psp-prod"}, "")
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := mi.(model)
	if mm.selected != "psp-dev" {
		t.Errorf("selected = %q", mm.selected)
	}
	if mm.cancelled {
		t.Error("cancelled true after enter")
	}
}

func TestModelEscCancels(t *testing.T) {
	m := newModel([]string{"a"}, "")
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := mi.(model)
	if !mm.cancelled {
		t.Error("expected cancelled")
	}
}

func TestModelDownUpMoveCursor(t *testing.T) {
	m := newModel([]string{"a", "b", "c"}, "")
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
	m := newModel([]string{"a", "b"}, "")
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mi, _ = mi.(model).Update(tea.KeyMsg{Type: tea.KeyDown})
	mi, _ = mi.(model).Update(tea.KeyMsg{Type: tea.KeyDown})
	if mi.(model).cursor != 1 {
		t.Errorf("cursor = %d, want 1", mi.(model).cursor)
	}
}

func TestModelEnterOnEmptyMatchesDoesNotSelect(t *testing.T) {
	m := newModel([]string{"a"}, "")
	// Force no matches by swapping the match list.
	m.matches = nil
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if mi.(model).selected != "" {
		t.Error("should not select when no matches")
	}
}
