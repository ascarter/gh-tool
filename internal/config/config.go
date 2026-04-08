package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the top-level TOML manifest structure.
type Config struct {
	Settings Settings `toml:"settings,omitempty"`
	Tools    []Tool   `toml:"tool"`
}

// Settings holds optional path overrides and global configuration.
type Settings struct {
	DataHome  string `toml:"data_home,omitempty"`
	StateHome string `toml:"state_home,omitempty"`
	CacheHome string `toml:"cache_home,omitempty"`
}

// Tool describes a single tool to install from a GitHub release.
type Tool struct {
	Repo        string   `toml:"repo"`
	Pattern     string   `toml:"pattern,omitempty"`
	Tag         string   `toml:"tag,omitempty"`
	Bin         []string `toml:"bin,omitempty"`
	Man         []string `toml:"man,omitempty"`
	Completions []string `toml:"completions,omitempty"`
}

// Name returns the tool name derived from the repo (the part after /).
func (t Tool) Name() string {
	_, name := splitRepo(t.Repo)
	return name
}

// Owner returns the repository owner (the part before /).
func (t Tool) Owner() string {
	owner, _ := splitRepo(t.Repo)
	return owner
}

// Load reads and parses the TOML config file at path.
// If the file does not exist, an empty Config is returned.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the config as TOML to the given path.
func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// FindTool returns the tool entry matching repo, or nil if not found.
func (c *Config) FindTool(repo string) *Tool {
	for i := range c.Tools {
		if c.Tools[i].Repo == repo {
			return &c.Tools[i]
		}
	}
	return nil
}

// AddOrUpdateTool adds a new tool or updates an existing one.
func (c *Config) AddOrUpdateTool(t Tool) {
	for i := range c.Tools {
		if c.Tools[i].Repo == t.Repo {
			c.Tools[i] = t
			return
		}
	}
	c.Tools = append(c.Tools, t)
}

// RemoveTool removes the tool matching repo. Returns true if found.
func (c *Config) RemoveTool(repo string) bool {
	for i := range c.Tools {
		if c.Tools[i].Repo == repo {
			c.Tools = append(c.Tools[:i], c.Tools[i+1:]...)
			return true
		}
	}
	return false
}

func splitRepo(repo string) (owner, name string) {
	for i := range repo {
		if repo[i] == '/' {
			return repo[:i], repo[i+1:]
		}
	}
	return "", repo
}
