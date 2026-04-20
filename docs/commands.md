# Commands

```
gh tool add <owner/repo>        Interactively author a manifest entry (does not install)
gh tool install [owner/repo]    Reconcile from manifest, or install a single tool
gh tool remove <owner/repo>     Remove an installed tool (manifest is not modified)
gh tool list                    List installed tools with installed and latest versions
gh tool upgrade [owner/repo]    Upgrade to latest release (state-driven)
gh tool reset                   Remove all installed tools and clear gh-tool data
gh tool cache list              Show cached downloads
gh tool cache clean [tool]      Remove cached downloads
gh tool shell [bash|zsh|fish]   Print shell integration config (auto-detects from $SHELL)
gh tool version                 Print version
```

## Notable flags

- **`add`**: `--file/-f`, `--tag/-t`, `--no-write` (preview the generated block without saving).
- **`install`**: `--pattern/-p`, `--tag/-t`, `--bin`, `--man`, `--completion`, `--no-verify`, `--force`, `--file/-f`, `--jobs/-j`, `--no-progress`, `--verbose/-v`.
- **`list`**: `--outdated`, `--pinned`.
- **`upgrade`**: `--jobs/-j`, `--no-progress`, `--verbose/-v`.
- **`reset`**: `--yes/-y`.
- **`shell`**: `--no-completions`.

## Common flag semantics

| Flag               | Purpose                                                        |
|--------------------|----------------------------------------------------------------|
| `-j, --jobs N`     | Parallelism cap (default `min(8, NumCPU)`).                    |
| `--no-progress`    | Disable the live progress UI; print one line per event.        |
| `-v, --verbose`    | Log every step (download, verify, extract) per tool.           |
| `--no-verify`      | Skip attestation verification (install only).                  |

## How install works

1. `gh tool install` downloads a release asset via `gh release download` into a cache directory.
2. The asset is verified with `gh attestation verify` (best-effort — most repos don't publish attestations yet).
3. Archives (`tar.gz`, `tar.xz`, `zip`) are extracted; bare binaries are copied directly. If an archive has a single top-level directory, it is stripped automatically.
4. Symlinks are created from the bin directory into the extracted tool directory. Use `source:link` in `bin` to rename binaries (e.g., `jq-macos-arm64:jq`).
5. A state file under the state directory records the installed tag, the resolved download pattern, and the symlinked `bin`/`man`/`completions`. `list`, `remove`, and `upgrade` operate from these state files; the manifest is only consulted by `install` (and by `list` for drift reporting).
