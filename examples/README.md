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

## Trimming image size

The example Dockerfiles keep the download cache around so subsequent `gh tool install` / `gh tool upgrade` runs in the container don't need to re-download. If you want a smaller, immutable image, you have two options.

### Option A — drop the cache in the same layer

Append a `gh tool cache clean` to the bootstrap `RUN` so the cache never lands in the image layer:

```dockerfile
RUN --mount=type=secret,id=gh_token \
    GH_TOKEN="$(cat /run/secrets/gh_token)" gh-tool-bootstrap /etc/gh-tool/config.toml && \
    gh tool cache clean
```

Smallest delta from the existing example. Keeps `gh`, the `gh-tool` extension, and the manifest in the image — useful if you want to upgrade or add tools at runtime.

### Option B — multi-stage: copy only the installed tools

Use a builder stage that has `gh` and the extension, then copy just `$GHTOOL_HOME/data/bin` and `$GHTOOL_HOME/data/tools` into a minimal runtime image. The runtime image doesn't need `gh` at all:

```dockerfile
# syntax=docker/dockerfile:1.7
FROM ubuntu:24.04 AS builder
ENV DEBIAN_FRONTEND=noninteractive GHTOOL_HOME=/opt/gh-tool
# ... (install gh + extension as in Dockerfile.ubuntu) ...
COPY config.toml /etc/gh-tool/config.toml
COPY install.sh  /usr/local/bin/gh-tool-bootstrap
RUN chmod +x /usr/local/bin/gh-tool-bootstrap
RUN --mount=type=secret,id=gh_token \
    GH_TOKEN="$(cat /run/secrets/gh_token)" gh-tool-bootstrap /etc/gh-tool/config.toml

FROM ubuntu:24.04
ENV PATH=/opt/gh-tool/data/bin:/usr/local/bin:/usr/bin:/bin
COPY --from=builder /opt/gh-tool/data /opt/gh-tool/data
CMD ["bash"]
```

Smallest runtime image. Good for distributing a fixed toolchain. You can't add or upgrade tools inside the running container — rebuild instead.

Pick A when the container is a workstation you'll iterate in. Pick B when it's an immutable artifact (CI runner image, sealed agent base, etc.).



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
