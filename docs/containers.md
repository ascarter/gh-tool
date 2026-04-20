# Containers, toolboxes, and ephemeral sessions

`gh-tool` is designed to be portable: a manifest committed to your dotfiles is enough to reproduce a working toolchain anywhere `gh` is installed and authenticated. This page shows how to use it inside containers, [Fedora Toolbox][toolbox], and ephemeral agent or cloud-IDE sessions.

[toolbox]: https://containertoolbx.org/

## Prerequisites

1. **`gh` is installed** — gh-tool is a `gh` extension. Install `gh` from your package manager, the [official binaries][gh-install], or the GitHub apt/dnf repos.
2. **`gh` is authenticated** — `gh auth status` should succeed. In a non-interactive environment, set `GH_TOKEN` to a PAT with `read:packages` and (optionally) `repo` scope.
3. **`gh tool` extension is installed** — `gh extension install ascarter/gh-tool`.

[gh-install]: https://github.com/cli/cli#installation

## Recommended layout: `GHTOOL_HOME`

In a container or sandbox, point gh-tool at one root directory so everything (manifest, binaries, cache, state) lives in a predictable place:

```sh
export GHTOOL_HOME="/opt/gh-tool"
```

This sidesteps XDG path discovery and makes cleanup trivial (`rm -rf "$GHTOOL_HOME"`). See [shell.md](shell.md#ghtool_home-single-root-override) for the full layout.

## Bootstrap script

The minimum bootstrap inside any Debian/Fedora-based container:

```sh
#!/usr/bin/env bash
set -euo pipefail

export GHTOOL_HOME="${GHTOOL_HOME:-/opt/gh-tool}"
export PATH="$GHTOOL_HOME/data/bin:$PATH"

gh extension install ascarter/gh-tool
mkdir -p "$GHTOOL_HOME/config"
cp /path/to/your/config.toml "$GHTOOL_HOME/config/config.toml"
gh tool install
```

## Docker examples

The [`examples/`](../examples) directory in this repo contains:

- `Dockerfile.ubuntu` — `ubuntu:24.04` base, installs `gh` from the GitHub apt repo.
- `Dockerfile.fedora` — `fedora:latest` base, installs `gh` from `dnf`.
- `config.toml` — a sample manifest (`fzf`, `ripgrep`, `jq`).
- `install.sh` — the bootstrap script above, suitable for any sandbox.

Build and run:

```sh
cd examples
docker build -f Dockerfile.ubuntu --build-arg GH_TOKEN="$(gh auth token)" -t gh-tool-demo .
docker run --rm -it gh-tool-demo bash -c 'fzf --version && rg --version && jq --version'
```

If you'd rather mount a manifest from the host than bake one into the image:

```sh
docker run --rm -it \
  -v "$PWD/my-config.toml:/opt/gh-tool/config/config.toml:ro" \
  -e GH_TOKEN="$(gh auth token)" \
  gh-tool-demo \
  bash -c 'gh tool install && exec bash'
```

## Fedora Toolbox

Inside a toolbox container, the flow is the same:

```sh
toolbox enter
sudo dnf install -y gh
gh auth login           # one-time, persists across toolbox sessions
gh extension install ascarter/gh-tool

export GHTOOL_HOME="$HOME/.gh-tool"
echo 'export GHTOOL_HOME="$HOME/.gh-tool"' >> ~/.bashrc
echo 'eval "$(gh tool shell)"'             >> ~/.bashrc

# Drop your manifest into place and install
mkdir -p "$GHTOOL_HOME/config"
cp ~/dotfiles/gh-tool.toml "$GHTOOL_HOME/config/config.toml"
gh tool install
```

## Cloud agent sessions

For a coding agent operating in an ephemeral workspace:

1. Have the agent ensure `gh` is installed and authenticated (`GH_TOKEN` from the session secrets).
2. Have it install the extension: `gh extension install ascarter/gh-tool`.
3. Have it set `GHTOOL_HOME=$WORKSPACE/.gh-tool` so installs are scoped to the session and disappear when the workspace is torn down.
4. Have it write or fetch the project's `config.toml` and run `gh tool install`.

A drop-in [skill file](agent-skill.md) describes this flow in a form you can copy into your own repo so an agent picks it up automatically.

## Cleanup

```sh
gh tool reset                       # wipe data/state/cache, keep manifest
rm -rf "$GHTOOL_HOME"               # nuke everything including manifest
gh extension remove ascarter/gh-tool
```
