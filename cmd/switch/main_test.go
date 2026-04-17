package main

import (
	"strings"
	"testing"

	"github.com/xetius/switcher/internal/config"
)

func fixture() *config.Config {
	return &config.Config{
		Contexts: map[string]config.Context{
			"psp-dev":  {Profile: "PSPAdmin"},
			"psp-pt":   {Profile: "PSPAdmin"},
			"psp-prod": {Profile: "PSPAdmin"},
			"other":    {Profile: "Other"},
		},
	}
}

func TestTryDirectExactMatch(t *testing.T) {
	name, ok := tryDirect(fixture(), "psp-dev")
	if !ok || name != "psp-dev" {
		t.Errorf("name=%q ok=%v", name, ok)
	}
}

func TestTryDirectUniqueFuzzy(t *testing.T) {
	// "ppt" matches only psp-pt: psp-prod has no 't', psp-dev has no 't'.
	name, ok := tryDirect(fixture(), "ppt")
	if !ok || name != "psp-pt" {
		t.Errorf("name=%q ok=%v", name, ok)
	}
}

func TestTryDirectAmbiguousFuzzy(t *testing.T) {
	_, ok := tryDirect(fixture(), "ppp")
	if ok {
		t.Error("expected ambiguous, got direct hit")
	}
}

func TestTryDirectEmpty(t *testing.T) {
	_, ok := tryDirect(fixture(), "")
	if ok {
		t.Error("empty query must open picker")
	}
}

func TestTryDirectNoMatches(t *testing.T) {
	_, ok := tryDirect(fixture(), "zzz")
	if ok {
		t.Error("no matches must open picker (so user can see full list)")
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"simple":       "'simple'",
		"with space":   "'with space'",
		"it's":         `'it'\''s'`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShellInitContainsWrapper(t *testing.T) {
	s := shellInit()
	for _, want := range []string{
		"switch()",
		"command switch",
		"--help|-h|--version|--bash|--zsh",
		"eval",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("shellInit missing %q\n---\n%s", want, s)
		}
	}
}

func TestPrintHelpContainsKeyLines(t *testing.T) {
	var buf strings.Builder
	printHelp(&buf)
	s := buf.String()
	for _, want := range []string{"Usage:", "switch [query]", "--bash", "--zsh", "eval \"$(switch --zsh)\""} {
		if !strings.Contains(s, want) {
			t.Errorf("help missing %q", want)
		}
	}
}
