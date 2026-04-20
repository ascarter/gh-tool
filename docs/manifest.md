# Manifest reference

`gh tool add` is the easiest way to author manifest entries, but you can also edit `~/.config/gh-tool/config.toml` directly. The manifest is a **read-only input**: `gh tool install` reads it; only `gh tool add` writes it. Your local install set is tracked separately in per-tool state files under `~/.local/state/gh-tool/`.

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

## Tool attributes

| Attribute     | Description                                                     | Default        |
|---------------|-----------------------------------------------------------------|----------------|
| `repo`        | GitHub `owner/repo`                                             | required       |
| `pattern`     | Glob for release asset (supports template variables; see below) | required*      |
| `patterns`    | Platform-specific pattern overrides (keyed by `os_arch`)        | none           |
| `tag`         | Release tag to pin                                              | latest         |
| `bin`         | Binary name(s) to symlink; use `source:link` to rename          | `[<toolname>]` |
| `man`         | Man page path(s) relative to extracted archive                  | none           |
| `completions` | Shell completion path(s) relative to extracted archive          | none           |
| `os`          | OS filter; install only on listed OSes (e.g., `["linux"]`)      | all            |

*Either `pattern` or `patterns` must be provided.

## Pattern variables

Patterns support template variables that expand to platform-specific values at runtime:

| Variable         | Description                     | macOS ARM64            | Linux x86_64               |
|------------------|---------------------------------|------------------------|----------------------------|
| `{{os}}`         | Go-style OS name                | `darwin`               | `linux`                    |
| `{{arch}}`       | Go-style architecture           | `arm64`                | `amd64`                    |
| `{{triple}}`     | Rust target triple (Linux=gnu)  | `aarch64-apple-darwin` | `x86_64-unknown-linux-gnu` |
| `{{musltriple}}` | Rust target triple (Linux=musl) | `aarch64-apple-darwin` | `x86_64-unknown-linux-musl`|
| `{{platform}}`   | User-facing OS name             | `macos`                | `linux`                    |
| `{{gnuarch}}`    | GNU/Rust-style architecture     | `aarch64`              | `x86_64`                   |
| `{{tag}}`        | Resolved release tag            | `v0.24.0`              | `v0.24.0`                  |

- Use `{{os}}` / `{{arch}}` for Go-style release naming (e.g., `fzf-*-{{os}}_{{arch}}.tar.gz`).
- Use `{{triple}}` for Rust-style naming on glibc Linux (e.g., `ripgrep-*-{{triple}}.tar.gz`).
- Use `{{musltriple}}` for Rust tools that ship statically-linked musl Linux binaries (e.g., `uv-{{musltriple}}.tar.gz`).
- Use `{{platform}}` / `{{gnuarch}}` for projects that use `macos` or `aarch64` in asset names.

## Per-platform patterns

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

1. If `patterns` has a key matching the current `os_arch` (e.g., `darwin_arm64`), use that pattern.
2. Otherwise, fall back to `pattern` (with template variable expansion).

## Binary renaming

When a downloaded binary or extracted file has a platform-specific name but you want a clean symlink, use the `source:link` syntax in `bin`. Template variables are supported in `bin` specs:

```toml
# Downloaded asset is "jq-macos-arm64", symlink as "jq" (cross-platform)
bin = ["jq-{{platform}}-{{arch}}:jq"]

# Bare binary with OS/arch in name
bin = ["direnv.{{os}}-{{arch}}:direnv"]
```

## OS filtering

Use `os` to restrict a tool to specific operating systems. This is useful when you manage a tool through a different package manager on one OS but want `gh-tool` on others:

```toml
# Only install neovim on Linux (use a system package manager on macOS)
[[tool]]
repo = "neovim/neovim"
pattern = "nvim-linux-{{gnuarch}}.tar.gz"
os = ["linux"]
bin = ["nvim"]
```

Values are Go-style OS names: `darwin`, `linux`, `windows`. If `os` is omitted, the tool installs on all platforms.

## Ad-hoc installs (without a manifest entry)

You can install one-off without touching the manifest — useful for trying a tool before committing to an entry:

```sh
gh tool install junegunn/fzf --pattern 'fzf-*-{{os}}_{{arch}}.tar.gz' --bin fzf
```

This does not modify the manifest; the install shows up in `gh tool list` as `orphan` until you add it. Run `gh tool add` afterwards if you want to persist it.

## Alternate manifests

`--file/-f` points `install` (and `add`) at a manifest other than the default:

```sh
gh tool install --file ./tools.toml
gh tool add --file ./laptop.toml sharkdp/bat
```

State always lives under `~/.local/state/gh-tool/` (or `$GHTOOL_HOME/state/` — see [shell.md](shell.md)) regardless of which manifest you load.
