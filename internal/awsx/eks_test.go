package awsx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func installDescribe(t *testing.T, fn func(context.Context, string, string, string) (string, string, error)) {
	t.Helper()
	orig := describeCluster
	describeCluster = fn
	t.Cleanup(func() { describeCluster = orig })
}

func installExecCommand(t *testing.T, fn func() string) {
	t.Helper()
	orig := execCommand
	execCommand = fn
	t.Cleanup(func() { execCommand = orig })
}

func TestUpdateKubeconfigWritesNewFile(t *testing.T) {
	dir := t.TempDir()
	kpath := filepath.Join(dir, "config")
	t.Setenv("KUBECONFIG", kpath)

	installDescribe(t, func(context.Context, string, string, string) (string, string, error) {
		return "https://example.eks.amazonaws.com", "CERT", nil
	})
	installExecCommand(t, func() string { return "switch" })

	if err := UpdateKubeconfig("psp-dev", "psp-dev-eks", "eu-west-1", "PSPAdmin"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(kpath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	for _, want := range []string{
		"name: psp-dev",
		"server: https://example.eks.amazonaws.com",
		"certificate-authority-data: CERT",
		"apiVersion: client.authentication.k8s.io/v1beta1",
		"command: switch",
		"eks-token",
		"--cluster",
		"psp-dev-eks",
		"--region",
		"eu-west-1",
		"--profile",
		"PSPAdmin",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("kubeconfig missing %q\n---\n%s", want, s)
		}
	}
}

func TestExecCommandPrefersPath(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "switch")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	// Reset execCommand to the production impl for this test.
	orig := execCommand
	t.Cleanup(func() { execCommand = orig })
	execCommand = func() string {
		// copy of the production body to avoid re-exporting
		if _, err := exec.LookPath("switch"); err == nil {
			return "switch"
		}
		if exe, err := os.Executable(); err == nil {
			return exe
		}
		return "switch"
	}

	if got := execCommand(); got != "switch" {
		t.Errorf("execCommand() = %q, want %q", got, "switch")
	}
}

func TestExecCommandFallsBackToAbsPath(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	orig := execCommand
	t.Cleanup(func() { execCommand = orig })
	execCommand = func() string {
		if _, err := exec.LookPath("switch"); err == nil {
			return "switch"
		}
		if exe, err := os.Executable(); err == nil {
			return exe
		}
		return "switch"
	}

	got := execCommand()
	if got == "switch" {
		t.Errorf("expected absolute path fallback, got %q", got)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("fallback should be absolute path, got %q", got)
	}
}

func TestUpdateKubeconfigMergesIntoExistingFile(t *testing.T) {
	dir := t.TempDir()
	kpath := filepath.Join(dir, "config")
	t.Setenv("KUBECONFIG", kpath)

	existing := `apiVersion: v1
kind: Config
current-context: other
contexts:
- name: other
  context:
    cluster: other
    user: other
clusters:
- name: other
  cluster:
    server: https://other
users:
- name: other
  user:
    token: xxx
`
	if err := os.WriteFile(kpath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	installDescribe(t, func(context.Context, string, string, string) (string, string, error) {
		return "https://new.eks", "CA", nil
	})

	if err := UpdateKubeconfig("psp-dev", "psp-dev-eks", "eu-west-1", "PSPAdmin"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(kpath)
	s := string(data)
	if !strings.Contains(s, "name: other") {
		t.Error("existing 'other' context was dropped")
	}
	if !strings.Contains(s, "name: psp-dev") {
		t.Error("new context not added")
	}
	if !strings.Contains(s, "current-context: other") {
		t.Error("current-context was overwritten")
	}
}

func TestUpdateKubeconfigReplacesExistingEntry(t *testing.T) {
	dir := t.TempDir()
	kpath := filepath.Join(dir, "config")
	t.Setenv("KUBECONFIG", kpath)

	existing := `apiVersion: v1
kind: Config
clusters:
- name: psp-dev
  cluster:
    server: https://stale
contexts:
- name: psp-dev
  context:
    cluster: psp-dev
    user: psp-dev
users:
- name: psp-dev
  user:
    token: stale
`
	os.WriteFile(kpath, []byte(existing), 0o600)

	installDescribe(t, func(context.Context, string, string, string) (string, string, error) {
		return "https://fresh", "CA", nil
	})

	if err := UpdateKubeconfig("psp-dev", "psp-dev-eks", "eu-west-1", "PSPAdmin"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(kpath)
	s := string(data)
	if strings.Contains(s, "https://stale") {
		t.Error("stale server URL not replaced")
	}
	if !strings.Contains(s, "https://fresh") {
		t.Error("fresh server URL missing")
	}
	if strings.Contains(s, "token: stale") {
		t.Error("stale user token not replaced by exec plugin")
	}
}

func TestUpdateKubeconfigRequiresFields(t *testing.T) {
	installDescribe(t, func(context.Context, string, string, string) (string, string, error) {
		return "https://x", "CA", nil
	})
	if err := UpdateKubeconfig("", "c", "r", "p"); err == nil {
		t.Error("expected error for empty context name")
	}
	if err := UpdateKubeconfig("n", "", "r", "p"); err == nil {
		t.Error("expected error for empty cluster")
	}
}
