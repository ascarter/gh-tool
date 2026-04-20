# gh-tool

A GitHub CLI extension for installing and managing binary tools from GitHub releases.

Maintain a simple TOML manifest in your dotfiles that describes the tools you want. `gh tool` downloads release assets, extracts them, and symlinks binaries, man pages, and shell completions into XDG-compliant directories — one `bin/` directory to add to your PATH.

## Install

```sh
gh extension install ascarter/gh-tool
```

Add gh-tool's `bin/` to your PATH via your shell profile:

```sh
# bash (~/.bashrc)
eval "$(gh tool shell bash)"

# zsh (~/.zshrc)
eval "$(gh tool shell zsh)"

# fish (~/.config/fish/config.fish)
gh tool shell fish | source
```

Open a new shell (or `source` the file you edited) before continuing.

## Tutorial

Most people never need to hand-edit TOML. The flow is:

1. **`gh tool add owner/repo`** — interactive picker. Walks you from a GitHub repo to a working manifest entry: fetches the latest release, lets you pick the asset variants you want, inspects the archive for the binary/man/completion paths, and writes the entry to your manifest.
2. **`gh tool install`** — reconciles the manifest. Installs everything that's listed but not installed yet.

For example, add `bat` and `ripgrep` to your toolbox:

```sh
gh tool add sharkdp/bat
gh tool add BurntSushi/ripgrep
gh tool install
```

Your manifest now lives at `~/.config/gh-tool/config.toml` and is ready to commit to your dotfiles repo. Running `gh tool install` on a new machine reproduces the same set.

`add` only writes the manifest — installation is a separate step so you can review the entry (and commit it) before downloading anything.

## Daily workflow

```sh
# Just the names of installed tools.
gh tool list

# Names with installed versions.
gh tool list --versions

# Table of installed tools with their installed and latest available versions.
gh tool list --long

# Just the tools that have a newer release available.
gh tool outdated

# Pull latest releases for everything installed.
gh tool upgrade

# Upgrade a single tool.
gh tool upgrade junegunn/fzf

# Re-apply the manifest after you edited it by hand (renamed a bin, changed
# a pattern, etc.). Clears stale symlinks and cached downloads before
# reinstalling.
gh tool install --force

# Remove a single tool. The manifest is NOT modified — if the entry is
# still there, `gh tool install` will put it back.
gh tool remove junegunn/fzf
```

Install and upgrade run in parallel by default. Handy flags on both:

| Flag               | Purpose                                                        |
|--------------------|----------------------------------------------------------------|
| `-j, --jobs N`     | Parallelism cap (default `min(8, NumCPU)`).                    |
| `--no-progress`    | Disable the live progress UI; print one line per event.        |
| `-v, --verbose`    | Log every step (download, verify, extract) per tool.           |
| `--no-verify`      | Skip attestation verification (install only).                  |

## Removing gh-tool entirely

`gh tool reset` removes every installed tool and wipes gh-tool's data, state, and cache directories. Your manifest is preserved so you can restore everything with `gh tool install` later.

```sh
gh tool reset           # confirms interactively
gh tool reset --yes     # skip the prompt
```

`reset` does not remove the gh-tool extension itself. If you also want the extension gone, follow up with:

```sh
gh extension remove ascarter/gh-tool
```

If you want a truly clean slate (including the manifest), delete `~/.config/gh-tool/config.toml` yourself after running `reset`.

## Manifest reference

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

### Tool attributes

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

### Pattern variables

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

### Per-platform patterns

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

### Binary renaming

When a downloaded binary or extracted file has a platform-specific name but you want a clean symlink, use the `source:link` syntax in `bin`. Template variables are supported in `bin` specs:

```toml
# Downloaded asset is "jq-macos-arm64", symlink as "jq" (cross-platform)
bin = ["jq-{{platform}}-{{arch}}:jq"]

# Bare binary with OS/arch in name
bin = ["direnv.{{os}}-{{arch}}:direnv"]
```

### OS filtering

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

### Ad-hoc installs (without a manifest entry)

You can install one-off without touching the manifest — useful for trying a tool before committing to an entry:

```sh
gh tool install junegunn/fzf --pattern 'fzf-*-{{os}}_{{arch}}.tar.gz' --bin fzf
```

This does not modify the manifest; the install shows up in `gh tool list` as `orphan` until you add it. Run `gh tool add` afterwards if you want to persist it.

### Alternate manifests

`--file/-f` points `install` (and `add`) at a manifest other than the default:

```sh
gh tool install --file ./tools.toml
gh tool add --file ./laptop.toml sharkdp/bat
```

State always lives under `~/.local/state/gh-tool/` regardless of which manifest you load.

## Commands

```
gh tool add <owner/repo>        Interactively author a manifest entry (does not install)
gh tool install [owner/repo]    Reconcile from manifest, or install a single tool
gh tool remove <owner/repo>     Remove an installed tool (manifest is not modified)
gh tool list                    List installed tool names; --versions adds versions, --long for the full table
gh tool outdated                List installed tools with a newer release available
gh tool upgrade [owner/repo]    Upgrade to latest release (state-driven)
gh tool reset                   Remove all installed tools and clear gh-tool data
gh tool cache list              Show cached downloads
gh tool cache clean [tool]      Remove cached downloads
gh tool shell <bash|zsh|fish>   Print shell integration config
gh tool version                 Print version
```

Notable flags:

- `add`: `--file/-f`, `--tag/-t`, `--no-write` (preview the generated block without saving).
- `install`: `--pattern/-p`, `--tag/-t`, `--bin`, `--man`, `--completion`, `--no-verify`, `--force`, `--file/-f`, `--jobs/-j`, `--no-progress`, `--verbose/-v`.
- `list`: `--versions`, `--long/-l`.
- `upgrade`: `--jobs/-j`, `--no-progress`, `--verbose/-v`.
- `reset`: `--yes/-y`.

## How it works

1. `gh tool install` downloads a release asset via `gh release download` into a cache directory.
2. The asset is verified with `gh attestation verify` (best-effort — most repos don't publish attestations yet).
3. Archives (`tar.gz`, `tar.xz`, `zip`) are extracted; bare binaries are copied directly. If an archive has a single top-level directory, it is stripped automatically.
4. Symlinks are created from `~/.local/share/gh-tool/bin/` into the extracted tool directory. Use `source:link` in `bin` to rename binaries (e.g., `jq-macos-arm64:jq`).
5. A state file under `~/.local/state/gh-tool/<name>.toml` records the installed tag, the resolved download pattern, and the symlinked `bin`/`man`/`completions`. `list`, `remove`, and `upgrade` operate from these state files; the manifest is only consulted by `install` (and by `list` for drift reporting).

## Filesystem layout

```
~/.config/gh-tool/config.toml          Manifest (TOML)
~/.local/share/gh-tool/bin/            Binary symlinks (add to PATH)
~/.local/share/gh-tool/share/man/man1/ Man page symlinks
~/.local/share/gh-tool/tools/<name>/   Extracted tool payloads
~/.local/state/gh-tool/<name>.toml     Installed version tracking
~/.cache/gh-tool/<name>/               Download cache
```

All paths respect XDG environment variables (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`).

## Migration

If you are upgrading from an earlier version of gh-tool, run this once to refresh state files into the new schema (which now also records the symlinked `bin`/`man`/`completions` so list/upgrade/remove no longer need to consult the manifest):

```sh
gh tool install --force
```

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
