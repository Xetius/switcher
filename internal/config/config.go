package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Context is a single entry in the mapping file.
type Context struct {
	Profile    string `yaml:"profile"`
	EKSCluster string `yaml:"eks_cluster"`
	Region     string `yaml:"region"`
}

// Config is the parsed mapping file.
type Config struct {
	Contexts map[string]Context `yaml:"contexts"`

	// Path is the file the config was loaded from (empty if constructed in-memory).
	Path string `yaml:"-"`
}

// ErrNotFound is returned by Load when no config file is found in any of the
// search paths.
var ErrNotFound = errors.New("config file not found")

// Load searches for the config file in order:
//  1. ./switch.yaml
//  2. $XDG_CONFIG_HOME/switch/config.yaml (or ~/.config/switch/config.yaml)
//  3. $HOME/.switch.yaml
//
// The first existing file is parsed and returned. If none exist, ErrNotFound.
func Load() (*Config, error) {
	paths, err := searchPaths()
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return LoadFile(p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
	}
	return nil, ErrNotFound
}

// LoadFile parses a config file at the given path.
func LoadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.Path = path
	return &c, nil
}

// Lookup returns the context for the given name, or false if absent.
func (c *Config) Lookup(name string) (Context, bool) {
	ctx, ok := c.Contexts[name]
	return ctx, ok
}

// Names returns all configured context names.
func (c *Config) Names() []string {
	out := make([]string, 0, len(c.Contexts))
	for k := range c.Contexts {
		out = append(out, k)
	}
	return out
}

func (c *Config) validate() error {
	if len(c.Contexts) == 0 {
		return errors.New("no contexts defined")
	}
	for name, ctx := range c.Contexts {
		if ctx.Profile == "" {
			return fmt.Errorf("context %q: profile is required", name)
		}
	}
	return nil
}

func searchPaths() ([]string, error) {
	var paths []string

	// 1. current directory
	paths = append(paths, "switch.yaml")

	// 2. XDG config dir
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		xdg = filepath.Join(home, ".config")
	}
	paths = append(paths, filepath.Join(xdg, "switch", "config.yaml"))

	// 3. home dotfile
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	paths = append(paths, filepath.Join(home, ".switch.yaml"))

	return paths, nil
}
