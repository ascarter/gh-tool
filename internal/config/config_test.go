package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolName(t *testing.T) {
	tests := []struct {
		repo string
		want string
	}{
		{"junegunn/fzf", "fzf"},
		{"jqlang/jq", "jq"},
		{"BurntSushi/ripgrep", "ripgrep"},
		{"fzf", "fzf"},
	}
	for _, tt := range tests {
		tool := Tool{Repo: tt.repo}
		if got := tool.Name(); got != tt.want {
			t.Errorf("Tool{Repo: %q}.Name() = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestToolOwner(t *testing.T) {
	tests := []struct {
		repo string
		want string
	}{
		{"junegunn/fzf", "junegunn"},
		{"jqlang/jq", "jqlang"},
		{"fzf", ""},
	}
	for _, tt := range tests {
		tool := Tool{Repo: tt.repo}
		if got := tool.Owner(); got != tt.want {
			t.Errorf("Tool{Repo: %q}.Owner() = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Load non-existent returns empty
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load non-existent: %v", err)
	}
	if len(cfg.Tools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(cfg.Tools))
	}

	// Add tools and save
	cfg.AddOrUpdateTool(Tool{
		Repo:    "junegunn/fzf",
		Pattern: "fzf-*-darwin_arm64.tar.gz",
		Bin:     []string{"fzf"},
	})
	cfg.AddOrUpdateTool(Tool{
		Repo:    "jqlang/jq",
		Pattern: "jq-macos-arm64",
		Tag:     "jq-1.7.1",
		Bin:     []string{"jq"},
	})

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify
	cfg2, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg2.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(cfg2.Tools))
	}
	if cfg2.Tools[0].Repo != "junegunn/fzf" {
		t.Errorf("tool[0].Repo = %q, want junegunn/fzf", cfg2.Tools[0].Repo)
	}
	if cfg2.Tools[1].Tag != "jq-1.7.1" {
		t.Errorf("tool[1].Tag = %q, want jq-1.7.1", cfg2.Tools[1].Tag)
	}
}

func TestFindTool(t *testing.T) {
	cfg := &Config{
		Tools: []Tool{
			{Repo: "junegunn/fzf"},
			{Repo: "jqlang/jq"},
		},
	}

	if tool := cfg.FindTool("junegunn/fzf"); tool == nil {
		t.Error("FindTool(junegunn/fzf) returned nil")
	}
	if tool := cfg.FindTool("nonexistent/tool"); tool != nil {
		t.Error("FindTool(nonexistent/tool) should return nil")
	}
}

func TestAddOrUpdateTool(t *testing.T) {
	cfg := &Config{}
	cfg.AddOrUpdateTool(Tool{Repo: "junegunn/fzf", Tag: "v0.50.0"})
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(cfg.Tools))
	}

	// Update existing
	cfg.AddOrUpdateTool(Tool{Repo: "junegunn/fzf", Tag: "v0.51.0"})
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 tool after update, got %d", len(cfg.Tools))
	}
	if cfg.Tools[0].Tag != "v0.51.0" {
		t.Errorf("tag = %q, want v0.51.0", cfg.Tools[0].Tag)
	}
}

func TestRemoveTool(t *testing.T) {
	cfg := &Config{
		Tools: []Tool{
			{Repo: "junegunn/fzf"},
			{Repo: "jqlang/jq"},
		},
	}

	if !cfg.RemoveTool("junegunn/fzf") {
		t.Error("RemoveTool should return true")
	}
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(cfg.Tools))
	}
	if cfg.Tools[0].Repo != "jqlang/jq" {
		t.Errorf("remaining tool = %q, want jqlang/jq", cfg.Tools[0].Repo)
	}

	if cfg.RemoveTool("nonexistent/tool") {
		t.Error("RemoveTool(nonexistent) should return false")
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "config.toml")
	cfg := &Config{}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save with nested dir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestLoadParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("[[[invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("Load with invalid TOML should error")
	}
}
