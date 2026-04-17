package kube

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// kubeconfig is a minimal view of ~/.kube/config — only the fields needed to
// enumerate context names.
type kubeconfig struct {
	Contexts []struct {
		Name string `yaml:"name"`
	} `yaml:"contexts"`
}

// Contexts returns the union of context names across every file in $KUBECONFIG
// (colon-separated on unix, semicolon on windows), or ~/.kube/config when
// $KUBECONFIG is unset. Missing files are ignored; parse errors are returned.
func Contexts() ([]string, error) {
	paths, err := configPaths()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var out []string
	for _, p := range paths {
		names, err := readContexts(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, n := range names {
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	return out, nil
}

// HasContext reports whether the given context exists in kubeconfig.
func HasContext(name string) (bool, error) {
	names, err := Contexts()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}

// UseContextCmd returns the shell command that switches the current context.
// The main binary prints this to stdout for the shell wrapper to eval.
func UseContextCmd(name string) string {
	return "kubectl config use-context " + shellQuote(name)
}

func readContexts(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var kc kubeconfig
	if err := yaml.Unmarshal(data, &kc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	names := make([]string, 0, len(kc.Contexts))
	for _, c := range kc.Contexts {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	return names, nil
}

func configPaths() ([]string, error) {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return filepath.SplitList(env), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	return []string{filepath.Join(home, ".kube", "config")}, nil
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes, so
// it survives `eval` in the shell wrapper.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
