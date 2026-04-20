#!/usr/bin/env bash
# Bootstrap gh-tool inside any container or sandbox where `gh` is already
# installed and authenticated (GH_TOKEN in the environment is enough).
#
# Layout: everything lives under $GHTOOL_HOME so cleanup is a single rm -rf.
set -euo pipefail

: "${GHTOOL_HOME:=/opt/gh-tool}"
export GHTOOL_HOME
export PATH="$GHTOOL_HOME/data/bin:$PATH"

manifest="${1:-/etc/gh-tool/config.toml}"

gh --version >/dev/null
gh auth status >/dev/null

gh extension install ascarter/gh-tool >/dev/null 2>&1 || true

mkdir -p "$GHTOOL_HOME/config"
cp "$manifest" "$GHTOOL_HOME/config/config.toml"

gh tool install
