# Agent skill: provisioning tools with `gh tool`

> **What this is.** A portable, copy-into-your-own-repo skill file that teaches a coding agent (Copilot CLI, Claude Code, Cursor, etc.) how to use the GitHub CLI plus the [`gh-tool`](https://github.com/ascarter/gh-tool) extension to install and manage CLI tools from a manifest.
>
> **Where to put it.** Drop this file into your repo at one of:
> - `AGENTS.md` (or include this content from there)
> - `.github/copilot-instructions.md`
> - `.cursor/rules/gh-tool.md`
> - any other location your agent reads instructions from
>
> **What it is _not_.** It's not a config file for `gh-tool` itself, and it's not a skill specific to one agent platform. It's prose instructions an agent reads.

---

## Capability summary

You (the agent) can provision a reproducible set of CLI tools by:

1. Verifying `gh` is installed and authenticated.
2. Installing the `gh-tool` extension once.
3. Reading or authoring a TOML manifest that lists the tools the project needs.
4. Running `gh tool install` to materialize the binaries on `PATH`.

`gh-tool` only fetches assets from GitHub Releases. It does not run arbitrary install scripts.

## Prerequisites — verify before doing anything else

```sh
gh --version          # must succeed
gh auth status        # must show an authenticated account
```

If `gh` is missing, install it from the OS package manager (`apt install gh`, `dnf install gh`, `brew install gh`) or the official binaries at https://github.com/cli/cli#installation. Do **not** try to install `gh` via `gh-tool` — that's a chicken-and-egg.

If `gh auth status` fails and you have a token in the environment (e.g., `GH_TOKEN`), it should work automatically. Otherwise stop and ask the user to authenticate.

## One-time setup

```sh
gh extension install ascarter/gh-tool
```

In a sandbox or ephemeral workspace, also set:

```sh
export GHTOOL_HOME="$WORKSPACE/.gh-tool"
```

so all gh-tool state lives under the workspace and is cleaned up with it. `$WORKSPACE` here is whatever your session's writable working directory is.

Then activate the gh-tool `bin/` on `PATH`:

```sh
eval "$(gh tool shell --no-completions)"
```

(`--no-completions` keeps the snippet small and side-effect-free in non-interactive shells.)

## The manifest

The manifest is a TOML file. Default location is `~/.config/gh-tool/config.toml`, or `$GHTOOL_HOME/config/config.toml` when `GHTOOL_HOME` is set. Minimal example:

```toml
[[tool]]
repo = "junegunn/fzf"
pattern = "fzf-*-{{os}}_{{arch}}.tar.gz"
bin = ["fzf"]

[[tool]]
repo = "BurntSushi/ripgrep"
pattern = "ripgrep-*-{{triple}}.tar.gz"
bin = ["rg"]

[[tool]]
repo = "jqlang/jq"
pattern = "jq-{{platform}}-{{arch}}"
bin = ["jq-{{platform}}-{{arch}}:jq"]
```

Template variables: `{{os}}` (darwin/linux/windows), `{{arch}}` (arm64/amd64), `{{platform}}` (macos/linux/windows), `{{gnuarch}}` (aarch64/x86_64), `{{triple}}` (Rust target triple, e.g., `x86_64-unknown-linux-gnu`).

`bin` accepts a `source:link` syntax to rename platform-specific filenames to a clean symlink (e.g., `jq-macos-arm64:jq`).

For full details see [manifest.md](manifest.md).

## Common operations

| Goal                                  | Command                                    |
|---------------------------------------|--------------------------------------------|
| Install everything in the manifest    | `gh tool install`                          |
| Install one tool ad-hoc               | `gh tool install owner/repo --pattern '...' --bin name` |
| Add a tool to the manifest interactively | `gh tool add owner/repo`                |
| List installed tools and versions     | `gh tool list`                             |
| Show outdated tools                   | `gh tool list --outdated`                  |
| Upgrade everything                    | `gh tool upgrade`                          |
| Upgrade one tool                      | `gh tool upgrade owner/repo`               |
| Remove an installed tool              | `gh tool remove owner/repo`                |
| Wipe all installed tools              | `gh tool reset --yes`                      |

## Decision rules for the agent

- **If the project already has a manifest** (look for `config.toml` referenced in dotfiles, a `gh-tool.toml` at repo root, or instructions pointing to one), use `--file` to point at it: `gh tool install --file ./gh-tool.toml`.
- **If the project asks you to add a new tool**, prefer `gh tool add owner/repo` over hand-editing TOML. `add` inspects the latest release and produces a working entry. Only fall back to manual edits if `add` can't determine the right pattern.
- **Never modify the manifest as a side effect of `install`.** The manifest is a read-only input. `gh tool add` is the only command that writes to it.
- **Do not use `sudo`.** gh-tool installs into the user's home (or `$GHTOOL_HOME`), never system-wide.
- **Do not commit installed binaries or tool state into the repo.** Only the manifest belongs in version control.

## Troubleshooting

- `gh auth status` fails → ask the user to run `gh auth login` or set `GH_TOKEN`.
- `gh tool install` reports `no asset matched pattern` → the upstream release asset names changed. Inspect the release page and update `pattern` (or set a per-platform override under `[tool.patterns]`).
- Attestation verification warnings are non-fatal — most projects don't publish attestations yet.
- A binary doesn't appear on `PATH` → confirm the `gh tool shell` snippet was eval'd in the current shell, or that `$GHTOOL_HOME/data/bin` (or `~/.local/share/gh-tool/bin`) is on `PATH`.
