# gh-tool

A GitHub CLI extension for installing and managing binary tools from GitHub releases.

Maintain a simple TOML manifest in your dotfiles that describes the tools you want. `gh tool` downloads release assets, extracts them, and symlinks binaries, man pages, and shell completions into XDG-compliant directories — one `bin/` directory to add to your PATH.

## Install

```sh
gh extension install ascarter/gh-tool
```

## Quick Start

```sh
# Install a tool
gh tool install junegunn/fzf --pattern 'fzf-*-darwin_arm64.tar.gz'

# Install with version pinning
gh tool install jqlang/jq --pattern 'jq-macos-arm64' --tag jq-1.7.1

# Install with architecture template variables
gh tool install junegunn/fzf --pattern 'fzf-*-{{os}}_{{arch}}.tar.gz'

# Install a Rust tool using platform triples
gh tool install BurntSushi/ripgrep --pattern 'ripgrep-*-{{triple}}.tar.gz'

# Install a bare binary with rename (source:target)
gh tool install jqlang/jq --pattern 'jq-macos-arm64' --bin 'jq-macos-arm64:jq'

# List installed tools and check for updates
gh tool list

# Upgrade everything to latest
gh tool upgrade

# Remove a tool
gh tool remove junegunn/fzf
```

## Shell Integration

Add to your shell profile to put installed tools on your PATH:

```sh
# bash (~/.bashrc)
eval "$(gh tool shell bash)"

# zsh (~/.zshrc)
eval "$(gh tool shell zsh)"
```

## Manifest

Declare tools in `$XDG_CONFIG_HOME/gh-tool/config.toml` (typically `~/.config/gh-tool/config.toml`). This file is designed to be checked into a dotfiles repo.

The manifest is a **read-only input** — like a Brewfile. `gh tool install` reads it; nothing writes to it unless you pass `--save`. The local install set is tracked separately in per-tool state files under `$XDG_STATE_HOME/gh-tool/`.

```toml
[[tool]]
repo = "junegunn/fzf"
pattern = "fzf-*-{{os}}_{{arch}}.tar.gz"
bin = ["fzf"]

[[tool]]
repo = "jqlang/jq"
pattern = "jq-macos-arm64"
tag = "jq-1.7.1"
bin = ["jq-macos-arm64:jq"]

[[tool]]
repo = "BurntSushi/ripgrep"
pattern = "ripgrep-*-{{triple}}.tar.gz"
bin = ["rg"]

[[tool]]
repo = "dandavison/delta"
pattern = "delta-*-{{triple}}.tar.gz"
bin = ["delta"]
man = ["delta.1.gz"]
completions = ["completion/_delta"]

[[tool]]
repo = "mikefarah/yq"
pattern = "yq_{{os}}_{{arch}}.tar.gz"
bin = ["yq_darwin_arm64:yq"]
```

Reconcile the local install set against the manifest at any time:

```sh
gh tool install                  # install anything missing; leave up-to-date tools alone
gh tool install --force          # rebuild every manifest tool from scratch
gh tool install junegunn/fzf     # install (or reinstall) just one tool from the manifest
```

After editing the manifest, use `--force` to apply spec changes (renamed `bin`, swapped `pattern`, etc.) — it clears stale symlinks and the cached download before reinstalling. `gh tool list` will flag changed-but-not-reapplied entries as `drift`.

### Using an alternate manifest

`--file` / `-f` points `install` at a manifest other than the default:

```sh
gh tool install --file ./tools.toml
gh tool install -f ./laptop.toml --force
```

State always lives under `$XDG_STATE_HOME/gh-tool/` regardless of which manifest you load.

### Recording an ad-hoc install

Without a manifest entry you can still install one-off:

```sh
gh tool install junegunn/fzf --pattern 'fzf-*-{{os}}_{{arch}}.tar.gz' --bin fzf
```

By default this does not modify the manifest — the install only shows up in `gh tool list` (as `orphan` until you add it). Add `--save` to record the resulting spec in the manifest:

```sh
gh tool install junegunn/fzf --pattern 'fzf-*-{{os}}_{{arch}}.tar.gz' --save
```

Note: `--save` reformats the manifest (TOML comments and key ordering are not preserved). Editing the manifest by hand is the recommended workflow for most users.

### Tool Attributes

| Attribute     | Description                                                   | Default        |
|---------------|---------------------------------------------------------------|----------------|
| `repo`        | GitHub `owner/repo`                                           | required       |
| `pattern`     | Glob for release asset (supports template variables; see below) | required*      |
| `patterns`    | Platform-specific pattern overrides (keyed by `os_arch`)      | none           |
| `tag`         | Release tag to pin                                            | latest         |
| `bin`         | Binary name(s) to symlink; use `source:link` to rename        | `[<toolname>]` |
| `man`         | Man page path(s) relative to extracted archive                | none           |
| `completions` | Shell completion path(s) relative to extracted archive        | none           |
| `os`          | OS filter; install only on listed OSes (e.g., `["linux"]`)    | all            |

*Either `pattern` or `patterns` must be provided.

### Pattern Variables

Patterns support template variables that expand to platform-specific values at runtime:

| Variable       | Description                   | macOS ARM64              | Linux x86_64                     |
|----------------|-------------------------------|--------------------------|----------------------------------|
| `{{os}}`       | Go-style OS name              | `darwin`                 | `linux`                          |
| `{{arch}}`     | Go-style architecture         | `arm64`                  | `amd64`                          |
| `{{triple}}`   | Rust target triple            | `aarch64-apple-darwin`   | `x86_64-unknown-linux-gnu`       |
| `{{platform}}` | User-facing OS name           | `macos`                  | `linux`                          |
| `{{gnuarch}}`  | GNU/Rust-style architecture   | `aarch64`                | `x86_64`                         |

