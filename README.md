# gh-tool

A GitHub CLI extension for installing and managing binary tools from GitHub releases.

Maintain a TOML manifest in your dotfiles that lists the tools you want. `gh tool` downloads release assets, extracts them, and symlinks binaries, man pages, and shell completions into one `bin/` directory you add to your `PATH`. The same manifest reproduces the same toolchain on a new laptop, in a container, or in a fresh agent session.

## Install

```sh
gh extension install ascarter/gh-tool
```

Add gh-tool's `bin/` to your `PATH` via your shell profile:

```sh
# bash, zsh — auto-detects from $SHELL
eval "$(gh tool shell)"

# fish
gh tool shell fish | source
```

Open a new shell (or `source` the file you edited) before continuing.

## Quick start

```sh
gh tool add sharkdp/bat          # interactive: pick the asset, write a manifest entry
gh tool add BurntSushi/ripgrep
gh tool install                  # download, extract, symlink
gh tool list                     # see what's installed and what's outdated
gh tool upgrade                  # pull latest releases for everything
```

Your manifest lives at `~/.config/gh-tool/config.toml` (or `$GHTOOL_HOME/config/config.toml`) — commit it to your dotfiles. `gh tool install` on a new machine reproduces the same set.

## Documentation

- [Manifest reference][manifest] — tool attributes, pattern variables, per-platform overrides, ad-hoc installs.
- [Commands & flags][commands] — every command with its flags and a short description.
- [Shell integration & paths][shell] — `gh tool shell`, XDG paths, `GHTOOL_HOME`.
- [Containers, toolboxes & ephemeral sessions][containers] — running gh-tool in Docker, Toolbox, or a cloud agent.
- [Agent skill][agent] — a portable markdown file you can copy into your own repo to teach a coding agent how to use gh + gh-tool.
- [Examples][examples] — sample manifest, Dockerfiles for Ubuntu and Fedora, a bootstrap script.

[manifest]: docs/manifest.md
[commands]: docs/commands.md
[shell]: docs/shell.md
[containers]: docs/containers.md
[agent]: docs/agent-skill.md
[examples]: examples/

## Removing gh-tool

```sh
gh tool reset                       # remove all installed tools and clear data
gh extension remove ascarter/gh-tool
```

`reset` preserves your manifest. Delete `~/.config/gh-tool/config.toml` (or `$GHTOOL_HOME`) yourself for a clean slate.

## Development

```sh
make build      # build with version from git tags
make test       # run tests
make vet        # run go vet
make clean      # remove binary and caches
```

## License

MIT
