# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Purpose

`cvps` — Go CLI for managing ClaudeVPS sandboxes (login, up/down, status, connect, sync). Distribution is the public `Achronon/cvps` GitHub repo (Homebrew tap + release downloads); this private repo `claudevps-cli` is the source of truth and where development happens.

This repo is a peer of `claudevps` (the SaaS that the CLI talks to). Issue tracking is folded into the **`claudevps` Linear project** in the **Helheim** team — there is no separate Drive `llm-context/` for the CLI; durable CLI context lives here.

## Source of truth pointers

- **Backlog & shipped history:** Linear team `Helheim` / project `claudevps` (CLI work tagged in the issue title or body — search `cvps` / `cli`).
- **Distribution context:** `claudevps` engineering.md decision log entry 2026-04-23 ("cvps CLI releases publish to public `Achronon/cvps`...") explains why releases publish to the public mirror rather than this private repo.
- **OAuth device-auth flow:** see `docs/authentication.md` and the 401 auto-reauth helper in `cmd/`.
- **Release/Homebrew automation:** `docs/distribution.md` + `docs/homebrew-formula.rb`.

## Layout

| Path | Purpose |
|---|---|
| `cmd/cvps/` | CLI entry — Cobra root + subcommands |
| `internal/api/` | HTTP client against the claudevps backend |
| `internal/cmd/` | Subcommand implementations |
| `internal/config/` | Viper config + token storage |
| `internal/migration/`, `internal/mutagen/`, `internal/terminal/`, `internal/version/` | Helpers for sandbox sync, terminal multiplexing, build version metadata |
| `bin/` | Local build artefacts (gitignored) |
| `scripts/` | Install + release scripts mirrored to `Achronon/cvps` |
| `docs/` | Authentication flow, distribution setup, Homebrew formula template |

## Build / Test / Lint

```bash
make build      # builds bin/cvps with VERSION + COMMIT ldflags
make test       # go test -v ./...
make lint       # golangci-lint run
make install    # build + cp to /usr/local/bin
```

Go 1.23+. Direct deps: cobra, viper, gorilla/websocket, briandowns/spinner, schollz/progressbar.

## Cross-repo expectations

- **Backend contract** lives in `claudevps` (NestJS). When changing CLI request/response shapes, check the matching backend route under `claudevps/` and file a Linear ticket in the `claudevps` project covering both sides.
- **Releases** push to public `Achronon/cvps` (not `Achronon/claudevps`), because Homebrew needs anonymous downloads. Don't redirect this in CI without re-reading the engineering.md decision log first.
- **Auth tokens** live under `~/.config/cvps/`. The CLI auto-runs the OAuth device flow on 401 (see `reauthenticateOnUnauthorized` in `cmd/`); API-key sessions surface the auth error verbatim.

## What this file is not

- Not a backlog. Use Linear `Helheim` / `claudevps` project; tag CLI items by `cvps` in the title or by referencing the binary in the description.
- Not a product spec. The CLI follows the SaaS product; product context lives in the `claudevps` Drive `llm-context/`.
