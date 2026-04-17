package awsx

import (
	"context"
	"errors"
	"testing"
)

func installProbe(t *testing.T, fn func(context.Context, string) error) {
	t.Helper()
	orig := probe
	probe = fn
	t.Cleanup(func() { probe = orig })
}

func installLogin(t *testing.T, fn func(context.Context, string) error) {
	t.Helper()
	orig := login
	login = fn
	t.Cleanup(func() { login = orig })
}

func TestSessionValid(t *testing.T) {
	installProbe(t, func(context.Context, string) error { return nil })
	if !SessionValid("p") {
		t.Fatal("expected valid")
	}
}

func TestSessionInvalid(t *testing.T) {
	installProbe(t, func(context.Context, string) error { return errors.New("expired") })
	if SessionValid("p") {
		t.Fatal("expected invalid")
	}
}

func TestSessionValidPassesProfile(t *testing.T) {
	var got string
	installProbe(t, func(_ context.Context, profile string) error {
		got = profile
		return nil
	})
	SessionValid("ABCAdmin")
	if got != "ABCAdmin" {
		t.Errorf("profile = %q", got)
	}
}

func TestSSOLoginSkipsWhenValid(t *testing.T) {
	installProbe(t, func(context.Context, string) error { return nil })
	called := false
	installLogin(t, func(context.Context, string) error { called = true; return nil })

	if err := SSOLogin("p"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("expected login not to run when session valid")
	}
}

func TestSSOLoginRunsWhenInvalid(t *testing.T) {
	installProbe(t, func(context.Context, string) error { return errors.New("no") })
	var gotProfile string
	installLogin(t, func(_ context.Context, profile string) error {
		gotProfile = profile
		return nil
	})

	if err := SSOLogin("ABCAdmin"); err != nil {
		t.Fatal(err)
	}
	if gotProfile != "ABCAdmin" {
		t.Errorf("login called with profile=%q, want ABCAdmin", gotProfile)
	}
}

func TestSSOLoginErrorsWithoutProfile(t *testing.T) {
	if err := SSOLogin(""); err == nil {
		t.Fatal("expected error for empty profile")
	}
}
