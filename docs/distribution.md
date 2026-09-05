# CLI Distribution (Public GitHub)

This document explains the setup needed so users can install `cvps` via Homebrew and GitHub release channels from public repositories.

## Goals

- `brew tap Achronon/tap && brew install cvps` works for macOS users
- GitHub Releases provide binaries for macOS/Linux/Windows
- A one-command installer is available for macOS/Linux

## One-time Setup

### 1) Create a public Homebrew tap repository

Create this repository as **public**:

- `Achronon/homebrew-tap`

Homebrew command users run:

```bash
brew tap Achronon/tap
brew install cvps
```

`Achronon/tap` maps to `Achronon/homebrew-tap`.

### 2) Canonical release publisher

The private `Achronon/claudevps` monorepo owns the CLI source under `cli/`
and publishes versioned binaries and checksums to the public
`Achronon/cvps` GitHub releases. Publish releases through the monorepo's CLI
release workflow using its reviewed source commit.

The public repository's `.github/workflows/build-cli.yml` runs Go tests and
cross-platform builds on pull requests and `main`. Its outputs are CI
artifacts, not release assets. It has read-only repository permissions and
does not run on version tags or publish releases. Do not restore a second
release writer here: a public tag may point at older distribution-source code
and replace the canonical binary with a stale build.

### 3) Homebrew updates

`Achronon/homebrew-tap` updates its formula through its own scheduled
`.github/workflows/update-formula.yml`, which polls public releases. The public
CLI repository does not publish formula updates and needs no tap publishing
credential.

## Modern install channels

### Homebrew

```bash
brew tap Achronon/tap
brew install cvps
```

### Curl installer (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/Achronon/cvps/main/scripts/install.sh | sh
```

### Direct binaries

From GitHub Releases:

- `cvps-darwin-arm64`
- `cvps-darwin-amd64`
- `cvps-linux-arm64`
- `cvps-linux-amd64`
- `cvps-windows-amd64.exe`

## Troubleshooting

### `brew tap Achronon/tap` fails with “repository not found”

Cause:

- `Achronon/homebrew-tap` does not exist or is private.

Fix:

- Create `Achronon/homebrew-tap` as public
- Confirm the tap repository is public and reachable

### Formula does not update after tag release

Check:

- The monorepo release job published the expected binaries and checksums
- The tap repository scheduled update workflow completed successfully
- Tag is stable (`vX.Y.Z`, not prerelease suffix)
