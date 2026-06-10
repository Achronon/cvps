# CVPS CLI

Command-line interface for managing ClaudeVPS sandboxes.

## Installation

### macOS (Homebrew)

```bash
# Requires public tap repo: Achronon/homebrew-tap
brew tap Achronon/tap
brew install cvps
```

### Quick Install Script (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/Achronon/cvps/main/scripts/install.sh | sh
```

The installer verifies SHA256 checksums from the GitHub release before install.

### Linux

```bash
# AMD64
curl -L https://github.com/Achronon/cvps/releases/latest/download/cvps-linux-amd64 -o /usr/local/bin/cvps
chmod +x /usr/local/bin/cvps

# ARM64
curl -L https://github.com/Achronon/cvps/releases/latest/download/cvps-linux-arm64 -o /usr/local/bin/cvps
chmod +x /usr/local/bin/cvps
```

### Alpine / Minimal Linux

```bash
apk add --no-cache curl ca-certificates
update-ca-certificates
curl -fsSL https://raw.githubusercontent.com/Achronon/cvps/main/scripts/install.sh | sh
```

### Windows

Download from [GitHub Releases](https://github.com/Achronon/cvps/releases)
or use winget:

```powershell
# Coming soon
# winget install achronon.cvps
```

### Distribution Setup (Maintainers)

See `docs/distribution.md` for public Homebrew tap and release automation setup.

### From Source

```bash
go install github.com/achronon/cvps/cmd/cvps@latest
```

## Quick Start

```bash
# Login
cvps login

# Create sandbox
cvps up --name my-project

# Check status
cvps status

# Connect
cvps connect

# Sync files
cvps sync

# Terminate
cvps down
```

If `cvps status` says no sandbox context, either create one with `cvps up`, list all with
`cvps status --all`, or pass an explicit sandbox ID like `cvps status sbx-abc123`.

If `cvps connect <sandbox-id>` reports `sandbox is not running (status: RUNNING)`,
upgrade to `v0.1.4+`:

```bash
brew update
brew upgrade cvps
cvps version
```

Then connect directly from the `status --all` output:

```bash
cvps status --all
cvps connect <sandbox-id>
# or by exact name
cvps connect --name <sandbox-name>
```

`cvps connect <arg>` treats `<arg>` as a sandbox ID. To connect by name, use
`cvps connect --name <sandbox-name>`.

`--method websocket` is currently unsupported in the CLI because the backend terminal
transport is Socket.IO. Use the default SSH method.

## Commands

| Command | Description |
|---------|-------------|
| `cvps login` | Authenticate with ClaudeVPS |
| `cvps logout` | Log out |
| `cvps up` | Provision new sandbox |
| `cvps down` | Terminate sandbox |
| `cvps status` | Show sandbox status |
| `cvps connect` | Open terminal to sandbox |
| `cvps sync` | Start file synchronization |
| `cvps migrate` | Upload local workspace |
| `cvps config` | Manage configuration |
| `cvps token` | Mint/manage API tokens for agents (scopes advisory until backend enforcement) |

## Configuration

Config file: `~/.cvps/config.yaml`

```yaml
api_key: cvps_xxx
api_base_url: https://api.claudevps.com
# Optional: resolve the token on demand instead of storing it on disk
# token_command: op read op://vault/cvps/token

defaults:
  cpu_cores: 1
  memory_gb: 2
  storage_gb: 5
```

## Headless / agent auth

No browser, no TTY, token never on argv:

```bash
# Option 1: environment variable (fresh shell, no config file needed)
export CVPS_API_TOKEN=cvps_xxx
cvps status

# Option 2: persist a token read from stdin (mirrors gh auth login --with-token)
cvps login --with-token < token.txt

# Option 3: resolve the token on demand (never touches disk)
cvps config set token_command "op read op://vault/cvps/token"
```

Credential precedence: env (`CVPS_API_TOKEN` > `CVPS_TOKEN` > `CVPS_API_KEY`)
> `token_command` > stored config (`access_token` then `api_key`).

Pass `--no-interactive` to make any command that would prompt (login method
chooser, `down` confirmation, `secret rm` confirmation, `secret create`
hidden prompt) fail fast with a clear error instead of hanging an agent.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CVPS_API_TOKEN` | API token (highest-precedence credential) |
| `CVPS_TOKEN` | API token (alias; injected by the provisioner for in-sandbox use) |
| `CVPS_API_KEY` | API key (legacy alias) |
| `CVPS_TOKEN_COMMAND` | Shell command whose stdout is the token (overrides `token_command`) |
| `CVPS_API_URL` | API URL (overrides config) |

## Development

### Build

```bash
make build
```

### Test

```bash
make test
```

### Lint

```bash
make lint
```

### Clean

```bash
make clean
```

## License

Proprietary - ClaudeVPS SaaS
