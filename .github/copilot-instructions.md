# Copilot Instructions

## Project Overview

`gh-tool` is a [GitHub CLI extension](https://docs.github.com/en/github-cli/github-cli/creating-github-cli-extensions) (precompiled Go binary) for installing binary tools from GitHub releases. It is invoked as `gh tool`.

Users maintain a TOML manifest (`config.toml`) that lists tools (by `owner/repo`) with download patterns, version pins, and symlink targets. The extension downloads release assets, extracts archives, and creates symlinks into XDG-compliant directories.

## Build & Test

```sh
go build -o gh-tool .        # Build
go test ./...                 # Run all tests
go test ./internal/config/    # Run a single package's tests
go test -run TestLoadSave ./internal/config/  # Run a single test
go vet ./...                  # Lint
```

## Architecture

- **`main.go`** — entry point, calls `cmd.Execute()`
- **`cmd/`** — cobra command definitions (root, install, remove, list, upgrade, cache, shell)
- **`internal/config/`** — TOML manifest structs and load/save logic (`Config`, `Tool`, `Settings`)
- **`internal/paths/`** — XDG Base Directory path resolution (`Dirs` struct with helpers)
- **`internal/archive/`** — archive extraction (tar.gz, zip) with leading-directory stripping
- **`internal/tool/`** — core install/remove/state management (`Manager` struct); shells out to `gh` via `go-gh`

## Key Conventions

- Use `gh.Exec()` from `github.com/cli/go-gh/v2` to run `gh` subcommands (release download, attestation verify) rather than reimplementing API calls
- Tool names are derived from the repo name (e.g., `junegunn/fzf` → `fzf`)
- Archives with a single top-level directory have that directory stripped on extraction
- Symlinks point from `$XDG_DATA_HOME/gh-tool/bin/` into tool-specific directories under `tools/`
- Config uses TOML with `[[tool]]` array-of-tables for the tool list
- Patterns support `{{os}}` and `{{arch}}` template variables expanded at runtime
- The `version` variable in `cmd/root.go` is set at build time via `-ldflags`
- Releases are built by `cli/gh-extension-precompile@v2` in GitHub Actions with attestation

