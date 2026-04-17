package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "switch.yaml")
	if err := os.WriteFile(path, []byte(`
contexts:
  psp-dev:
    profile: PSPAdmin
    eks_cluster: psp-dev-eks
    region: eu-west-1
`), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	ctx, ok := c.Lookup("psp-dev")
	if !ok {
		t.Fatalf("psp-dev missing")
	}
	if ctx.Profile != "PSPAdmin" {
		t.Errorf("profile = %q", ctx.Profile)
	}
	if ctx.EKSCluster != "psp-dev-eks" {
		t.Errorf("cluster = %q", ctx.EKSCluster)
	}
	if c.Path != path {
		t.Errorf("Path = %q", c.Path)
	}
}

func TestLoadFileRejectsMissingProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "switch.yaml")
	os.WriteFile(path, []byte(`
contexts:
  bad:
    eks_cluster: c
`), 0o600)

	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadFileRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "switch.yaml")
	os.WriteFile(path, []byte(`contexts: {}`), 0o600)

	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected error for empty contexts")
	}
}

func TestLoadNotFound(t *testing.T) {
	// Point every search path at an empty temp dir.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Run from the temp dir so ./switch.yaml also misses.
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLoadPrefersCwd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	os.WriteFile("switch.yaml", []byte(`
contexts:
  a:
    profile: P
`), 0o600)

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Lookup("a"); !ok {
		t.Fatal("expected context 'a'")
	}
}
