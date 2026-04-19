package cmd

import (
	"runtime"
	"testing"

	"github.com/ascarter/gh-tool/internal/config"
	"github.com/ascarter/gh-tool/internal/tool"
)

func TestSliceEqualOrEmpty(t *testing.T) {
	tests := []struct {
		name     string
		state    []string
		manifest []string
		want     bool
	}{
		{"both empty", nil, nil, true},
		{"state empty, manifest set", nil, []string{"a"}, true}, // not recorded == no opinion
		{"equal", []string{"a", "b"}, []string{"a", "b"}, true},
		{"different", []string{"a"}, []string{"b"}, false},
		{"different lengths", []string{"a"}, []string{"a", "b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sliceEqualOrEmpty(tt.state, tt.manifest); got != tt.want {
				t.Errorf("sliceEqualOrEmpty(%v,%v)=%v want %v", tt.state, tt.manifest, got, tt.want)
			}
		})
	}
}

func TestSpecDriftsFromManifest(t *testing.T) {
	state := tool.InstalledState{
		Repo:    "foo/bar",
		Tag:     "v1.0.0",
		Pattern: "bar-v1.0.0-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz",
		Bin:     []string{"bar"},
	}
	manifestSame := config.Tool{
		Repo:    "foo/bar",
		Pattern: "bar-{{tag}}-{{os}}-{{arch}}.tar.gz",
		Bin:     []string{"bar"},
	}
	if specDriftsFromManifest(state, manifestSame) {
		t.Errorf("expected no drift; state=%+v manifest=%+v", state, manifestSame)
	}

	manifestDiffPattern := config.Tool{
		Repo:    "foo/bar",
		Pattern: "different-{{tag}}-{{os}}-{{arch}}.tar.gz",
		Bin:     []string{"bar"},
	}
	if !specDriftsFromManifest(state, manifestDiffPattern) {
		t.Errorf("expected drift on pattern change")
	}

	manifestDiffBin := config.Tool{
		Repo:    "foo/bar",
		Pattern: "bar-{{tag}}-{{os}}-{{arch}}.tar.gz",
		Bin:     []string{"baz"},
	}
	if !specDriftsFromManifest(state, manifestDiffBin) {
		t.Errorf("expected drift on bin change")
	}
}

func TestClassifyInstalled(t *testing.T) {
	state := tool.InstalledState{Repo: "foo/bar", Tag: "v1.0.0"}
	manifest := config.Tool{Repo: "foo/bar"}

	if got := classifyInstalled(state, manifest, false, "v1.0.0"); got != "orphan" {
		t.Errorf("not in manifest -> %q, want orphan", got)
	}
	if got := classifyInstalled(state, manifest, true, "v1.0.0"); got != "up to date" {
		t.Errorf("matching latest -> %q, want up to date", got)
	}
	if got := classifyInstalled(state, manifest, true, "v1.1.0"); got != "update available" {
		t.Errorf("newer latest -> %q, want update available", got)
	}
	if got := classifyInstalled(state, manifest, true, "?"); got != "up to date" {
		t.Errorf("unknown latest -> %q, want up to date", got)
	}

	driftState := tool.InstalledState{Repo: "foo/bar", Tag: "v1.0.0", Bin: []string{"x"}}
	driftManifest := config.Tool{Repo: "foo/bar", Bin: []string{"y"}}
	if got := classifyInstalled(driftState, driftManifest, true, "v1.0.0"); got != "drift" {
		t.Errorf("drift -> %q, want drift", got)
	}
}
