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

### 2) Add GitHub secret for tap publishing

In `Achronon/cvps`, add repository secret:

- `HOMEBREW_TAP_TOKEN`

Token requirements:

- Fine-grained PAT
- Repository access to `Achronon/homebrew-tap`
- Contents: Read and Write

### 3) Tag-based release process

Create and push a version tag:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This runs `.github/workflows/build-cli.yml` which:

- builds `cvps` binaries for target platforms
- publishes GitHub Release assets
- generates `Formula/cvps.rb` with real checksums
- pushes formula updates to `Achronon/homebrew-tap`

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
- Confirm token can push to that repo

### Formula does not update after tag release

Check:

- `HOMEBREW_TAP_TOKEN` exists and has write access
- Release job completed before Homebrew job
- Tag is stable (`vX.Y.Z`, not prerelease suffix)
