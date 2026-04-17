package kube

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleConfig = `
apiVersion: v1
kind: Config
contexts:
- name: abc-dev
  context:
    cluster: abc-dev-eks
    user: aws
- name: abc-prod
  context:
    cluster: abc-prod-eks
    user: aws
current-context: abc-dev
`

func writeConfig(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestContexts(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "config", sampleConfig)
	t.Setenv("KUBECONFIG", path)

	names, err := Contexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "abc-dev" || names[1] != "abc-prod" {
		t.Errorf("names = %v", names)
	}
}

func TestContextsMergesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	a := writeConfig(t, dir, "a", `contexts:
- name: alpha
- name: shared`)
	b := writeConfig(t, dir, "b", `contexts:
- name: shared
- name: beta`)
	t.Setenv("KUBECONFIG", a+string(filepath.ListSeparator)+b)

	names, err := Contexts()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"alpha": true, "shared": true, "beta": true}
	if len(names) != len(want) {
		t.Fatalf("names = %v", names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected name %q", n)
		}
	}
}

func TestContextsIgnoresMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, "config", sampleConfig)
	missing := filepath.Join(dir, "does-not-exist")
	t.Setenv("KUBECONFIG", missing+string(filepath.ListSeparator)+path)

	names, err := Contexts()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Errorf("names = %v", names)
	}
}

func TestHasContext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBECONFIG", writeConfig(t, dir, "config", sampleConfig))

	ok, err := HasContext("abc-dev")
	if err != nil || !ok {
		t.Errorf("abc-dev: ok=%v err=%v", ok, err)
	}
	ok, err = HasContext("missing")
	if err != nil || ok {
		t.Errorf("missing: ok=%v err=%v", ok, err)
	}
}

func TestUseContextCmd(t *testing.T) {
	cases := map[string]string{
		"abc-dev":       "kubectl config use-context 'abc-dev'",
		"weird'name":    `kubectl config use-context 'weird'\''name'`,
		"with space":    "kubectl config use-context 'with space'",
	}
	for in, want := range cases {
		if got := UseContextCmd(in); got != want {
			t.Errorf("UseContextCmd(%q) = %q, want %q", in, got, want)
		}
	}
}
