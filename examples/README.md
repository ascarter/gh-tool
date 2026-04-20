# Examples

Reference setups for using `gh-tool` in containers and sandboxes. See [`docs/containers.md`](../docs/containers.md) for the narrative walkthrough.

## Files

| File                 | Purpose                                                             |
|----------------------|---------------------------------------------------------------------|
| `config.toml`        | Sample manifest with `fzf`, `ripgrep`, and `jq`.                    |
| `install.sh`         | Bootstrap script: installs the gh-tool extension, drops the manifest into place, and runs `gh tool install`. |
| `Dockerfile.ubuntu`  | `ubuntu:24.04` image with `gh` from the GitHub apt repo + gh-tool.  |
| `Dockerfile.fedora`  | `fedora:latest` image with `gh` from `dnf` + gh-tool.               |

All variants set `GHTOOL_HOME=/opt/gh-tool` so installs live under one tree.

## Build & run

You need a GitHub token in the environment. The simplest source is your local `gh` login:

```sh
export GH_TOKEN="$(gh auth token)"
```

Build and run either image. The build context must be this `examples/` directory (the Dockerfiles `COPY` `config.toml` and `install.sh` from it). The token is passed as a [BuildKit secret][buildkit-secret] (`--secret id=gh_token,env=GH_TOKEN`) so it never lands in image layers or `docker history`:

```sh
# From the repo root
docker build -f examples/Dockerfile.ubuntu --secret id=gh_token,env=GH_TOKEN -t gh-tool-ubuntu examples
docker build -f examples/Dockerfile.fedora --secret id=gh_token,env=GH_TOKEN -t gh-tool-fedora examples

# Or from inside examples/
cd examples
docker build -f Dockerfile.ubuntu --secret id=gh_token,env=GH_TOKEN -t gh-tool-ubuntu .
docker build -f Dockerfile.fedora --secret id=gh_token,env=GH_TOKEN -t gh-tool-fedora .
```

Then:

```sh
docker run --rm -it gh-tool-ubuntu bash -lc 'fzf --version && rg --version && jq --version'
docker run --rm -it gh-tool-fedora bash -lc 'fzf --version && rg --version && jq --version'
```

[buildkit-secret]: https://docs.docker.com/build/building/secrets/

> **Note on the token:** the token is mounted as a BuildKit secret and never appears in the image or in `docker history`. The `--secret id=gh_token,env=GH_TOKEN` form reads from the `GH_TOKEN` env var on your host.

## Bind-mounting your own manifest

To install your own toolset instead of the sample, build the image once, then mount your manifest at runtime and re-run the bootstrap:

```sh
docker run --rm -it \
  -v "$PWD/my-config.toml:/etc/gh-tool/config.toml:ro" \
  -e GH_TOKEN="$GH_TOKEN" \
  gh-tool-ubuntu \
  bash -lc 'gh-tool-bootstrap /etc/gh-tool/config.toml && exec bash'
```

## Using the script outside Docker

`install.sh` works in any sandbox where `gh` is already installed and authenticated (Fedora Toolbox, GitHub Codespaces, dev containers, agent workspaces). Set `GHTOOL_HOME` to a writable directory and pass the manifest path:

```sh
GHTOOL_HOME="$HOME/.gh-tool" ./install.sh ./my-config.toml
```
