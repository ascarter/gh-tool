# Shell integration & paths

## Shell integration

`gh tool shell` prints a snippet you `eval` from your shell profile. It adds the gh-tool `bin/` directory to `PATH`, sets `MANPATH`, and (in interactive shells) loads installed completions.

```sh
# bash (~/.bashrc)
eval "$(gh tool shell bash)"

# zsh (~/.zshrc)
eval "$(gh tool shell zsh)"

# fish (~/.config/fish/config.fish)
gh tool shell fish | source
```

If you omit the shell argument, gh-tool detects it from `$SHELL`:

```sh
eval "$(gh tool shell)"
```

### Skipping completions

In a minimal sandbox or container where you only need `PATH` set up, use `--no-completions`:

```sh
eval "$(gh tool shell bash --no-completions)"
```

The completion-loading block is also guarded by an interactive-mode check, so it's safe to source from non-interactive scripts even without the flag — completions are simply skipped.

## Paths

By default gh-tool follows the [XDG Base Directory Specification][xdg]:

```
~/.config/gh-tool/config.toml          Manifest (TOML)
~/.local/share/gh-tool/bin/            Binary symlinks (add to PATH)
~/.local/share/gh-tool/share/man/man1/ Man page symlinks
~/.local/share/gh-tool/tools/<name>/   Extracted tool payloads
~/.local/state/gh-tool/<name>.toml     Installed version tracking
~/.cache/gh-tool/<name>/               Download cache
```

XDG environment variables (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`) are honored when set.

[xdg]: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html

## `GHTOOL_HOME`: single-root override

For containers, sandboxes, ephemeral agent sessions, or anywhere you want all gh-tool state under one directory, set `GHTOOL_HOME`:

```sh
export GHTOOL_HOME="$HOME/.gh-tool"
```

Layout under `GHTOOL_HOME`:

```
$GHTOOL_HOME/config/config.toml
$GHTOOL_HOME/data/bin/
$GHTOOL_HOME/data/share/man/man1/
$GHTOOL_HOME/data/tools/<name>/
$GHTOOL_HOME/state/<name>.toml
$GHTOOL_HOME/cache/<name>/
```

### Resolution order

1. `$GHTOOL_HOME` (single-root override; subdirs are `config/`, `data/`, `state/`, `cache/`).
2. XDG environment variables, each with a `gh-tool` segment appended.
3. Platform defaults, each with a `gh-tool` segment appended.

`GHTOOL_HOME` and the XDG vars are mutually exclusive: when `GHTOOL_HOME` is set, XDG vars are ignored.

### Inheriting `GHTOOL_HOME` in subshells

When `GHTOOL_HOME` is set in the environment at the time `gh tool shell` runs, it's emitted as an `export` (or `set -gx` for fish) so subshells started from your profile inherit the same root:

```sh
export GHTOOL_HOME="$HOME/.gh-tool"
eval "$(gh tool shell)"
```

## Container & agent use

See [containers.md](containers.md) for a walkthrough using gh-tool inside Docker, Toolbox, or a cloud agent session.
