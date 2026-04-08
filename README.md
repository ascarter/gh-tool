# gh-tool

A GitHub CLI extension for installing and managing binary tools from GitHub releases.

## Install

```sh
gh extension install ascarter/gh-tool
```

## Quick Start

```sh
# Install a tool from a GitHub release
gh tool install junegunn/fzf --pattern 'fzf-*-darwin_arm64.tar.gz'

# Install with version pinning
gh tool install jqlang/jq --pattern 'jq-macos-arm64' --tag jq-1.7.1

# List installed tools
gh tool list

# Upgrade all tools to latest
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

Tools can be declared in a TOML manifest at `$XDG_CONFIG_HOME/gh-tool/config.toml` (typically `~/.config/gh-tool/config.toml`). This file is suitable for checking into a dotfiles repo.

```toml
[settings]
# Optional path overrides
# data_home = "~/.local/share/gh-tool"

[[tool]]
repo = "junegunn/fzf"
pattern = "fzf-*-{{os}}_{{arch}}.tar.gz"
bin = ["fzf"]

[[tool]]
repo = "jqlang/jq"
pattern = "jq-macos-arm64"
tag = "jq-1.7.1"
bin = ["jq"]

[[tool]]
repo = "dandavison/delta"
pattern = "delta-*-aarch64-apple-darwin.tar.gz"
bin = ["delta"]
man = ["delta.1.gz"]
completions = ["completion/_delta"]
```

Install all tools from the manifest:

```sh
gh tool install
```

### Tool Attributes

| Attribute     | Description                                          | Default       |
|---------------|------------------------------------------------------|---------------|
| `repo`        | GitHub `owner/repo`                                  | required      |
| `pattern`     | Glob pattern for release asset (supports `{{os}}`, `{{arch}}`) | required |
| `tag`         | Release tag to pin                                   | latest        |
| `bin`         | Binary names to symlink                              | `[<toolname>]`|
| `man`         | Man page paths relative to extracted archive         | none          |
| `completions` | Completion file paths relative to extracted archive  | none          |

## Commands

| Command            | Description                             |
|--------------------|-----------------------------------------|
| `install [repo]`   | Install a tool or all manifest tools    |
| `remove <repo>`    | Remove an installed tool                |
| `list`             | List installed tools with update status |
| `upgrade [repo]`   | Upgrade to latest release               |
| `cache list`       | Show cached downloads                   |
| `cache clean`      | Remove cached downloads                 |
| `shell <bash\|zsh>` | Generate shell integration config      |
| `version`          | Print version                           |

## Filesystem Layout

```
$XDG_CONFIG_HOME/gh-tool/config.toml     # Manifest
$XDG_DATA_HOME/gh-tool/bin/              # Binary symlinks
$XDG_DATA_HOME/gh-tool/share/man/man1/   # Man page symlinks
$XDG_DATA_HOME/gh-tool/tools/<name>/     # Extracted tool payloads
$XDG_STATE_HOME/gh-tool/<name>.toml      # Installed version tracking
$XDG_CACHE_HOME/gh-tool/<name>/          # Download cache
```

## License

MIT
