package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/xetius/switcher/internal/awsx"
	"github.com/xetius/switcher/internal/config"
	"github.com/xetius/switcher/internal/kube"
	"github.com/xetius/switcher/internal/tui"
)

// version is stamped at build time via -ldflags; falls back to "dev".
var version = "dev"

func main() {
	args := os.Args[1:]

	// eks-token is invoked by kubectl via the exec credential plugin protocol;
	// it must emit JSON on stdout and nothing else.
	if len(args) > 0 && args[0] == "eks-token" {
		if err := eksToken(args[1:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "switch eks-token:", err)
			os.Exit(1)
		}
		return
	}

	// Flag modes: output diagnostic info (help/version) to stderr so the shell
	// wrapper's eval never picks it up. Shell-init modes (--bash/--zsh) emit to
	// stdout because their output is meant to be eval'd at rc-file time.
	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h":
			printHelp(os.Stderr)
			return
		case "--version":
			fmt.Fprintln(os.Stderr, "switch", version)
			return
		case "--bash", "--zsh":
			fmt.Fprint(os.Stdout, shellInit())
			return
		}
	}

	if err := run(args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "switch:", err)
		os.Exit(1)
	}
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `switch — change AWS SSO account + kubectl context in one step

Usage:
  switch [query]         resolve query → pick context → SSO login if needed →
                         emit shell exports for the wrapper to eval
  switch --bash          print bash wrapper function (eval in .bashrc)
  switch --zsh           print zsh wrapper function (eval in .zshrc)
  switch --help, -h      show this help
  switch --version       show version

Subcommands (internal):
  switch eks-token ...   kubectl exec-plugin; emits ExecCredential JSON

Shell integration:
  add to .zshrc / .bashrc:
      eval "$(switch --zsh)"          # or --bash
  then:
      switch psp-dev

Query resolution:
  - exact name match → used directly
  - unique fuzzy (subsequence) match → used directly
  - empty / ambiguous / no match → opens fuzzy finder TUI

Config lookup order:
  1. ./switch.yaml
  2. $XDG_CONFIG_HOME/switch/config.yaml
  3. ~/.switch.yaml
`)
}

// shellInit returns the wrapper function to be eval'd in the user's shell rc.
// The function captures stdout from the binary and evals it, so the binary can
// mutate the parent shell's env (export AWS_PROFILE, kubectl config use-
// context). Flag modes are passed through without eval so `switch --help`
// works as expected once the wrapper is installed.
func shellInit() string {
	return `switch() {
  case "$1" in
    --help|-h|--version|--bash|--zsh)
      command switch "$@"
      return $?
      ;;
  esac
  local __switch_out
  __switch_out="$(command switch "$@")" || return $?
  [ -n "$__switch_out" ] && eval "$__switch_out"
}
`
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			return fmt.Errorf("no config found (looked in ./switch.yaml, $XDG_CONFIG_HOME/switch/config.yaml, ~/.switch.yaml)")
		}
		return fmt.Errorf("load config: %w", err)
	}

	query := ""
	if len(args) > 0 {
		query = args[0]
	}

	name, err := resolve(cfg, query)
	if err != nil {
		return err
	}
	if name == "" {
		fmt.Fprintln(stderr, "cancelled")
		return nil
	}

	ctx, ok := cfg.Lookup(name)
	if !ok {
		return fmt.Errorf("resolver returned unknown context %q", name)
	}

	if err := awsx.SSOLogin(ctx.Profile); err != nil {
		return err
	}

	has, err := kube.HasContext(name)
	if err != nil {
		return fmt.Errorf("read kubeconfig: %w", err)
	}
	if !has {
		if ctx.EKSCluster == "" {
			return fmt.Errorf("k8s context %q missing and eks_cluster not configured", name)
		}
		fmt.Fprintf(stderr, "context %q missing; fetching cluster info from EKS...\n", name)
		if err := awsx.UpdateKubeconfig(name, ctx.EKSCluster, ctx.Region, ctx.Profile); err != nil {
			return err
		}
	}

	fmt.Fprintln(stdout, "export AWS_PROFILE="+shellQuote(ctx.Profile))
	fmt.Fprintln(stdout, kube.UseContextCmd(name))
	return nil
}

// resolve maps a user query to a configured context name. Returns "" if the
// user cancelled the picker.
func resolve(cfg *config.Config, query string) (string, error) {
	if name, ok := tryDirect(cfg, query); ok {
		return name, nil
	}
	return tui.Pick(cfg.Names(), query)
}

// tryDirect returns (name, true) when the query can be resolved without a
// picker: exact name match, or a unique fuzzy match. Otherwise returns
// ("", false) and the caller should open the picker.
func tryDirect(cfg *config.Config, query string) (string, bool) {
	if query == "" {
		return "", false
	}
	if _, ok := cfg.Lookup(query); ok {
		return query, true
	}
	matches := tui.Filter(cfg.Names(), query)
	if len(matches) == 1 {
		return matches[0], true
	}
	return "", false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// eksToken implements the client-go exec credential plugin protocol: parse
// --cluster/--region/--profile and write an ExecCredential JSON to stdout.
// Invoked by kubectl every ~14 minutes to refresh the bearer token.
func eksToken(args []string, stdout io.Writer) error {
	var cluster, region, profile string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cluster":
			i++
			if i < len(args) {
				cluster = args[i]
			}
		case "--region":
			i++
			if i < len(args) {
				region = args[i]
			}
		case "--profile":
			i++
			if i < len(args) {
				profile = args[i]
			}
		}
	}
	if cluster == "" {
		return fmt.Errorf("--cluster is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, expiry, err := awsx.GenerateEKSToken(ctx, cluster, region, profile)
	if err != nil {
		return err
	}

	cred := map[string]interface{}{
		"kind":       "ExecCredential",
		"apiVersion": "client.authentication.k8s.io/v1beta1",
		"status": map[string]interface{}{
			"expirationTimestamp": expiry.Format(time.RFC3339),
			"token":               token,
		},
	}
	return json.NewEncoder(stdout).Encode(cred)
}