Use `{{os}}` / `{{arch}}` for Go-style release naming (e.g., `fzf-*-{{os}}_{{arch}}.tar.gz`).
Use `{{triple}}` for Rust-style naming (e.g., `ripgrep-*-{{triple}}.tar.gz`).
Use `{{platform}}` / `{{gnuarch}}` for projects that use `macos` or `aarch64` in asset names.

### Per-Platform Patterns

When a project uses inconsistent naming across platforms, use `patterns` to provide platform-specific overrides keyed by `goos_goarch`. The `pattern` field serves as the default fallback:

```toml
# Neovim uses arm64 on ARM but x86_64 on Intel
[[tool]]
repo = "neovim/neovim"
pattern = "nvim-{{platform}}-{{arch}}.tar.gz"
bin = ["nvim"]
man = ["share/man/man1/nvim.1"]

[tool.patterns]
darwin_amd64 = "nvim-macos-x86_64.tar.gz"
linux_amd64 = "nvim-linux-x86_64.tar.gz"
```

Resolution order:
1. If `patterns` has a key matching the current `os_arch` (e.g., `darwin_arm64`), use that pattern
2. Otherwise, fall back to `pattern` (with template variable expansion)

### Binary Renaming

When a downloaded binary or extracted file has a platform-specific name but you want a clean symlink, use the `source:link` syntax in `bin`. Template variables are supported in `bin` specs:

```toml
# Downloaded asset is "jq-macos-arm64", symlink as "jq" (cross-platform)
bin = ["jq-{{platform}}-{{arch}}:jq"]

# Bare binary with OS/arch in name
bin = ["direnv.{{os}}-{{arch}}:direnv"]
```

### OS Filtering

Use `os` to restrict a tool to specific operating systems. This is useful when you manage a tool through another package manager (e.g., Homebrew) on one OS but want `gh-tool` on others:

```toml
# Only install neovim on Linux (use Homebrew on macOS)
[[tool]]
repo = "neovim/neovim"
pattern = "nvim-linux-{{gnuarch}}.tar.gz"
os = ["linux"]
bin = ["nvim"]
```

Values are Go-style OS names: `darwin`, `linux`, `windows`. If `os` is omitted, the tool installs on all platforms.

## Commands

```
gh tool install [owner/repo]    Reconcile from manifest, or install a single tool
gh tool remove <owner/repo>     Remove an installed tool (manifest is not modified)
gh tool list                    List installed tools and any drift from the manifest
gh tool upgrade [owner/repo]    Upgrade to latest release (state-driven)
gh tool cache list              Show cached downloads
gh tool cache clean [tool]      Remove cached downloads
gh tool shell <bash|zsh>        Print shell integration config
gh tool version                 Print version
```

`install` flags: `--pattern/-p`, `--tag/-t`, `--bin`, `--man`, `--completion`, `--no-verify`, `--force`, `--file/-f`, `--save`.

### List status values

| Status             | Meaning |
|--------------------|---------|
| `up to date`       | Installed, on the latest release, matches the manifest spec |
| `update available` | Installed, but a newer release exists |
| `drift`            | Installed spec differs from the manifest spec — run `gh tool install --force` |
| `orphan`           | Installed but not in the manifest |
| `pending`          | In the manifest but not installed |
| `skipped (os)`     | In the manifest, filtered out by `os` on this platform |

## How It Works

1. `gh tool install` downloads a release asset via `gh release download` into a cache directory
2. The asset is verified with `gh attestation verify` (best-effort — most repos don't publish attestations yet)
3. Archives (tar.gz, tar.xz, zip) are extracted; bare binaries are copied directly. If an archive has a single top-level directory, it is stripped automatically
4. Symlinks are created from `$XDG_DATA_HOME/gh-tool/bin/` into the extracted tool directory. Use `source:link` in `bin` to rename binaries (e.g., `jq-macos-arm64:jq`)
5. A state file under `$XDG_STATE_HOME/gh-tool/<name>.toml` records the installed tag, the resolved download pattern, and the symlinked `bin`/`man`/`completions`. `list`, `remove`, and `upgrade` operate from these state files; the manifest is only consulted by `install` (and by `list` for drift reporting).

## Migration

If you are upgrading from an earlier version of gh-tool, run this once to
refresh state files into the new schema (which now also records the
symlinked `bin`/`man`/`completions` so list/upgrade/remove no longer need
to consult the manifest):

```sh
gh tool install --force
```

## Filesystem Layout

```
~/.config/gh-tool/config.toml          Manifest (TOML)
~/.local/share/gh-tool/bin/            Binary symlinks (add to PATH)
~/.local/share/gh-tool/share/man/man1/ Man page symlinks
~/.local/share/gh-tool/tools/<name>/   Extracted tool payloads
~/.local/state/gh-tool/<name>.toml     Installed version tracking
~/.cache/gh-tool/<name>/               Download cache
```

All paths respect XDG environment variables (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`).

## Development

```sh
make build      # Build with version from git tags
make test       # Run tests
make vet        # Run go vet
make clean      # Remove binary and caches
```

### Release

```sh
make release TAG=v0.1.0
```

This creates a GitHub release. The CI workflow cross-compiles for all platforms and uploads binaries automatically.

## License

MIT
